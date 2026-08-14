package handlers

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/face-search-ai/api/internal/platform"
)

type Checker interface {
	Check(context.Context) map[string]platform.Status
}

type healthResponse struct {
	Status       string                     `json:"status"`
	Service      string                     `json:"service"`
	Dependencies map[string]platform.Status `json:"dependencies,omitempty"`
}

func Live(w http.ResponseWriter, _ *http.Request) {
	respond(w, http.StatusOK, healthResponse{Status: "ok", Service: "api"})
}

func Ready(checker Checker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		statuses := checker.Check(r.Context())
		statusCode := http.StatusOK
		status := "ready"
		if !platform.Ready(statuses) {
			statusCode = http.StatusServiceUnavailable
			status = "not_ready"
		}
		respond(w, statusCode, healthResponse{Status: status, Service: "api", Dependencies: statuses})
	}
}

func respond(w http.ResponseWriter, status int, payload healthResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
