// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package kubeapiserver

import (
	"reflect"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/config"
)

func Test_filterMapStringKey(t *testing.T) {
	annotationstest := map[string]string{
		"foo":                               "bar",
		"ad.datadoghq.com/foo.checks":       `{"json":"foo"}`,
		"ad.datadoghq.com/checks":           `{"json":"foo"}`,
		"ad.datadoghq.com/foo.check_names":  `{"json":"foo"}`,
		"ad.datadoghq.com/instances":        `{"json":"foo"}`,
		"ad.datadoghq.com/foo.instances":    `{"json":"foo"}`,
		"ad.datadoghq.com/init_configs":     `{"json":"foo"}`,
		"ad.datadoghq.com/foo.init_configs": `{"json":"foo"}`,
		"ad.datadoghq.com/tags":             `["bar","foo"]`,
	}

	defaultExclude := config.Datadog.GetStringSlice("cluster_agent.kubernetes_resources_collection.pod_annotations_exclude")
	extraExclude := append(defaultExclude, "foo")

	tests := []struct {
		name          string
		excludeString []string
		annotations   map[string]string
		want          map[string]string
	}{
		{
			name:        "no filter",
			annotations: copyMap(annotationstest),
			want:        annotationstest,
		},
		{
			name:          "default filters",
			excludeString: defaultExclude,
			annotations:   copyMap(annotationstest),
			want: map[string]string{
				"foo":                   "bar",
				"ad.datadoghq.com/tags": `["bar","foo"]`,
			},
		},
		{
			name:          "default filters",
			excludeString: extraExclude,
			annotations:   copyMap(annotationstest),
			want: map[string]string{
				"ad.datadoghq.com/tags": `["bar","foo"]`,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			options, err := newParseOptions(tt.excludeString)
			if err != nil {
				t.Errorf("newParseOptions() return an error %v", err)
			}
			if got := filterMapStringKey(tt.annotations, options.annotationsFilter); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("filterPodAnnotations() = %v, want %v", got, tt.want)
			}
		})
	}
}

func copyMap(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}
