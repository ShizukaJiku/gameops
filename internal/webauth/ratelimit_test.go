package webauth

import (
	"testing"
	"time"
)

func TestRateLimiterAllowsUpToMax(t *testing.T) {
	rl := NewRateLimiter(3, time.Minute)
	for i := 0; i < 3; i++ {
		if !rl.Allow("1.2.3.4") {
			t.Fatalf("expected attempt %d to be allowed", i+1)
		}
	}
}

func TestRateLimiterBlocksAfterMax(t *testing.T) {
	rl := NewRateLimiter(3, time.Minute)
	for i := 0; i < 3; i++ {
		rl.Allow("1.2.3.4")
	}
	if rl.Allow("1.2.3.4") {
		t.Fatal("expected the 4th attempt within the window to be blocked")
	}
}

func TestRateLimiterTracksKeysIndependently(t *testing.T) {
	rl := NewRateLimiter(1, time.Minute)
	if !rl.Allow("1.2.3.4") {
		t.Fatal("expected first attempt from 1.2.3.4 to be allowed")
	}
	if !rl.Allow("5.6.7.8") {
		t.Fatal("expected first attempt from a different key to be allowed independently")
	}
	if rl.Allow("1.2.3.4") {
		t.Fatal("expected second attempt from 1.2.3.4 to be blocked")
	}
}

func TestRateLimiterAllowsAgainAfterWindowPasses(t *testing.T) {
	rl := NewRateLimiter(1, 50*time.Millisecond)
	if !rl.Allow("1.2.3.4") {
		t.Fatal("expected first attempt to be allowed")
	}
	if rl.Allow("1.2.3.4") {
		t.Fatal("expected second immediate attempt to be blocked")
	}
	time.Sleep(70 * time.Millisecond)
	if !rl.Allow("1.2.3.4") {
		t.Fatal("expected attempt after the window passed to be allowed again")
	}
}
