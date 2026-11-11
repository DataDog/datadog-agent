// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && test

package util

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSplitCollectorName(t *testing.T) {
	cases := []struct {
		name             string
		collectorName    string
		wantGroupVersion string
		wantResource     string
	}{
		{name: "Success - Core group version", collectorName: "v1/nodes", wantGroupVersion: "v1", wantResource: "nodes"},
		{name: "Success - Empty name", collectorName: "", wantGroupVersion: "", wantResource: ""},
		{name: "Success - Leading slash", collectorName: "/widgets", wantGroupVersion: "", wantResource: "widgets"},
		{name: "Success - Named group version", collectorName: "batch/v1/cronjobs", wantGroupVersion: "batch/v1", wantResource: "cronjobs"},
		{name: "Success - No group version", collectorName: "services", wantGroupVersion: "", wantResource: "services"},
		{name: "Success - Trailing slash", collectorName: "batch/v1/", wantGroupVersion: "batch/v1", wantResource: ""},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			groupVersion, resource := SplitCollectorName(tt.collectorName)
			assert.Equal(t, tt.wantGroupVersion, groupVersion)
			assert.Equal(t, tt.wantResource, resource)
		})
	}
}
