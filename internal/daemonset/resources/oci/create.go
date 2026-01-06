package oci

import (
	"encoding/json"
	"log/slog"
	"net/http"

	stove8sv1beta1 "bud.studio/stove8s/api/v1beta1"
	"bud.studio/stove8s/internal/k8s"
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

type CreateResp struct {
	JobID string `json:"job_id"`
}

func (rs Resource) CreateAsync(id uuid.UUID, data *CreateReq) {
	status := Status{
		Stage: stove8sv1beta1.Fromating,
		State: stove8sv1beta1.Started,
	}
	rs.jobs[id] = &status

	img, dumpFile, err := oci.BuildImage(data.CheckpointDumpPath)
	defer func() {
		if dumpFile != nil {
			dumpFile.Close()
		}
	}()
	if err != nil {
		slog.Error("Building oci image", "err", err)
		status.State = stove8sv1beta1.Failed
		return
	}
	ref, err := name.ParseReference(data.ImageReference)
	if err != nil {
		slog.Error("Creating reference", "err", err)
		status.State = stove8sv1beta1.Failed
		return
	}

	status.Stage = stove8sv1beta1.Pushing
	status.State = stove8sv1beta1.Started

	auth, err := k8s.ImagePushSecretGet(
		rs.k8sClient,
		data.ImagePushSecret.Namespace,
		data.ImagePushSecret.Name,
		ref.Context().RegistryStr(),
	)
	if err != nil {
		slog.Error("Getting image push secret", "err", err)
		status.State = stove8sv1beta1.Failed
		return
	}
	err = remote.Write(
		ref,
		img,
		remote.WithAuth(auth),
	)
	if err != nil {
		slog.Error("Pushing to remote", "err", err)
		status.State = stove8sv1beta1.Failed
		return
	}

	status.State = stove8sv1beta1.Success
}

func (rs Resource) Create(rw http.ResponseWriter, req *http.Request) {
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

	resp, err := json.Marshal(CreateResp{
		JobID: id.String(),
	})
	if err != nil {
		slog.Error("Marshaling response json", "err", err.Error())
		http.Error(rw, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	go rs.CreateAsync(id, &data)

	rw.WriteHeader(http.StatusCreated)
	_, err = rw.Write(resp)
	if err != nil {
		slog.Error("Writing response", "err", err.Error())
		http.Error(rw, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
}
