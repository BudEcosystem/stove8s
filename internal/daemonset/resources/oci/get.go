package oci

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type listResp struct {
	Jobs map[uuid.UUID]OciStatus `json:"jobs"`
}

func (rs OciResource) List(rw http.ResponseWriter, req *http.Request) {
	resp, err := json.Marshal(listResp{
		Jobs: rs.jobs,
	})
	if err != nil {
		slog.Error("Marshaling response json", "err", err.Error())
	}

	_, err = rw.Write(resp)
	if err != nil {
		slog.Error("Writing response", "err", err.Error())
		http.Error(rw, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
}

func (rs OciResource) Get(rw http.ResponseWriter, req *http.Request) {
	idString := chi.URLParam(req, "id")
	if idString == "" {
		http.Error(rw, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	id, err := uuid.Parse(idString)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusBadRequest)
		return
	}
	job, ok := rs.jobs[id]
	if !ok {
		http.Error(rw, http.StatusText(http.StatusBadRequest), http.StatusNotFound)
		return
	}

	resp, err := json.Marshal(job)
	if err != nil {
		slog.Error("Marshaling response json", "err", err.Error())
		http.Error(rw, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	_, err = rw.Write(resp)
	if err != nil {
		slog.Error("Writing response", "err", err.Error())
		http.Error(rw, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
}
