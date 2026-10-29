// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build trivy && (docker || containerd || crio)

package trivy

import (
	"errors"
	"testing"
	"time"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	dimage "github.com/moby/moby/api/types/image"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPopulateImageExportsOnce checks that the image is exported once, and
// that a failed export returns its error again rather than exporting anew.
func TestPopulateImageExportsOnce(t *testing.T) {
	errExport := errors.New("export failed")
	tests := []struct {
		name    string
		opener  opener
		wantErr error
	}{
		{
			name:    "export fails",
			opener:  func() (v1.Image, error) { return nil, errExport },
			wantErr: errExport,
		},
		{
			name:   "export succeeds",
			opener: func() (v1.Image, error) { return empty.Image, nil },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			img := &image{opener: func() (v1.Image, error) {
				calls++
				return tt.opener()
			}}
			for range 2 {
				err := img.populateImage()
				if tt.wantErr != nil {
					assert.ErrorIs(t, err, tt.wantErr)
				} else {
					assert.NoError(t, err)
				}
			}
			assert.Equal(t, 1, calls)
		})
	}
}

// mismatchedImage returns an image whose history lists one of its two
// layers, as for an image built without history, and whose export fails. It
// counts the exports in exports.
func mismatchedImage(exports *int) *image {
	return &image{
		opener: func() (v1.Image, error) {
			*exports++
			return nil, errors.New("export failed")
		},
		inspect: dimage.InspectResponse{
			ID:      "sha256:3333333333333333333333333333333333333333333333333333333333333333",
			Created: "2024-11-07T00:26:15.123456789Z",
			RootFS: dimage.RootFS{
				Type: "layers",
				Layers: []string{
					"sha256:1111111111111111111111111111111111111111111111111111111111111111",
					"sha256:2222222222222222222222222222222222222222222222222222222222222222",
				},
			},
		},
		history: []v1.History{{CreatedBy: "COPY . /"}},
	}
}

// TestInspectConfigFile checks that a history disagreeing with the diff IDs
// makes ConfigFile export the image, while inspectConfigFile builds the config
// from the inspect response.
func TestInspectConfigFile(t *testing.T) {
	var exports int
	img := mismatchedImage(&exports)

	configFile, err := img.inspectConfigFile()
	require.NoError(t, err)
	assert.Len(t, configFile.RootFS.DiffIDs, 2)
	assert.Equal(t, time.Date(2024, time.November, 7, 0, 26, 15, 123456789, time.UTC), configFile.Created.Time)
	assert.Equal(t, 0, exports)

	_, err = img.ConfigFile()
	assert.Error(t, err)
	assert.Equal(t, 1, exports)
}
