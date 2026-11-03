package main

import (
	"errors"
	"testing"
	"time"
)

func TestWaitForHealth(t *testing.T) {
	calls := 0
	h, err := waitForHealth(func() (healthInfo, error) {
		calls++
		if calls < 3 {
			return healthInfo{}, errors.New("connection refused")
		}
		return healthInfo{Status: "ok", Version: "2026.11.1"}, nil
	}, 5, time.Millisecond)
	if err != nil || h.Version != "2026.11.1" || calls != 3 {
		t.Fatalf("got %+v, %v after %d calls", h, err, calls)
	}

	_, err = waitForHealth(func() (healthInfo, error) { return healthInfo{Status: "starting"}, nil }, 3, time.Millisecond)
	if err == nil {
		t.Fatal("non-ok status accepted")
	}
}

func TestDescribeVersion(t *testing.T) {
	if got := describeVersion(healthInfo{Version: "1.2"}); got != "v1.2" {
		t.Errorf("got %q", got)
	}
	if got := describeVersion(healthInfo{Version: "1.2", Build: "7"}); got != "v1.2 (build 7)" {
		t.Errorf("got %q", got)
	}
}
