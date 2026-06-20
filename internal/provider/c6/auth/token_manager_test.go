package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
)

func TestTokenManager_CachesToken(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		json.NewEncoder(w).Encode(map[string]any{
			"access_token": "tok-1",
			"expires_in":   300,
			"token_type":   "Bearer",
		})
	}))
	defer server.Close()

	tm := New(server.Client(), server.URL, "id", "secret")

	for i := 0; i < 5; i++ {
		tok, err := tm.GetToken(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if tok != "tok-1" {
			t.Fatalf("got %q, want tok-1", tok)
		}
	}

	if got := calls.Load(); got != 1 {
		t.Fatalf("expected exactly 1 auth call due to caching, got %d", got)
	}
}

func TestTokenManager_ConcurrentRefreshIsSerialized(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		json.NewEncoder(w).Encode(map[string]any{
			"access_token": "tok-concurrent",
			"expires_in":   300,
			"token_type":   "Bearer",
		})
	}))
	defer server.Close()

	tm := New(server.Client(), server.URL, "id", "secret")

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := tm.GetToken(context.Background()); err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		}()
	}
	wg.Wait()

	if got := calls.Load(); got != 1 {
		t.Fatalf("expected exactly 1 auth call across concurrent callers, got %d", got)
	}
}

func TestTokenManager_ForceRefreshFetchesNewToken(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		json.NewEncoder(w).Encode(map[string]any{
			"access_token": map[int32]string{1: "tok-a", 2: "tok-b"}[minInt32(n, 2)],
			"expires_in":   300,
			"token_type":   "Bearer",
		})
	}))
	defer server.Close()

	tm := New(server.Client(), server.URL, "id", "secret")

	first, err := tm.GetToken(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if first != "tok-a" {
		t.Fatalf("got %q, want tok-a", first)
	}

	second, err := tm.ForceRefresh(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if second != "tok-b" {
		t.Fatalf("got %q, want tok-b", second)
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("expected 2 auth calls, got %d", got)
	}
}

func minInt32(a, b int32) int32 {
	if a < b {
		return a
	}
	return b
}
