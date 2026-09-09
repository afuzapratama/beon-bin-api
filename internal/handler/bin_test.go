package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/beon/bin-api/internal/middleware"
	"github.com/beon/bin-api/internal/model"
	"github.com/beon/bin-api/internal/service"
	"github.com/gin-gonic/gin"
)

type fakeBINService struct {
	lookupResult *model.BINResponse
	lookupErr    error
	lookupCalls  int
	lastBIN      string
	count        int64
	countErr     error
}

func (s *fakeBINService) Lookup(_ context.Context, bin string) (*model.BINResponse, error) {
	s.lookupCalls++
	s.lastBIN = bin
	return s.lookupResult, s.lookupErr
}

func (s *fakeBINService) Stats(context.Context) (int64, error) {
	return s.count, s.countErr
}

func TestLookupResponseContract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name        string
		number      string
		service     *fakeBINService
		wantStatus  int
		wantSuccess bool
		wantCode    float64
	}{
		{
			name:        "found",
			number:      "411111",
			service:     &fakeBINService{lookupResult: &model.BINResponse{BIN: "411111", Brand: "visa"}},
			wantStatus:  http.StatusOK,
			wantSuccess: true,
		},
		{name: "not found", number: "99999999", service: &fakeBINService{}, wantStatus: http.StatusNotFound, wantCode: 404},
		{name: "invalid length", number: "4111111", service: &fakeBINService{}, wantStatus: http.StatusBadRequest, wantCode: 400},
		{
			name:       "enrichment rate limited",
			number:     "99999999",
			service:    &fakeBINService{lookupErr: service.ErrEnrichmentRateLimited},
			wantStatus: http.StatusTooManyRequests,
			wantCode:   429,
		},
		{
			name:       "enrichment unavailable",
			number:     "99999999",
			service:    &fakeBINService{lookupErr: service.ErrEnrichmentUnavailable},
			wantStatus: http.StatusServiceUnavailable,
			wantCode:   503,
		},
		{
			name:       "internal error",
			number:     "411111",
			service:    &fakeBINService{lookupErr: errors.New("database failed")},
			wantStatus: http.StatusInternalServerError,
			wantCode:   500,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := gin.New()
			router.GET("/api/v1/bin/:number", NewBINHandler(tt.service).Lookup)
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, "/api/v1/bin/"+tt.number, nil)
			router.ServeHTTP(recorder, request)

			if recorder.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d; body=%s", recorder.Code, tt.wantStatus, recorder.Body.String())
			}
			var body map[string]any
			if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if body["success"] != tt.wantSuccess {
				t.Fatalf("success = %v, want %v", body["success"], tt.wantSuccess)
			}
			if tt.wantCode != 0 && body["code"] != tt.wantCode {
				t.Fatalf("code = %v, want %.0f", body["code"], tt.wantCode)
			}
			if tt.wantStatus == http.StatusBadRequest && tt.service.lookupCalls != 0 {
				t.Fatal("invalid BIN reached service")
			}
			if tt.wantStatus == http.StatusOK && tt.service.lastBIN != tt.number {
				t.Fatalf("service BIN = %q, want %q", tt.service.lastBIN, tt.number)
			}
			if tt.wantStatus == http.StatusTooManyRequests && recorder.Header().Get("Retry-After") == "" {
				t.Fatal("upstream rate-limit response is missing Retry-After")
			}
		})
	}
}

func TestStatsResponseContract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tt := range []struct {
		name       string
		service    *fakeBINService
		wantStatus int
		wantCount  float64
	}{
		{name: "success", service: &fakeBINService{count: 374_789}, wantStatus: http.StatusOK, wantCount: 374_789},
		{name: "error", service: &fakeBINService{countErr: errors.New("database failed")}, wantStatus: http.StatusInternalServerError},
	} {
		t.Run(tt.name, func(t *testing.T) {
			router := gin.New()
			router.GET("/api/v1/stats", NewBINHandler(tt.service).Stats)
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/stats", nil))
			if recorder.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", recorder.Code, tt.wantStatus)
			}
			if tt.wantCount != 0 {
				var body map[string]any
				if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
					t.Fatalf("decode response: %v", err)
				}
				if body["total_bins"] != tt.wantCount {
					t.Fatalf("total_bins = %v, want %.0f", body["total_bins"], tt.wantCount)
				}
			}
		})
	}
}

func TestAccessLogRedactsPANAndQueryString(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const pan = "4111111111111111"
	var output bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&output, nil))
	svc := &fakeBINService{}
	router := gin.New()
	router.Use(middleware.RequestID(), middleware.AccessLogger(logger))
	router.GET("/api/v1/bin/:number", NewBINHandler(svc).Lookup)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/bin/"+pan+"?card="+pan+"&api_key=secret", nil)
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
	logOutput := output.String()
	for _, secret := range []string{pan, "api_key", "secret", "card="} {
		if strings.Contains(logOutput, secret) {
			t.Fatalf("access log leaked %q: %s", secret, logOutput)
		}
	}
	if !strings.Contains(logOutput, "[REDACTED]") {
		t.Fatalf("access log did not mark redaction: %s", logOutput)
	}
}
