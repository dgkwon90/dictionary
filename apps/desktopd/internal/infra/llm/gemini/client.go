package gemini

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"neulsang/desktopd/internal/domain/explain"
	"neulsang/desktopd/internal/domain/suggest"
)

const (
	DefaultBaseURL = "https://generativelanguage.googleapis.com"
	// DefaultModel favors the most generous free tier: Flash-Lite has the highest
	// free RPM/RPD and lowest token cost, and is enough for short term/word
	// explanations. Override with NEULSANG_GEMINI_MODEL (e.g. gemini-flash-latest)
	// for higher-quality explanations. "-latest" is a rolling alias to the current
	// stable Flash-Lite.
	DefaultModel = "gemini-flash-lite-latest"
)

const (
	defaultTimeout = 20 * time.Second
	maxRetries     = 2
	retryBaseDelay = 300 * time.Millisecond
)

type Client struct {
	apiKey     string
	model      string
	baseURL    string
	httpClient *http.Client
	timeout    time.Duration
}

type Option func(*Client)

func WithModel(model string) Option {
	return func(c *Client) {
		if model != "" {
			c.model = model
		}
	}
}

func WithBaseURL(baseURL string) Option {
	return func(c *Client) {
		if baseURL != "" {
			c.baseURL = baseURL
		}
	}
}

func WithHTTPClient(httpClient *http.Client) Option {
	return func(c *Client) {
		if httpClient != nil {
			c.httpClient = httpClient
		}
	}
}

func WithTimeout(timeout time.Duration) Option {
	return func(c *Client) {
		if timeout > 0 {
			c.timeout = timeout
		}
	}
}

func New(apiKey string, opts ...Option) *Client {
	client := &Client{
		apiKey:     apiKey,
		model:      DefaultModel,
		baseURL:    DefaultBaseURL,
		httpClient: http.DefaultClient,
		timeout:    defaultTimeout,
	}
	for _, opt := range opts {
		opt(client)
	}
	client.baseURL = strings.TrimRight(client.baseURL, "/")
	return client
}

func (c *Client) Explain(ctx context.Context, text string) (explain.ExplainResult, string, error) {
	rawResponseBody, err := c.generate(ctx, buildPrompt(text), responseSchema())
	if err != nil {
		return explain.ExplainResult{}, "", err
	}
	result, err := parseResponse(rawResponseBody)
	if err != nil {
		return explain.ExplainResult{}, "", err
	}
	return result, rawResponseBody, nil
}

// Suggest infers English candidates from a Korean phonetic query (backlog #21).
func (c *Client) Suggest(ctx context.Context, query string) ([]suggest.Candidate, error) {
	rawResponseBody, err := c.generate(ctx, buildSuggestPrompt(query), suggestResponseSchema())
	if err != nil {
		return nil, err
	}
	return parseSuggestResponse(rawResponseBody)
}

// generate performs one structured-output generateContent call with retry/backoff
// and returns the raw response body. Callers parse it into their own result type.
func (c *Client) generate(ctx context.Context, promptText string, schema map[string]any) (string, error) {
	if c.apiKey == "" {
		return "", errors.New("gemini: API key is required")
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}

	body, err := json.Marshal(generateContentRequest{
		Contents: []content{{
			Parts: []part{{Text: promptText}},
		}},
		GenerationConfig: generationConfig{
			ResponseMimeType: "application/json",
			ResponseSchema:   schema,
		},
	})
	if err != nil {
		return "", fmt.Errorf("gemini: marshal request: %w", err)
	}

	endpoint := fmt.Sprintf("%s/v1beta/models/%s:generateContent", c.baseURL, c.model)
	attempts := maxRetries + 1
	var lastErr error
	for attempt := 0; attempt < attempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return "", err
		}

		attemptCtx, cancel := context.WithTimeout(ctx, c.timeout)
		rawResponseBody, retryable, err := c.postGenerateContent(attemptCtx, endpoint, body)
		cancel()
		if err == nil {
			return rawResponseBody, nil
		}
		if err := ctx.Err(); err != nil {
			return "", err
		}
		lastErr = err
		if !retryable {
			return "", err
		}
		if attempt == attempts-1 {
			return "", fmt.Errorf("gemini generateContent failed after %d attempts: %w", attempts, lastErr)
		}
		if err := waitRetry(ctx, retryBaseDelay*time.Duration(1<<attempt)); err != nil {
			return "", err
		}
	}

	return "", fmt.Errorf("gemini generateContent failed after %d attempts: %w", attempts, lastErr)
}

func (c *Client) postGenerateContent(ctx context.Context, endpoint string, body []byte) (rawBody string, retryable bool, resultErr error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", false, fmt.Errorf("gemini: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-goog-api-key", c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", true, err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			resultErr = errors.Join(resultErr, fmt.Errorf("gemini: close response body: %w", err))
		}
	}()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		snippet := readSnippet(resp.Body)
		err := fmt.Errorf("gemini generateContent status %d: %s", resp.StatusCode, snippet)
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= http.StatusInternalServerError {
			return "", true, err
		}
		return "", false, err
	}

	rawResponseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", true, fmt.Errorf("gemini: read response body: %w", err)
	}
	return string(rawResponseBody), false, nil
}

func parseResponse(rawResponseBody string) (explain.ExplainResult, error) {
	var response generateContentResponse
	if err := json.Unmarshal([]byte(rawResponseBody), &response); err != nil {
		return explain.ExplainResult{}, fmt.Errorf("gemini: parse response: %w; response prefix: %q", err, truncate(rawResponseBody, 512))
	}
	if len(response.Candidates) == 0 || len(response.Candidates[0].Content.Parts) == 0 {
		return explain.ExplainResult{}, errors.New("gemini: empty response")
	}

	var result explain.ExplainResult
	if err := json.Unmarshal([]byte(response.Candidates[0].Content.Parts[0].Text), &result); err != nil {
		return explain.ExplainResult{}, fmt.Errorf("gemini: parse structured output: %w; response prefix: %q", err, truncate(rawResponseBody, 512))
	}

	// Normalize soft fields the model does not reliably honor even with a response
	// schema, so a good explanation is not discarded by domain validation over a
	// minor metadata slip. Core content (brief_ko/detailed_ko) is left strict: if the
	// model returns those empty the explanation is genuinely useless and should fail.
	result.Difficulty = clamp01(result.Difficulty)
	for i := range result.SubItems {
		result.SubItems[i].Importance = clamp01(result.SubItems[i].Importance)
	}
	if result.DetectedLanguage == "" {
		result.DetectedLanguage = "und" // ISO 639-2 "undetermined"
	}
	return result, nil
}

func parseSuggestResponse(rawResponseBody string) ([]suggest.Candidate, error) {
	var response generateContentResponse
	if err := json.Unmarshal([]byte(rawResponseBody), &response); err != nil {
		return nil, fmt.Errorf("gemini: parse response: %w; response prefix: %q", err, truncate(rawResponseBody, 512))
	}
	if len(response.Candidates) == 0 || len(response.Candidates[0].Content.Parts) == 0 {
		return nil, errors.New("gemini: empty response")
	}

	var parsed struct {
		Candidates []suggest.Candidate `json:"candidates"`
	}
	if err := json.Unmarshal([]byte(response.Candidates[0].Content.Parts[0].Text), &parsed); err != nil {
		return nil, fmt.Errorf("gemini: parse structured output: %w; response prefix: %q", err, truncate(rawResponseBody, 512))
	}

	out := make([]suggest.Candidate, 0, len(parsed.Candidates))
	for _, candidate := range parsed.Candidates {
		if strings.TrimSpace(candidate.English) == "" {
			continue // drop empty/hallucinated blanks
		}
		candidate.Confidence = clamp01(candidate.Confidence)
		candidate.Source = suggest.SourceAI
		out = append(out, candidate)
	}
	return out, nil
}

func clamp01(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}

func waitRetry(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func readSnippet(reader io.Reader) string {
	data, err := io.ReadAll(io.LimitReader(reader, 4096))
	if err != nil {
		return fmt.Sprintf("read response body: %v", err)
	}
	return string(data)
}

func truncate(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit]) + "..."
}

type generateContentRequest struct {
	Contents         []content        `json:"contents"`
	GenerationConfig generationConfig `json:"generationConfig"`
}

type generationConfig struct {
	ResponseMimeType string         `json:"responseMimeType"`
	ResponseSchema   map[string]any `json:"responseSchema"`
}

type content struct {
	Parts []part `json:"parts"`
}

type part struct {
	Text string `json:"text"`
}

type generateContentResponse struct {
	Candidates []candidate `json:"candidates"`
}

type candidate struct {
	Content content `json:"content"`
}
