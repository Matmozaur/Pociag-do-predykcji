package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"golang.org/x/time/rate"
)

func TestRateLimit_PerClientIsolation(t *testing.T) {
	mw := RateLimit(rate.Limit(1), 1)
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	first := httptest.NewRequest(http.MethodGet, "/api/v1/schedules/search", nil)
	first.RemoteAddr = "10.0.0.1:1000"
	firstRec := httptest.NewRecorder()
	h.ServeHTTP(firstRec, first)
	if firstRec.Code != http.StatusOK {
		t.Fatalf("expected first request to pass, got %d", firstRec.Code)
	}

	second := httptest.NewRequest(http.MethodGet, "/api/v1/schedules/search", nil)
	second.RemoteAddr = "10.0.0.1:1001"
	secondRec := httptest.NewRecorder()
	h.ServeHTTP(secondRec, second)
	if secondRec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected second request from same client to be rate-limited, got %d", secondRec.Code)
	}

	otherClient := httptest.NewRequest(http.MethodGet, "/api/v1/schedules/search", nil)
	otherClient.RemoteAddr = "10.0.0.2:1002"
	otherRec := httptest.NewRecorder()
	h.ServeHTTP(otherRec, otherClient)
	if otherRec.Code != http.StatusOK {
		t.Fatalf("expected request from another client to pass, got %d", otherRec.Code)
	}
}

func TestRateLimit_HealthEndpointsExempt(t *testing.T) {
	mw := RateLimit(rate.Limit(1), 1)
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	healthOne := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	healthOne.RemoteAddr = "10.0.0.1:1000"
	healthOneRec := httptest.NewRecorder()
	h.ServeHTTP(healthOneRec, healthOne)
	if healthOneRec.Code != http.StatusOK {
		t.Fatalf("expected first health request to pass, got %d", healthOneRec.Code)
	}

	healthTwo := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	healthTwo.RemoteAddr = "10.0.0.1:1001"
	healthTwoRec := httptest.NewRecorder()
	h.ServeHTTP(healthTwoRec, healthTwo)
	if healthTwoRec.Code != http.StatusOK {
		t.Fatalf("expected second health request to pass, got %d", healthTwoRec.Code)
	}

	ready := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	ready.RemoteAddr = "10.0.0.1:1002"
	readyRec := httptest.NewRecorder()
	h.ServeHTTP(readyRec, ready)
	if readyRec.Code != http.StatusOK {
		t.Fatalf("expected readiness request to pass, got %d", readyRec.Code)
	}
}
