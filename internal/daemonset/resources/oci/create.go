package oci

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"bud.studio/stove8s/internal/oci"
	"github.com/go-playground/validator/v10"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/uuid"
)

type CreateReqImagePushSecret struct {
	Name      string `json:"name" validate:"required"`
	Namespace string `json:"namespace" validate:"required"`
}

type CreateReq struct {
	CheckpointDumpPath string                   `json:"checkpoint_dump_path" validate:"required,filepath"`
	ImagePushSecret    CreateReqImagePushSecret `json:"image_push_secret" validate:"required"`
	ImageReference     string                   `json:"image_reference" validate:"required"`
}

type createResp struct {
	JobId string `json:"job_id"`
}

func (rs OciResource) CreateAsync(id uuid.UUID, data *CreateReq) {
	status := OciStatus{
		Stage: Fromating,
		State: Started,
	}
	rs.jobs[id] = &status

	idx, err := oci.BuildIdx(data.CheckpointDumpPath)
	if err != nil {
		slog.Error("Building oci image", "err", err)
		status.State = Failed
		return
	}
	ref, err := name.ParseReference("harbor.bud.studio/stove8s/test:latest")
	if err != nil {
		slog.Error("Creating reference", "err", err)
		status.State = Failed
		return
	}

	status.Stage = Pushing
	status.State = Started

	auth, err := rs.k8sImagePushSecretGet(
		data.ImagePushSecret.Name,
		data.ImagePushSecret.Namespace,
		ref.Context().RegistryStr(),
	)
	if err != nil {
		slog.Error("Getting image push secret", "err", err)
		status.State = Failed
		return
	}
	err = remote.WriteIndex(
		ref,
		idx,
		remote.WithAuth(auth),
	)
	if err != nil {
		slog.Error("Pushing to remote", "err", err)
		status.State = Failed
		return
	}

	status.State = Success
}

func (rs OciResource) Create(rw http.ResponseWriter, req *http.Request) {
	var data CreateReq
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

	go rs.CreateAsync(id, &data)

	_, err = rw.Write(resp)
	if err != nil {
		slog.Error("Writing response", "err", err.Error())
		http.Error(rw, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	return
}
