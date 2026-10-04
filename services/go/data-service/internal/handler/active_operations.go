package handler

import (
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/pociag-do-predykcji/services/go/data-service/internal/model"
)

// activeOperationsCacheTTL is how long a listActiveOperations response without `at` is reused.
const activeOperationsCacheTTL = 15 * time.Second

// HandleListActiveOperations lists the trains active at an instant with estimated positions.
// @Summary		List active trains with estimated positions
// @Description	Returns the trains running at the reference instant at (status P, or S once past the first departure), each with its phase, surrounding stops and an estimated position, ordered by operation_id
// @Tags		operations
// @Produce		json
// @Param		at query string false "Reference instant (RFC 3339); defaults to the current time"
// @Param		carrierCodes query string false "Comma-separated carrier codes"
// @Param		limit query int false "Maximum number of trains to return (1-5000)" default(2000)
// @Success		200 {object} model.ActiveTrainListResponse
// @Failure		400 {object} model.ErrorResponse "Bad request"
// @Router		/api/v1/operations/active [get]
func (h *Handler) HandleListActiveOperations(w http.ResponseWriter, r *http.Request) {
	ctx, span := h.tracer.Start(r.Context(), "operations.active")
	defer span.End()

	var at *time.Time
	if r.URL.Query().Get("at") != "" {
		t, err := boardNow(r)
		if err != nil {
			h.writeError(w, span, http.StatusBadRequest, "invalid_request", "at: "+err.Error())
			return
		}
		at = &t
	}
	limit, err := parseIntInRange(r, "limit", 2000, 1, 5000)
	if err != nil {
		h.writeError(w, span, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	carrierCodes := parseCommaStrings(r.URL.Query().Get("carrierCodes"))

	// Only "now" responses are cached; an explicit `at` is evaluated every time.
	key := activeCacheKey(carrierCodes, limit)
	if at == nil {
		if resp, ok := h.activeCache.get(key); ok {
			span.SetAttributes(attribute.Bool("cache.hit", true))
			h.writeJSON(w, span, http.StatusOK, resp)
			return
		}
	}

	resp, err := h.svc.ListActiveOperations(ctx, at, carrierCodes, limit)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		h.writeError(w, span, http.StatusInternalServerError, "internal_error", "failed to list active operations")
		return
	}
	if at == nil {
		h.activeCache.put(key, resp)
	}

	h.writeJSON(w, span, http.StatusOK, resp)
}

func activeCacheKey(carrierCodes []string, limit int) string {
	codes := append([]string(nil), carrierCodes...)
	sort.Strings(codes)
	return strings.Join(codes, ",") + "|" + strconv.Itoa(limit)
}

// responseCache is a small TTL cache of active-operations responses keyed by request params.
type responseCache struct {
	ttl     time.Duration
	now     func() time.Time
	mu      sync.Mutex
	entries map[string]cacheEntry
}

type cacheEntry struct {
	resp    *model.ActiveTrainListResponse
	expires time.Time
}

func newResponseCache(ttl time.Duration) *responseCache {
	return &responseCache{ttl: ttl, now: time.Now, entries: map[string]cacheEntry{}}
}

func (c *responseCache) get(key string) (*model.ActiveTrainListResponse, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[key]
	if !ok || !c.now().Before(e.expires) {
		return nil, false
	}
	return e.resp, true
}

func (c *responseCache) put(key string, resp *model.ActiveTrainListResponse) {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.now()
	// Drop expired entries so arbitrary carrierCodes values cannot grow the map without bound.
	for k, e := range c.entries {
		if !now.Before(e.expires) {
			delete(c.entries, k)
		}
	}
	c.entries[key] = cacheEntry{resp: resp, expires: now.Add(c.ttl)}
}
