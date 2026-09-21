package cache

import (
	"path/filepath"
	"sync"
	"testing"
)

func newTestCache(t *testing.T, ttlSeconds int) *Cache {
	t.Helper()
	path := filepath.Join(t.TempDir(), "cache.db")
	c, err := New(path, ttlSeconds)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func TestSetThenGetRoundTrips(t *testing.T) {
	c := newTestCache(t, 300)

	type payload struct {
		Name string `json:"name"`
	}
	if err := c.Set("k1", payload{Name: "octocat"}); err != nil {
		t.Fatalf("Set: %v", err)
	}

	var got payload
	if !c.Get("k1", &got) {
		t.Fatal("Get: expected hit")
	}
	if got.Name != "octocat" {
		t.Errorf("got Name=%q, want %q", got.Name, "octocat")
	}
}

func TestGetMissingKeyReturnsFalse(t *testing.T) {
	c := newTestCache(t, 300)
	var dst map[string]any
	if c.Get("missing", &dst) {
		t.Error("Get: expected miss for absent key")
	}
}

func TestGetExpiredEntry(t *testing.T) {
	// A 1-second-negative-equivalent expiry is simulated by writing directly
	// with an already-past expires_at, since Cache has no injectable clock.
	c := newTestCache(t, 300)
	if err := c.Set("k1", "v1"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	// Force the row's expires_at into the past.
	if _, err := c.db.Exec("UPDATE cache SET expires_at = 0 WHERE key = ?", "k1"); err != nil {
		t.Fatalf("force-expire: %v", err)
	}

	var dst string
	if c.Get("k1", &dst) {
		t.Error("Get: expected miss for expired entry")
	}

	// The expired row should have been deleted as a side effect.
	var count int
	if err := c.db.QueryRow("SELECT COUNT(*) FROM cache WHERE key = ?", "k1").Scan(&count); err != nil {
		t.Fatalf("count query: %v", err)
	}
	if count != 0 {
		t.Errorf("expected expired row to be deleted, found %d rows", count)
	}
}

func TestSetOverwritesExistingKey(t *testing.T) {
	c := newTestCache(t, 300)
	if err := c.Set("k1", "first"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if err := c.Set("k1", "second"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	var got string
	if !c.Get("k1", &got) {
		t.Fatal("Get: expected hit")
	}
	if got != "second" {
		t.Errorf("got %q, want %q", got, "second")
	}
}

// TestConcurrentGetSet exercises Cache's mutex under -race: many goroutines
// hammer the same and different keys simultaneously (Issue #127).
func TestConcurrentGetSet(t *testing.T) {
	c := newTestCache(t, 300)
	const goroutines = 10
	const iterations = 20

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func(id int) {
			defer wg.Done()
			key := "shared-key"
			for i := 0; i < iterations; i++ {
				if err := c.Set(key, id*1000+i); err != nil {
					t.Errorf("Set: %v", err)
					return
				}
				var dst int
				c.Get(key, &dst) // value is racy by design (shared key); only checking no crash/data race
			}
		}(g)
	}
	wg.Wait()
}
