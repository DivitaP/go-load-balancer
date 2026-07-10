package balancer

import (
	"testing"

	"github.com/DivitaP/go-load-balancer/internal/backend"
)

func makeBackends(t *testing.T, n int) []*backend.Backend {
	t.Helper()
	out := make([]*backend.Backend, n)
	for i := 0; i < n; i++ {
		b, err := backend.New("http://localhost:808"+string(rune('1'+i)), 1)
		if err != nil {
			t.Fatal(err)
		}
		out[i] = b
	}
	return out
}

func TestRoundRobinEvenDistribution(t *testing.T) {
	backends := makeBackends(t, 3)
	rr := &RoundRobin{}
	counts := map[*backend.Backend]int{}
	for i := 0; i < 300; i++ {
		counts[rr.Next(backends)]++
	}
	for _, b := range backends {
		if counts[b] != 100 {
			t.Errorf("backend %s got %d, want 100", b.URL, counts[b])
		}
	}
}

func TestRoundRobinSkipsDead(t *testing.T) {
	backends := makeBackends(t, 3)
	backends[1].SetAlive(false)
	rr := &RoundRobin{}
	for i := 0; i < 50; i++ {
		if got := rr.Next(backends); got == backends[1] {
			t.Fatal("selected a dead backend")
		}
	}
}

func TestRoundRobinAllDead(t *testing.T) {
	backends := makeBackends(t, 2)
	backends[0].SetAlive(false)
	backends[1].SetAlive(false)
	rr := &RoundRobin{}
	if rr.Next(backends) != nil {
		t.Fatal("expected nil when all backends dead")
	}
}

func TestLeastConnectionsPicksIdle(t *testing.T) {
	backends := makeBackends(t, 3)
	backends[0].IncConns()
	backends[0].IncConns()
	backends[1].IncConns()
	// backends[2] has 0 connections
	lc := &LeastConnections{}
	if got := lc.Next(backends); got != backends[2] {
		t.Errorf("got %s, want idle backend", got.URL)
	}
}

func TestLeastConnectionsSkipsDead(t *testing.T) {
	backends := makeBackends(t, 2)
	backends[0].SetAlive(false) // idle but dead
	backends[1].IncConns()      // busy but alive
	lc := &LeastConnections{}
	if got := lc.Next(backends); got != backends[1] {
		t.Fatal("selected dead backend over alive one")
	}
}

func TestWeightedDistribution(t *testing.T) {
	a, _ := backend.New("http://localhost:8081", 3)
	b, _ := backend.New("http://localhost:8082", 1)
	backends := []*backend.Backend{a, b}
	w := &Weighted{}
	counts := map[*backend.Backend]int{}
	for i := 0; i < 400; i++ {
		counts[w.Next(backends)]++
	}
	if counts[a] != 300 || counts[b] != 100 {
		t.Errorf("got a=%d b=%d, want 300/100", counts[a], counts[b])
	}
}
