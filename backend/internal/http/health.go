package http

import "net/http"

// healthResponse is the body of GET /api/healthz (docs/04-api.md).
type healthResponse struct {
	Status   string `json:"status"`
	Subjects int    `json:"subjects"`
	Seeded   bool   `json:"seeded"`
}

// healthz reports liveness/readiness with the live subject count. "seeded" is
// true once the pool has any subjects; precise seed-state tracking arrives with
// ingestion (M2, docs/07-roadmap.md).
func (h *handlers) healthz(w http.ResponseWriter, r *http.Request) {
	n, err := h.store.CountSubjects(r.Context())
	if err != nil {
		h.logger.Error("healthz: count subjects", "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "Health check failed.")
		return
	}
	writeJSON(w, http.StatusOK, healthResponse{Status: "ok", Subjects: n, Seeded: n > 0})
}
