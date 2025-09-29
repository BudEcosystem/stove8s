package oci

import (
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type ociStatusStage string
type ociStatusState string

const (
	Fromating ociStatusStage = "Fromating"
	Pushing   ociStatusStage = "Pushing"

	Idle    ociStatusState = "Idle"
	Started ociStatusState = "Started"
	Failed  ociStatusState = "Failed"
	Success ociStatusState = "Success"
)

type OciStatus struct {
	Stage  ociStatusStage `json:"stage"`
	State  ociStatusState `json:"state"`
	Reason string         `json:"reason"`
}

type OciResource struct {
	jobs map[uuid.UUID]OciStatus
}

func (rs OciResource) Init() chi.Router {
	rs.jobs = make(map[uuid.UUID]OciStatus)

	r := chi.NewRouter()

	r.Get("/", rs.List)
	r.Post("/", rs.Create)

	r.Route("/{id}", func(r chi.Router) {
		r.Get("/", rs.Get)
	})

	return r
}
