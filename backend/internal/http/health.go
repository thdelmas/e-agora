package http

import "net/http"

// healthResponse is the body of GET /api/healthz (docs/04-api.md).
type healthResponse struct {
	Status   string `json:"status"`
	Subjects int    `json:"subjects"`
	Seeded   bool   `json:"seeded"`
}

// healthz reports liveness/readiness. The subject count and seeded flag are
// stubbed until the store is wired in M1 (docs/07-roadmap.md).
func (h *handlers) healthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, healthResponse{Status: "ok", Subjects: 0, Seeded: false})
}
