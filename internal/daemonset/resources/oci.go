package resources

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

type OciStatusStage string
type OciStatusState string

const (
	Fromating OciStatusStage = "Fromating"
	Pushing   OciStatusStage = "Pushing"

	Idle    OciStatusState = "Idle"
	Started OciStatusState = "Started"
	Failed  OciStatusState = "Failed"
	Success OciStatusState = "Success"
)

type OciStatus struct {
	stage OciStatusStage
	state OciStatusState
}

type OciResource struct {
	id map[string]OciStatus
}

func (rs OciResource) Routes() chi.Router {
	r := chi.NewRouter()

	r.Get("/", rs.List)
	r.Post("/", rs.Create)

	r.Route("/{id}", func(r chi.Router) {
		r.Get("/", rs.Get)
		r.Delete("/", rs.Delete)
	})

	return r
}

func (rs OciResource) List(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("TODO\n"))
}

func (rs OciResource) Create(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("TODO\n"))
}

func (rs OciResource) Get(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("TODO\n"))
}

func (rs OciResource) Delete(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("TODO\n"))
}
