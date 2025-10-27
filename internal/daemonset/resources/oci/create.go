package oci

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
)

type createReq struct {
	DumpPath string `json:"dump_path" validate:"required,filepath"`
}

type createResp struct {
	JobId string `json:"job_id"`
}

func (rs OciResource) CreateUnwrapped(id uuid.UUID) {
}

func (rs OciResource) Create(rw http.ResponseWriter, req *http.Request) {
	var data createReq
	err := json.NewDecoder(req.Body).Decode(&data)
	if err != nil {
		http.Error(rw, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	err = validator.New().Struct(data)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusBadRequest)
		return
	}

	id, err := uuid.NewV7()
	if err != nil {
		slog.Error("Generating uuid", "err", err.Error())
		http.Error(rw, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	resp, err := json.Marshal(createResp{
		JobId: id.String(),
	})
	if err != nil {
		slog.Error("Marshaling response json", "err", err.Error())
		http.Error(rw, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	rs.jobs[id] = OciStatus{
		Stage: Fromating,
		State: Idle,
	}
	go rs.CreateUnwrapped(id)

	_, err = rw.Write(resp)
	if err != nil {
		slog.Error("Writing response", "err", err.Error())
		http.Error(rw, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
}
