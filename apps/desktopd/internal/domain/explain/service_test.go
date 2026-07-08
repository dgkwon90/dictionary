package explain

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type fakeExplainer struct {
	explain func(context.Context, string) (ExplainResult, string, error)
}

func (f fakeExplainer) Explain(ctx context.Context, text string) (ExplainResult, string, error) {
	return f.explain(ctx, text)
}

type fakeRepository struct {
	markRunning func(context.Context, string, time.Time) error
	saveSuccess func(context.Context, string, string, ExplainResult, string, time.Time) error
	saveFailure func(context.Context, string, string, time.Time) error
}

func (f fakeRepository) MarkRunning(ctx context.Context, jobID string, startedAt time.Time) error {
	return f.markRunning(ctx, jobID, startedAt)
}

func (f fakeRepository) SaveSuccess(ctx context.Context, jobID, captureID string, result ExplainResult, rawResponseJSON string, finishedAt time.Time) error {
	return f.saveSuccess(ctx, jobID, captureID, result, rawResponseJSON, finishedAt)
}

func (f fakeRepository) SaveFailure(ctx context.Context, jobID string, errMessage string, finishedAt time.Time) error {
	return f.saveFailure(ctx, jobID, errMessage, finishedAt)
}

func TestServiceProcessSuccess(t *testing.T) {
	startedAt := time.Date(2026, 7, 7, 1, 0, 0, 0, time.FixedZone("KST", 9*60*60))
	finishedAt := time.Date(2026, 7, 7, 1, 0, 1, 0, time.FixedZone("KST", 9*60*60))
	result := validExplainResult()
	rawResponseJSON := `{"provider":"raw"}`
	var markRunningCalled bool
	var explainCalled bool
	var saveSuccessCalled bool
	service := NewService(fakeExplainer{explain: func(_ context.Context, text string) (ExplainResult, string, error) {
		explainCalled = true
		if text != "hello" {
			t.Fatalf("text = %q, want hello", text)
		}
		return result, rawResponseJSON, nil
	}}, fakeRepository{
		markRunning: func(_ context.Context, jobID string, gotStartedAt time.Time) error {
			markRunningCalled = true
			if jobID != "job-1" || !gotStartedAt.Equal(startedAt.UTC()) || gotStartedAt.Location() != time.UTC {
				t.Fatalf("MarkRunning(%q, %v)", jobID, gotStartedAt)
			}
			return nil
		},
		saveSuccess: func(_ context.Context, jobID, captureID string, gotResult ExplainResult, rawResponseJSON string, gotFinishedAt time.Time) error {
			saveSuccessCalled = true
			if jobID != "job-1" || captureID != "capture-1" || gotResult.BriefKo != result.BriefKo {
				t.Fatalf("SaveSuccess(%q, %q, %#v)", jobID, captureID, gotResult)
			}
			if !gotFinishedAt.Equal(finishedAt.UTC()) || gotFinishedAt.Location() != time.UTC {
				t.Fatalf("finishedAt = %v", gotFinishedAt)
			}
			if rawResponseJSON != `{"provider":"raw"}` {
				t.Fatalf("rawResponseJSON = %q, want provider raw", rawResponseJSON)
			}
			return nil
		},
		saveFailure: func(context.Context, string, string, time.Time) error {
			t.Fatal("SaveFailure should not be called")
			return nil
		},
	})
	calls := 0
	service.now = func() time.Time {
		calls++
		if calls == 1 {
			return startedAt
		}
		return finishedAt
	}

	if err := service.Process(context.Background(), "job-1", "capture-1", "hello"); err != nil {
		t.Fatalf("Process() error = %v", err)
	}
	if !markRunningCalled || !explainCalled || !saveSuccessCalled {
		t.Fatalf("calls markRunning=%t explain=%t saveSuccess=%t", markRunningCalled, explainCalled, saveSuccessCalled)
	}
}

func TestServiceProcessExplainerError(t *testing.T) {
	explainErr := errors.New("provider failed")
	var saveFailureCalled bool
	service := NewService(fakeExplainer{explain: func(context.Context, string) (ExplainResult, string, error) {
		return ExplainResult{}, "", explainErr
	}}, fakeRepository{
		markRunning: func(context.Context, string, time.Time) error { return nil },
		saveSuccess: func(context.Context, string, string, ExplainResult, string, time.Time) error {
			t.Fatal("SaveSuccess should not be called")
			return nil
		},
		saveFailure: func(_ context.Context, jobID string, errMessage string, _ time.Time) error {
			saveFailureCalled = true
			if jobID != "job-1" || !strings.Contains(errMessage, "provider failed") {
				t.Fatalf("SaveFailure(%q, %q)", jobID, errMessage)
			}
			return nil
		},
	})

	err := service.Process(context.Background(), "job-1", "capture-1", "hello")
	if !errors.Is(err, explainErr) {
		t.Fatalf("Process() error = %v, want provider error", err)
	}
	if !saveFailureCalled {
		t.Fatal("SaveFailure was not called")
	}
}

func TestServiceProcessInvalidResult(t *testing.T) {
	result := validExplainResult()
	result.DomainCategory = "bogus"
	var saveFailureCalled bool
	service := NewService(fakeExplainer{explain: func(context.Context, string) (ExplainResult, string, error) {
		return result, `{"provider":"raw"}`, nil
	}}, fakeRepository{
		markRunning: func(context.Context, string, time.Time) error { return nil },
		saveSuccess: func(context.Context, string, string, ExplainResult, string, time.Time) error {
			t.Fatal("SaveSuccess should not be called")
			return nil
		},
		saveFailure: func(_ context.Context, _ string, errMessage string, _ time.Time) error {
			saveFailureCalled = true
			if !strings.Contains(errMessage, "domain_category") {
				t.Fatalf("errMessage = %q, want domain_category", errMessage)
			}
			return nil
		},
	})

	err := service.Process(context.Background(), "job-1", "capture-1", "hello")
	if !errors.Is(err, ErrInvalidResult) {
		t.Fatalf("Process() error = %v, want ErrInvalidResult", err)
	}
	if !saveFailureCalled {
		t.Fatal("SaveFailure was not called")
	}
}

func TestServiceProcessSaveSuccessErrorMarksFailure(t *testing.T) {
	result := validExplainResult()
	saveSuccessErr := errors.New("insert explanation failed")
	var saveFailureCalled bool
	service := NewService(fakeExplainer{explain: func(context.Context, string) (ExplainResult, string, error) {
		return result, `{"provider":"raw"}`, nil
	}}, fakeRepository{
		markRunning: func(context.Context, string, time.Time) error { return nil },
		saveSuccess: func(context.Context, string, string, ExplainResult, string, time.Time) error {
			return saveSuccessErr
		},
		saveFailure: func(_ context.Context, jobID string, errMessage string, _ time.Time) error {
			saveFailureCalled = true
			if jobID != "job-1" || !strings.Contains(errMessage, "insert explanation failed") {
				t.Fatalf("SaveFailure(%q, %q)", jobID, errMessage)
			}
			return nil
		},
	})

	err := service.Process(context.Background(), "job-1", "capture-1", "hello")
	if !errors.Is(err, saveSuccessErr) {
		t.Fatalf("Process() error = %v, want save success error", err)
	}
	if !saveFailureCalled {
		t.Fatal("SaveFailure was not called")
	}
}

func TestServiceProcessMarkRunningError(t *testing.T) {
	markErr := errors.New("update failed")
	service := NewService(fakeExplainer{explain: func(context.Context, string) (ExplainResult, string, error) {
		t.Fatal("Explain should not be called")
		return ExplainResult{}, "", nil
	}}, fakeRepository{
		markRunning: func(context.Context, string, time.Time) error { return markErr },
		saveSuccess: func(context.Context, string, string, ExplainResult, string, time.Time) error {
			t.Fatal("SaveSuccess should not be called")
			return nil
		},
		saveFailure: func(context.Context, string, string, time.Time) error {
			t.Fatal("SaveFailure should not be called")
			return nil
		},
	})

	err := service.Process(context.Background(), "job-1", "capture-1", "hello")
	if !errors.Is(err, markErr) {
		t.Fatalf("Process() error = %v, want mark error", err)
	}
}
