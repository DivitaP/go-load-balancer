package backend

import (
	"sync"
	"testing"
	"time"
)

func TestEMAFirstSampleIsExact(t *testing.T) {
	b, _ := New("http://localhost:8081", 1)
	b.RecordLatency(100 * time.Millisecond)
	if b.AvgLatency() != 100*time.Millisecond {
		t.Errorf("got %v, want 100ms", b.AvgLatency())
	}
}

func TestEMAFormula(t *testing.T) {
	b, _ := New("http://localhost:8081", 1)
	b.RecordLatency(100 * time.Millisecond)
	b.RecordLatency(200 * time.Millisecond)
	// 0.2*200 + 0.8*100 = 120ms
	want := 120 * time.Millisecond
	got := b.AvgLatency()
	if got < want-time.Millisecond || got > want+time.Millisecond {
		t.Errorf("got %v, want ~%v", got, want)
	}
}

func TestFailureThreshold(t *testing.T) {
	b, _ := New("http://localhost:8081", 1)

	if transitioned := b.RecordFailure(3); transitioned || !b.IsAlive() {
		t.Fatal("should still be alive after 1 failure")
	}
	b.RecordFailure(3)
	if !b.IsAlive() {
		t.Fatal("should still be alive after 2 failures")
	}
	if transitioned := b.RecordFailure(3); !transitioned || b.IsAlive() {
		t.Fatal("should be dead after 3 failures")
	}

	b.RecordSuccess()
	if !b.IsAlive() {
		t.Fatal("should be alive after success")
	}
	// Failure count must have reset: two failures should not kill it.
	b.RecordFailure(3)
	b.RecordFailure(3)
	if !b.IsAlive() {
		t.Fatal("fail count did not reset on success")
	}
}

// Run with: go test -race ./...
func TestConcurrentStateAccess(t *testing.T) {
	b, _ := New("http://localhost:8081", 1)
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(3)
		go func() { defer wg.Done(); b.RecordLatency(10 * time.Millisecond) }()
		go func() { defer wg.Done(); _ = b.IsAlive(); _ = b.AvgLatency() }()
		go func() { defer wg.Done(); b.IncConns(); b.DecConns() }()
	}
	wg.Wait()
	if b.ActiveConns() != 0 {
		t.Errorf("conns leaked: %d", b.ActiveConns())
	}
}
