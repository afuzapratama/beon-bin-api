package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

type fakePinger struct {
	err error
}

func (p *fakePinger) PingContext(context.Context) error { return p.err }

type fakeReadinessMetrics struct {
	ready bool
	calls int
}

func (m *fakeReadinessMetrics) SetReady(ready bool) {
	m.ready = ready
	m.calls++
}

func TestHealthProbes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tt := range []struct {
		name       string
		path       string
		pingErr    error
		wantStatus int
		wantReady  bool
		wantCalls  int
	}{
		{name: "live ignores dependencies", path: "/live", pingErr: errors.New("down"), wantStatus: http.StatusOK},
		{name: "ready", path: "/ready", wantStatus: http.StatusOK, wantReady: true, wantCalls: 1},
		{name: "not ready", path: "/ready", pingErr: errors.New("down"), wantStatus: http.StatusServiceUnavailable, wantCalls: 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			pinger := &fakePinger{err: tt.pingErr}
			readiness := &fakeReadinessMetrics{}
			handler := NewHealthHandler(pinger, readiness)
			router := gin.New()
			router.GET("/live", handler.Live)
			router.GET("/ready", handler.Ready)
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, tt.path, nil))

			if recorder.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d; body=%s", recorder.Code, tt.wantStatus, recorder.Body.String())
			}
			if readiness.ready != tt.wantReady || readiness.calls != tt.wantCalls {
				t.Fatalf("readiness = %v calls=%d, want %v/%d", readiness.ready, readiness.calls, tt.wantReady, tt.wantCalls)
			}
		})
	}
}
