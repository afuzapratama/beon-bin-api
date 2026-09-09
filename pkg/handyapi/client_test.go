package handyapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"golang.org/x/time/rate"
)

func TestLookupParsesResponseAndSendsKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("x-api-key"); got != "secret-key" {
			t.Errorf("x-api-key = %q, want configured key", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"Status":"SUCCESS","Scheme":"MASTERCARD","Type":"CREDIT","Issuer":"Test Bank","CardTier":"PLATINUM","Country":{"A2":"ID","Name":"Indonesia"},"Luhn":true}`))
	}))
	defer server.Close()

	client := testClient(server.URL)
	result, err := client.Lookup(context.Background(), "535316")
	if err != nil {
		t.Fatalf("Lookup returned error: %v", err)
	}
	if result.Scheme != "MASTERCARD" || result.Issuer != "Test Bank" || result.Country.Alpha2 != "ID" {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestLookupAcceptsCountryArrayShapes(t *testing.T) {
	tests := []struct {
		name        string
		countryJSON string
		wantCode    string
	}{
		{name: "empty array", countryJSON: `[]`},
		{name: "object in array", countryJSON: `[{"A2":"SG","Name":"Singapore"}]`, wantCode: "SG"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(`{"Status":"SUCCESS","Scheme":"MASTERCARD","Country":` + tt.countryJSON + `}`))
			}))
			defer server.Close()

			result, err := testClient(server.URL).Lookup(context.Background(), "22217000")
			if err != nil {
				t.Fatalf("Lookup returned error: %v", err)
			}
			if result.Country.Alpha2 != tt.wantCode {
				t.Fatalf("country code = %q, want %q", result.Country.Alpha2, tt.wantCode)
			}
		})
	}
}

func TestLookupHandlesNotFoundAndRateLimit(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		wantNil    bool
		wantErr    error
	}{
		{name: "not found", statusCode: http.StatusNotFound, wantNil: true},
		{name: "rate limited", statusCode: http.StatusTooManyRequests, wantErr: ErrRateLimited},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.statusCode)
			}))
			defer server.Close()
			result, err := testClient(server.URL).Lookup(context.Background(), "535316")
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			}
			if tt.wantNil && result != nil {
				t.Fatalf("result = %+v, want nil", result)
			}
		})
	}
}

func TestLookupUsesLocalRateLimit(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		_, _ = w.Write([]byte(`{"Status":"SUCCESS","Scheme":"VISA"}`))
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
	client := New("secret-key")
	client.baseURL = serverURL
	client.limiter = rate.NewLimiter(rate.Inf, 100)
	return client
}
