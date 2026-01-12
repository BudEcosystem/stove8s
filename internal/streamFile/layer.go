// Copyright 2018 Google LLC All Rights Reserved.
// Copyright 2026 Bud Ecosystem Inc All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package streamFile implements a file streaming v1.Layer.
package streamFile

import (
	"compress/gzip"
	"crypto"
	"encoding/hex"
	"errors"
	"io"
	"log/slog"
	"os"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/types"
)

// Layer is a streaming implementation of v1.Layer.
type Layer struct {
	blobPath string

	compression int
	mediaType   types.MediaType

	digest, diffID *v1.Hash
	size           int64
}

var _ v1.Layer = (*Layer)(nil)

// LayerOption applies options to layer
type LayerOption func(*Layer)

// WithCompressionLevel sets the gzip compression. See `gzip.NewWriterLevel` for possible values.
func WithCompressionLevel(level int) LayerOption {
	return func(l *Layer) {
		l.compression = level
	}
}

// WithMediaType is a functional option for overriding the layer's media type.
func WithMediaType(mt types.MediaType) LayerOption {
	return func(l *Layer) {
		l.mediaType = mt
	}
}

// NewLayer creates a Layer from an blobPath.
func NewLayer(blobPath string, opts ...LayerOption) (*Layer, error) {
	blob, err := os.Open(blobPath)
	if err != nil {
		return nil, err
	}
	defer func() {
		err = blob.Close()
		if err != nil {
			slog.Error("closing blob", "err", err)
		}
	}()

	layer := &Layer{
		blobPath:    blobPath,
		compression: gzip.BestCompression,
		// We use DockerLayer for now as uncompressed layers
		// are unimplemented
		mediaType: types.DockerLayer,
	}
	for _, opt := range opts {
		opt(layer)
	}

	hash := crypto.SHA256.New()
	compressedHash := crypto.SHA256.New()
	compressedSize := &sizeWriter{}
	multiWriter := io.MultiWriter(compressedHash, compressedSize)
	gzipWriter, err := gzip.NewWriterLevel(multiWriter, layer.compression)
	if err != nil {
		return nil, err
	}
	_, err = io.Copy(io.MultiWriter(hash, gzipWriter), blob)
	if err != nil {
		return nil, err
	}
	err = gzipWriter.Close()
	if err != nil {
		return nil, err
	}

	diffID, err := v1.NewHash("sha256:" + hex.EncodeToString(hash.Sum(nil)))
	if err != nil {
		return nil, err
	}
	layer.diffID = &diffID
	digest, err := v1.NewHash("sha256:" + hex.EncodeToString(compressedHash.Sum(nil)))
	if err != nil {
		return nil, err
	}
	layer.digest = &digest
	layer.size = compressedSize.n

	return layer, nil
}

// Digest implements v1.Layer.
func (l *Layer) Digest() (v1.Hash, error) {
	return *l.digest, nil
}

// DiffID implements v1.Layer.
func (l *Layer) DiffID() (v1.Hash, error) {
	return *l.diffID, nil
}

// Size implements v1.Layer.
func (l *Layer) Size() (int64, error) {
	return l.size, nil
}

// MediaType implements v1.Layer
func (l *Layer) MediaType() (types.MediaType, error) {
	return l.mediaType, nil
}

// Uncompressed implements v1.Layer.
func (l *Layer) Uncompressed() (io.ReadCloser, error) {
	return nil, errors.New("NYI: streamFile.Layer.Uncompressed is not implemented")
}

// Compressed implements v1.Layer.
func (l *Layer) Compressed() (io.ReadCloser, error) {
	blob, err := os.Open(l.blobPath)
	if err != nil {
		return nil, err
	}

	pr, pw := io.Pipe()
	gzipWriter, err := gzip.NewWriterLevel(pw, l.compression)
	if err != nil {
		return nil, err
	}

	go func() {
		_, err := io.Copy(gzipWriter, blob)
		if err != nil {
			slog.Error("Copying blob to gzip writer", "err", err)
		}
		err = gzipWriter.Close()
		if err != nil {
			slog.Error("Closing gzip writer", "err", err)
		}
		err = pw.Close()
		if err != nil {
			slog.Error("Closing pipe", "err", err)
		}
		err = blob.Close()
		if err != nil {
			slog.Error("Closing blob", "err", err)
		}
	}()

	return io.NopCloser(pr), nil
}

type sizeWriter struct{ n int64 }

func (c *sizeWriter) Write(p []byte) (int, error) {
	c.n += int64(len(p))
	return len(p), nil
}
