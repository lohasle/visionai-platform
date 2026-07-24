package visionai

import (
	"testing"
	"time"
)

func TestOutboxBackoffIsBounded(t *testing.T) {
	cases := map[int]time.Duration{
		0:  1 * time.Second,
		1:  1 * time.Second,
		2:  2 * time.Second,
		6:  32 * time.Second,
		20: 32 * time.Second,
	}
	for attempt, want := range cases {
		if got := outboxBackoff(attempt); got != want {
			t.Fatalf("attempt %d: got %s, want %s", attempt, got, want)
		}
	}
}
