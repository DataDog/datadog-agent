// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package system

import (
	"errors"
	"testing"
)

func TestParseMountinfo(t *testing.T) {
	tests := []struct {
		name            string
		filePath        string
		wantContainerID string
		wantErr         error
	}{
		{
			name:            "Docker cgroupv2",
			filePath:        "./testdata/mountinfo_docker",
			wantContainerID: "0cfa82bf3ab29da271548d6a044e95c948c6fd2f7578fb41833a44ca23da425f",
		},
		{
			name:            "Kubernetes containerd cgroupv2 (does not work)",
			filePath:        "./testdata/mountinfo_k8s",
			wantContainerID: "",
		},
		{
			name:            "Kubernetes with mounts (Agent)",
			filePath:        "./testdata/mountinfo_k8s_agent",
			wantContainerID: "fc7038bc73a8d3850c66ddbfb0b2901afa378bfcbb942cc384b051767e4ac6b0",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseMountinfo(tt.filePath)
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("got error = %v, wantErr %v", err, tt.wantErr)
			}

			if got != tt.wantContainerID {
				t.Errorf("got %v, want %v", got, tt.wantContainerID)
			}
		})
	}
}
