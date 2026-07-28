package plk

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestClient_FetchOperations_UsesDocumentedQueryParameters(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/operations" {
			t.Errorf("path = %q", r.URL.Path)
		}
		query := r.URL.Query()
		if got := query.Get("page"); got != "2" {
			t.Errorf("page = %q, want 2", got)
		}
		if got := query.Get("pageSize"); got != "1000" {
			t.Errorf("pageSize = %q, want 1000", got)
		}
		if got := query.Get("withPlanned"); got != "true" {
			t.Errorf("withPlanned = %q, want true", got)
		}
		if got := query.Get("date"); got != "" {
			t.Errorf("unsupported date query parameter = %q", got)
		}
		_, _ = io.WriteString(w, `{"pagination":{"totalPages":2,"hasNextPage":false},"trains":[]}`)
	}))
	defer server.Close()

	client := New(server.URL, "test-key", server.Client())
	if _, err := client.FetchOperations(context.Background(), 2, 1000); err != nil {
		t.Fatalf("FetchOperations() error = %v", err)
	}
}

func TestClient_FetchScheduleRoutes_DoesNotSendPaginationParameters(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/schedules/routes/2026-05-01" {
			t.Errorf("path = %q", r.URL.Path)
		}
		if r.URL.RawQuery != "" {
			t.Errorf("query = %q, want empty", r.URL.RawQuery)
		}
		_, _ = io.WriteString(w, `{"routes":[]}`)
	}))
	defer server.Close()

	client := New(server.URL, "test-key", server.Client())
	if _, err := client.FetchScheduleRoutes(context.Background(), time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("FetchScheduleRoutes() error = %v", err)
	}
}
