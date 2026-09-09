package binlist

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"golang.org/x/time/rate"
)

func TestLookupParsesSuccessfulResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Accept-Version"); got != "3" {
			t.Errorf("Accept-Version = %q, want 3", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"scheme":"visa","type":"debit","country":{"alpha2":"ID"},"bank":{"name":"Test Bank"}}`))
	}))
	defer server.Close()

	client := testClient(server.URL)
	response, err := client.Lookup(context.Background(), "411111")
	if err != nil {
		t.Fatalf("Lookup returned error: %v", err)
	}
	if response.Scheme != "visa" || response.Bank.Name != "Test Bank" {
		t.Fatalf("unexpected response: %+v", response)
	}
}

func TestLookupUsesLocalRateLimit(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := testClient(server.URL)
	client.limiter = rate.NewLimiter(0, 1)
	if _, err := client.Lookup(context.Background(), "411111"); err != nil {
		t.Fatalf("first Lookup returned error: %v", err)
	}
	if _, err := client.Lookup(context.Background(), "555555"); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("second Lookup error = %v, want ErrRateLimited", err)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("upstream calls = %d, want 1", got)
	}
}

func TestLookupOpensCircuitAfterRepeatedFailures(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := testClient(server.URL)
	for range 3 {
		if _, err := client.Lookup(context.Background(), "411111"); err == nil {
			t.Fatal("Lookup unexpectedly succeeded")
		}
	}
	if _, err := client.Lookup(context.Background(), "411111"); !errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("fourth Lookup error = %v, want ErrCircuitOpen", err)
	}
	if got := calls.Load(); got != 3 {
		t.Fatalf("upstream calls = %d, want 3", got)
	}
}

func testClient(serverURL string) *Client {
	client := New()
	client.baseURL = serverURL
	client.limiter = rate.NewLimiter(rate.Inf, 100)
	return client
}
