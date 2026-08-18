package ratelimit

import (
	"testing"
	"time"
)

func TestLimiterAllowsUpToLimitThenBlocks(t *testing.T) {
	now := time.Unix(0, 0)
	limiter := newWithClock(2, time.Minute, func() time.Time { return now })

	if !limiter.Allow("key") || !limiter.Allow("key") {
		t.Fatal("first two requests within the window should be allowed")
	}
	if limiter.Allow("key") {
		t.Fatal("third request within the window should be blocked")
	}
}

func TestLimiterResetsAfterWindow(t *testing.T) {
	now := time.Unix(0, 0)
	limiter := newWithClock(1, time.Minute, func() time.Time { return now })

	if !limiter.Allow("key") {
		t.Fatal("first request should be allowed")
	}
	if limiter.Allow("key") {
		t.Fatal("second request in the same window should be blocked")
	}
	now = now.Add(time.Minute + time.Second)
	if !limiter.Allow("key") {
		t.Fatal("request after the window elapses should be allowed")
	}
}

func TestLimiterIsolatesKeys(t *testing.T) {
	now := time.Unix(0, 0)
	limiter := newWithClock(1, time.Minute, func() time.Time { return now })

	if !limiter.Allow("a") || !limiter.Allow("b") {
		t.Fatal("independent keys must not share a budget")
	}
	if limiter.Allow("a") {
		t.Fatal("key a should be exhausted")
	}
}

func TestLimiterDisabledWhenLimitNonPositive(t *testing.T) {
	limiter := New(0, time.Minute)
	for i := 0; i < 100; i++ {
		if !limiter.Allow("key") {
			t.Fatal("a non-positive limit disables limiting")
		}
	}
}
