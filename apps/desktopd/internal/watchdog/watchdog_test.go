package watchdog

import (
	"context"
	"io"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestParentGone(t *testing.T) {
	if parentGone(1234, func() int { return 1234 }) {
		t.Fatal("parentGone = true while ppid unchanged, want false")
	}
	if !parentGone(1234, func() int { return 1 }) {
		t.Fatal("parentGone = false after reparent to init, want true")
	}
}

func TestWatchParentDisabledWhenEnvUnset(t *testing.T) {
	t.Setenv(ParentPIDEnv, "")
	ctx := context.Background()
	got := watchParent(ctx, discardLogger(), func() int { return 1 }, time.Millisecond)

	// 비활성이면 입력 컨텍스트를 그대로 반환하고 절대 취소되지 않는다.
	if got != ctx {
		t.Fatal("watchParent returned a derived context while disabled")
	}
	select {
	case <-got.Done():
		t.Fatal("disabled watchdog cancelled the context")
	case <-time.After(20 * time.Millisecond):
	}
}

func TestWatchParentCancelsOnReparent(t *testing.T) {
	t.Setenv(ParentPIDEnv, "4321")

	// 최초 getppid=4321, 이후 재입양되어 1을 반환.
	var reparented atomic.Bool
	getppid := func() int {
		if reparented.Load() {
			return 1
		}
		return 4321
	}

	wctx := watchParent(context.Background(), discardLogger(), getppid, time.Millisecond)
	reparented.Store(true)

	select {
	case <-wctx.Done():
	case <-time.After(time.Second):
		t.Fatal("watchdog did not cancel after parent reparented")
	}
}

func TestWatchParentPropagatesParentCancel(t *testing.T) {
	t.Setenv(ParentPIDEnv, "4321")
	parent, cancel := context.WithCancel(context.Background())

	wctx := watchParent(parent, discardLogger(), func() int { return 4321 }, time.Millisecond)
	cancel()

	select {
	case <-wctx.Done():
	case <-time.After(time.Second):
		t.Fatal("watchdog did not propagate parent cancellation")
	}
}

func TestParentPIDFromEnv(t *testing.T) {
	cases := []struct {
		raw    string
		wantOK bool
	}{
		{"", false},
		{"0", false},
		{"-3", false},
		{"abc", false},
		{"4321", true},
	}
	for _, c := range cases {
		t.Setenv(ParentPIDEnv, c.raw)
		if _, ok := parentPIDFromEnv(); ok != c.wantOK {
			t.Errorf("parentPIDFromEnv(%q) ok = %v, want %v", c.raw, ok, c.wantOK)
		}
	}
}
