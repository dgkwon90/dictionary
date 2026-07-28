package explain

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// formatRecordingExplainer captures the Format a lookup was actually made with, which
// is the only thing that proves a saved style reached the provider.
type formatRecordingExplainer struct {
	got    Format
	result ExplainResult
}

func (f *formatRecordingExplainer) Explain(_ context.Context, _ string, format Format) (ExplainResult, string, error) {
	f.got = format
	return f.result, "{}", nil
}

type stubFormatSource struct {
	format Format
	err    error
	calls  int
}

func (s *stubFormatSource) ExplainFormat(context.Context) (Format, error) {
	s.calls++
	return s.format, s.err
}

func okRepository() fakeRepository {
	return fakeRepository{
		markRunning: func(context.Context, string, time.Time) error { return nil },
		saveSuccess: func(context.Context, string, string, ExplainResult, string, time.Time) error { return nil },
		saveFailure: func(context.Context, string, string, time.Time) error { return nil },
	}
}

func TestServiceProcessUsesSavedFormat(t *testing.T) {
	want := Format{PromptStyle: "짧게 설명해줘", ExamplesCount: 1}
	explainer := &formatRecordingExplainer{result: validExplainResult()}
	source := &stubFormatSource{format: want}
	service := NewService(explainer, okRepository(), source)

	if err := service.Process(context.Background(), "job-1", "cap-1", "hello"); err != nil {
		t.Fatalf("Process() error = %v", err)
	}
	if explainer.got != want {
		t.Fatalf("provider asked with %+v, want the saved %+v", explainer.got, want)
	}
}

// The style is read per lookup, so changing it in Settings applies to the next word
// the user looks up rather than the next launch.
func TestServiceProcessReloadsFormatEachLookup(t *testing.T) {
	source := &stubFormatSource{format: DefaultFormat()}
	service := NewService(&formatRecordingExplainer{result: validExplainResult()}, okRepository(), source)

	for i := 0; i < 3; i++ {
		if err := service.Process(context.Background(), "job-1", "cap-1", "hello"); err != nil {
			t.Fatalf("Process() error = %v", err)
		}
	}
	if source.calls != 3 {
		t.Fatalf("format source called %d times, want 3", source.calls)
	}
}

func TestNewServiceNilFormatSourceUsesDefault(t *testing.T) {
	explainer := &formatRecordingExplainer{result: validExplainResult()}
	service := NewService(explainer, okRepository(), nil)

	if err := service.Process(context.Background(), "job-1", "cap-1", "hello"); err != nil {
		t.Fatalf("Process() error = %v", err)
	}
	if explainer.got != DefaultFormat() {
		t.Fatalf("provider asked with %+v, want the default format", explainer.got)
	}
}

// If the format cannot be read the job is recorded as failed rather than left running:
// the settings row is in the same local database the result would be written to, so a
// silent retry-forever state would tell the user nothing.
func TestServiceProcessRecordsFailureWhenFormatCannotBeLoaded(t *testing.T) {
	var failureMessage string
	repo := okRepository()
	repo.saveSuccess = func(context.Context, string, string, ExplainResult, string, time.Time) error {
		t.Fatal("SaveSuccess must not be called when the format could not be read")
		return nil
	}
	repo.saveFailure = func(_ context.Context, _ string, message string, _ time.Time) error {
		failureMessage = message
		return nil
	}
	explainer := &formatRecordingExplainer{result: validExplainResult()}
	service := NewService(explainer, repo, &stubFormatSource{err: errors.New("settings unavailable")})

	err := service.Process(context.Background(), "job-1", "cap-1", "hello")
	if err == nil {
		t.Fatal("Process() error = nil, want the load failure surfaced")
	}
	if !strings.Contains(failureMessage, "settings unavailable") {
		t.Errorf("recorded failure = %q, want it to name the cause", failureMessage)
	}
	if explainer.got != (Format{}) {
		t.Error("the provider must not be called on a format nobody could read")
	}
}
