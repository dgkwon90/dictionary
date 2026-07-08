package gemini

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"neulsang/desktopd/internal/domain/explain"
)

func TestClientExplainSuccess(t *testing.T) {
	resultJSON := mustMarshalExplainResult(t, validGeminiExplainResult())
	rawResponseJSON := geminiResponse(resultJSON)
	var called atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called.Add(1)
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/v1beta/models/gemini-flash-lite-latest:generateContent" {
			t.Errorf("path = %s", r.URL.Path)
		}
		if got := r.Header.Get("x-goog-api-key"); got != "test-key" {
			t.Errorf("x-goog-api-key = %q, want test-key", got)
		}

		var requestBody struct {
			Contents []struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"contents"`
			GenerationConfig struct {
				ResponseMimeType string `json:"responseMimeType"`
				ResponseSchema   any    `json:"responseSchema"`
			} `json:"generationConfig"`
		}
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Errorf("decode request body: %v", err)
		}
		if len(requestBody.Contents) != 1 || len(requestBody.Contents[0].Parts) != 1 || !strings.Contains(requestBody.Contents[0].Parts[0].Text, "stale cache") {
			t.Errorf("prompt did not contain input text: %#v", requestBody.Contents)
		}
		if requestBody.GenerationConfig.ResponseMimeType != "application/json" {
			t.Errorf("responseMimeType = %q", requestBody.GenerationConfig.ResponseMimeType)
		}
		if requestBody.GenerationConfig.ResponseSchema == nil {
			t.Error("responseSchema is nil")
		}

		w.Header().Set("Content-Type", "application/json")
		if _, err := io.WriteString(w, rawResponseJSON); err != nil {
			t.Errorf("write response: %v", err)
		}
	}))
	defer server.Close()

	client := New("test-key", WithBaseURL(server.URL))
	result, raw, err := client.Explain(context.Background(), "stale cache")
	if err != nil {
		t.Fatalf("Explain() error = %v", err)
	}
	if result.BriefKo != "짧은 설명" || result.DomainCategory != "backend" {
		t.Fatalf("result = %#v", result)
	}
	if raw != rawResponseJSON {
		t.Fatalf("rawResponseJSON = %q, want %q", raw, rawResponseJSON)
	}
	if called.Load() != 1 {
		t.Fatalf("called = %d, want 1", called.Load())
	}
}

func TestClientExplainRequiresAPIKey(t *testing.T) {
	var called atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		called.Add(1)
	}))
	defer server.Close()

	client := New("", WithBaseURL(server.URL))
	_, raw, err := client.Explain(context.Background(), "stale")
	if err == nil || !strings.Contains(err.Error(), "API key is required") {
		t.Fatalf("Explain() error = %v, want API key error", err)
	}
	if raw != "" {
		t.Fatalf("rawResponseJSON = %q, want empty", raw)
	}
	if called.Load() != 0 {
		t.Fatalf("server called = %d, want 0", called.Load())
	}
}

func TestClientExplainRetries429ThenSucceeds(t *testing.T) {
	resultJSON := mustMarshalExplainResult(t, validGeminiExplainResult())
	rawResponseJSON := geminiResponse(resultJSON)
	var called atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if called.Add(1) <= 2 {
			http.Error(w, "rate limited", http.StatusTooManyRequests)
			return
		}
		if _, err := io.WriteString(w, rawResponseJSON); err != nil {
			t.Errorf("write response: %v", err)
		}
	}))
	defer server.Close()

	client := New("test-key", WithBaseURL(server.URL), WithTimeout(2*time.Second))
	result, raw, err := client.Explain(context.Background(), "stale")
	if err != nil {
		t.Fatalf("Explain() error = %v", err)
	}
	if result.BriefKo == "" || raw != rawResponseJSON {
		t.Fatalf("result=%#v raw=%q", result, raw)
	}
	if called.Load() != 3 {
		t.Fatalf("called = %d, want 3", called.Load())
	}
}

func TestClientExplainExhausts5xxRetries(t *testing.T) {
	var called atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called.Add(1)
		http.Error(w, "temporary failure", http.StatusInternalServerError)
	}))
	defer server.Close()

	client := New("test-key", WithBaseURL(server.URL), WithTimeout(2*time.Second))
	_, raw, err := client.Explain(context.Background(), "stale")
	if err == nil {
		t.Fatal("Explain() error = nil, want failure")
	}
	if raw != "" {
		t.Fatalf("rawResponseJSON = %q, want empty", raw)
	}
	if !strings.Contains(err.Error(), "failed after 3 attempts") {
		t.Fatalf("error = %v, want retry exhaustion", err)
	}
	if called.Load() != 3 {
		t.Fatalf("called = %d, want 3", called.Load())
	}
}

func TestClientExplainFails4xxWithoutRetry(t *testing.T) {
	var called atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called.Add(1)
		http.Error(w, "bad request", http.StatusBadRequest)
	}))
	defer server.Close()

	client := New("test-key", WithBaseURL(server.URL))
	_, raw, err := client.Explain(context.Background(), "stale")
	if err == nil {
		t.Fatal("Explain() error = nil, want 4xx failure")
	}
	if raw != "" {
		t.Fatalf("rawResponseJSON = %q, want empty", raw)
	}
	if !strings.Contains(err.Error(), "status 400") || !strings.Contains(err.Error(), "bad request") {
		t.Fatalf("error = %v, want status/body", err)
	}
	if called.Load() != 1 {
		t.Fatalf("called = %d, want 1", called.Load())
	}
}

func TestClientExplainCanceledContextReturnsQuickly(t *testing.T) {
	var called atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		called.Add(1)
	}))
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	client := New("test-key", WithBaseURL(server.URL))
	_, raw, err := client.Explain(ctx, "stale")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Explain() error = %v, want context.Canceled", err)
	}
	if raw != "" {
		t.Fatalf("rawResponseJSON = %q, want empty", raw)
	}
	if called.Load() > 1 {
		t.Fatalf("called = %d, want <= 1", called.Load())
	}
}

func TestClientExplainStructuredOutputParseFailureReturnsEmptyRaw(t *testing.T) {
	rawResponseJSON := geminiResponse([]byte("not-json"))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if _, err := io.WriteString(w, rawResponseJSON); err != nil {
			t.Errorf("write response: %v", err)
		}
	}))
	defer server.Close()

	client := New("test-key", WithBaseURL(server.URL))
	_, raw, err := client.Explain(context.Background(), "stale")
	if err == nil || !strings.Contains(err.Error(), "gemini: parse structured output") {
		t.Fatalf("Explain() error = %v, want structured parse error", err)
	}
	if raw != "" {
		t.Fatalf("rawResponseJSON = %q, want empty", raw)
	}
}

func TestClientExplainEmptyCandidates(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if _, err := io.WriteString(w, `{"candidates":[]}`); err != nil {
			t.Errorf("write response: %v", err)
		}
	}))
	defer server.Close()

	client := New("test-key", WithBaseURL(server.URL))
	_, raw, err := client.Explain(context.Background(), "stale")
	if err == nil || !strings.Contains(err.Error(), "gemini: empty response") {
		t.Fatalf("Explain() error = %v, want empty response", err)
	}
	if raw != "" {
		t.Fatalf("rawResponseJSON = %q, want empty", raw)
	}
}

func validGeminiExplainResult() explain.ExplainResult {
	return explain.ExplainResult{
		InputType:        "term",
		DetectedLanguage: "en",
		BriefKo:          "짧은 설명",
		DetailedKo:       "자세한 설명",
		PronunciationKo:  "스테일 캐시",
		DomainCategory:   "backend",
		Difficulty:       0.4,
		Examples: []explain.Example{{
			English: "This cache entry is stale.",
			Korean:  "이 캐시 항목은 오래되어 최신 상태가 아니다.",
			Note:    "개발 문맥 예문",
		}},
		SubItems: []explain.SubItem{{
			SurfaceText:     "stale",
			NormalizedKey:   "stale",
			ItemType:        "word",
			MeaningKo:       "오래된",
			PronunciationKo: "스테일",
			Importance:      0.9,
			CardCandidates: []explain.ReviewCardCandidate{{
				CardType:    "meaning",
				Question:    "stale의 의미는?",
				Answer:      "오래되어 최신이 아닌 상태",
				Explanation: "stale cache처럼 쓴다.",
			}},
		}},
	}
}

func mustMarshalExplainResult(t *testing.T, result explain.ExplainResult) []byte {
	t.Helper()
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal result: %v", err)
	}
	return data
}

func geminiResponse(resultJSON []byte) string {
	return fmt.Sprintf(`{"candidates":[{"content":{"parts":[{"text":%q}]}}]}`, string(resultJSON))
}

func TestParseResponseClampsScores(t *testing.T) {
	// The model sometimes returns out-of-range difficulty/importance; the adapter
	// must clamp them so a good explanation is not discarded by domain validation.
	inner := explain.ExplainResult{
		InputType:        "word",
		DetectedLanguage: "en",
		BriefKo:          "간단",
		DetailedKo:       "설명",
		DomainCategory:   "general",
		Difficulty:       5,
		SubItems:         []explain.SubItem{{SurfaceText: "x", NormalizedKey: "x", ItemType: "word", Importance: 3}},
	}
	innerJSON, err := json.Marshal(inner)
	if err != nil {
		t.Fatalf("marshal inner: %v", err)
	}

	result, err := parseResponse(geminiResponse(innerJSON))
	if err != nil {
		t.Fatalf("parseResponse() error = %v", err)
	}
	if result.Difficulty != 1 {
		t.Errorf("difficulty = %v, want clamped to 1", result.Difficulty)
	}
	if result.SubItems[0].Importance != 1 {
		t.Errorf("importance = %v, want clamped to 1", result.SubItems[0].Importance)
	}
	if err := result.Validate(); err != nil {
		t.Errorf("Validate() after clamp = %v, want nil", err)
	}
}

func TestParseResponseDefaultsDetectedLanguage(t *testing.T) {
	// An empty detected_language (a soft metadata field the prompt never mentions)
	// must not fail validation and discard a good explanation.
	inner := explain.ExplainResult{
		InputType:        "word",
		DetectedLanguage: "",
		BriefKo:          "간단",
		DetailedKo:       "설명",
		DomainCategory:   "general",
		Difficulty:       0.5,
	}
	innerJSON, err := json.Marshal(inner)
	if err != nil {
		t.Fatalf("marshal inner: %v", err)
	}

	result, err := parseResponse(geminiResponse(innerJSON))
	if err != nil {
		t.Fatalf("parseResponse() error = %v", err)
	}
	if result.DetectedLanguage != "und" {
		t.Errorf("detected_language = %q, want defaulted to und", result.DetectedLanguage)
	}
	if err := result.Validate(); err != nil {
		t.Errorf("Validate() after default = %v, want nil", err)
	}
}
