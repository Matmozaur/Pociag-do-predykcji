package handler

import (
	"net/http/httptest"
	"testing"
)

func TestParseCSVInts_InvalidValue_ReturnsError(t *testing.T) {
	_, err := parseCSVInts("10,abc")
	if err == nil {
		t.Fatal("expected error for invalid CSV integer")
	}
}

func TestParseLimitOffset_ValidQuery_ReturnsValues(t *testing.T) {
	req := httptest.NewRequest("GET", "/api/v1/schedules/search?limit=25&offset=5", nil)
	limit, offset, err := parseLimitOffset(req, 20, 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if limit != 25 || offset != 5 {
		t.Fatalf("unexpected values: limit=%d offset=%d", limit, offset)
	}
}
