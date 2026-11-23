// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOCIChartRegistryIndex(t *testing.T) {
	const ecr = "669783387624.dkr.ecr.us-east-1.amazonaws.com"
	tests := []struct {
		name       string
		chartRef   string
		registries string
		want       int
	}{
		{"chart on the only registry", "oci://" + ecr + "/ghcr/nvidia/chart/nvml-mock", ecr, 0},
		{"chart on the second registry", "oci://" + ecr + "/k8s/kueue/charts/kueue", "us-docker.pkg.dev," + ecr, 1},
		{"public chart", "oci://ghcr.io/nvidia/k8s-test-infra/chart/nvml-mock", ecr, -1},
		{"no registries configured", "oci://" + ecr + "/ghcr/chart", "", -1},
		{"not an OCI chart", ecr + "/ghcr/chart", ecr, -1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, ociChartRegistryIndex(tt.chartRef, tt.registries))
		})
	}
}

func TestImagePullCredential(t *testing.T) {
	const registries = "us-docker.pkg.dev,ecr.example.com"
	tests := []struct {
		name      string
		index     int
		usernames string
		passwords string
		wantUser  string
		wantPwd   string
		wantErr   string
	}{
		{name: "second registry", index: 1, usernames: "u0,u1", passwords: "p0,p1", wantUser: "u1", wantPwd: "p1"},
		{name: "base64 password", index: 0, usernames: "AWS", passwords: "b64=c2VjcmV0", wantUser: "AWS", wantPwd: "secret"},
		{name: "invalid base64", index: 0, usernames: "AWS", passwords: "b64=%%%", wantErr: "base64"},
		{name: "missing username", index: 1, usernames: "u0", passwords: "p0,p1", wantErr: "no image pull username"},
		{name: "missing password", index: 1, usernames: "u0,u1", passwords: "p0", wantErr: "no image pull password"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user, pwd, err := imagePullCredential(tt.index, registries, tt.usernames, tt.passwords)
			if tt.wantErr != "" {
				assert.ErrorContains(t, err, tt.wantErr)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.wantUser, user)
			assert.Equal(t, tt.wantPwd, pwd)
		})
	}
}
