package web

import (
	"net/http"
	"testing"
	"time"
)

func TestEvaluateLiveness(t *testing.T) {
	grace := 4 * time.Minute
	base := time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)

	t.Run("healthy returns 200 and clears the timer", func(t *testing.T) {
		stuck := base.Add(-10 * time.Minute)
		code, since, _ := evaluateLiveness(true, &stuck, base, grace)
		if code != http.StatusOK {
			t.Fatalf("got %d, want 200", code)
		}
		if since != nil {
			t.Fatalf("expected unhealthy-since cleared, got %v", since)
		}
	})

	t.Run("first unhealthy observation starts the timer and stays 200", func(t *testing.T) {
		code, since, stuckFor := evaluateLiveness(false, nil, base, grace)
		if code != http.StatusOK {
			t.Fatalf("got %d, want 200 within grace", code)
		}
		if since == nil || !since.Equal(base) {
			t.Fatalf("expected unhealthy-since=%v, got %v", base, since)
		}
		if stuckFor != 0 {
			t.Fatalf("expected stuckFor=0, got %v", stuckFor)
		}
	})

	t.Run("brief disconnect within grace stays 200", func(t *testing.T) {
		since := base
		now := base.Add(2 * time.Minute)
		code, _, stuckFor := evaluateLiveness(false, &since, now, grace)
		if code != http.StatusOK {
			t.Fatalf("got %d, want 200 within grace", code)
		}
		if stuckFor != 2*time.Minute {
			t.Fatalf("expected stuckFor=2m, got %v", stuckFor)
		}
	})

	t.Run("stuck beyond grace returns 503", func(t *testing.T) {
		since := base
		now := base.Add(5 * time.Minute)
		code, _, _ := evaluateLiveness(false, &since, now, grace)
		if code != http.StatusServiceUnavailable {
			t.Fatalf("got %d, want 503", code)
		}
	})
}
