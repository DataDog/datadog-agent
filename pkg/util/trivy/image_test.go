// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build trivy && (docker || containerd || crio)

package trivy

import (
	"errors"
	"testing"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/stretchr/testify/assert"
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
