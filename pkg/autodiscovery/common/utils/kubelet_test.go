// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetCustomCheckID(t *testing.T) {
	tests := []struct {
		name          string
		annotations   map[string]string
		containerName string
		want          string
		found         bool
	}{
		{
			name:          "found",
			annotations:   map[string]string{"ad.datadoghq.com/foo.check.id": "bar"},
			containerName: "foo",
			want:          "bar",
			found:         true,
		},
		{
			name:          "not found",
			annotations:   map[string]string{"ad.datadoghq.com/foo.check.id": "bar"},
			containerName: "baz",
			want:          "",
			found:         false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, found := GetCustomCheckID(tt.annotations, tt.containerName)
			assert.Equal(t, tt.want, got)
			assert.Equal(t, tt.found, found)
		})
	}
}

func TestValidateAnnotationsMatching(t *testing.T) {
	type args struct {
		annotations    map[string]string
		validIDs       map[string]bool
		containerNames map[string]bool
		adPrefix       string
	}
	tests := []struct {
		name string
		args args
		want []string
	}{
		{
			name: "match",
			args: args{
				annotations: map[string]string{
					"ad.datadoghq.com/nginx.check_names":  "[\"http_check\"]",
					"ad.datadoghq.com/nginx.init_configs": "[{}]",
					"ad.datadoghq.com/nginx.instances":    "[{\"name\": \"My service\", \"url\": \"http://%%host%%\", \"timeout\": 1}]",
				},
				validIDs: map[string]bool{
					"nginx": true,
				},
				containerNames: map[string]bool{
					"nginx": true,
				},
				adPrefix: "ad.datadoghq.com/",
			},
			want: []string{},
		},
		{
			name: "no match",
			args: args{
				annotations: map[string]string{
					"ad.datadoghq.com/nginx.check_names":  "[\"http_check\"]",
					"ad.datadoghq.com/nginx.init_configs": "[{}]",
					"ad.datadoghq.com/nginx.instances":    "[{\"name\": \"My service\", \"url\": \"http://%%host%%\", \"timeout\": 1}]",
				},
				validIDs: map[string]bool{
					"not-nginx": true,
				},
				containerNames: map[string]bool{
					"nginx": true,
				},
				adPrefix: "ad.datadoghq.com/",
			},
			want: []string{
				"annotation ad.datadoghq.com/nginx.check_names is invalid: nginx doesn't match a container identifier",
				"annotation ad.datadoghq.com/nginx.init_configs is invalid: nginx doesn't match a container identifier",
				"annotation ad.datadoghq.com/nginx.instances is invalid: nginx doesn't match a container identifier",
			},
		},
		{
			name: "no errors for pod tags",
			args: args{
				annotations: map[string]string{
					"ad.datadoghq.com/tags": `[{"service":"datadog"}]`,
				},
				validIDs: map[string]bool{
					"another-container": true,
				},
				adPrefix: "ad.datadoghq.com/",
			},
			want: []string{},
		},
		{
			name: "check.id match",
			args: args{
				annotations: map[string]string{
					"ad.datadoghq.com/nginx.check.id":            "nginx-custom",
					"ad.datadoghq.com/nginx-custom.check_names":  "[\"http_check\"]",
					"ad.datadoghq.com/nginx-custom.init_configs": "[{}]",
					"ad.datadoghq.com/nginx-custom.instances":    "[{\"name\": \"My service\", \"url\": \"http://%%host%%\", \"timeout\": 1}]",
				},
				validIDs: map[string]bool{
					"nginx-custom": true,
				},
				containerNames: map[string]bool{
					"nginx": true,
				},
				adPrefix: "ad.datadoghq.com/",
			},
			want: []string{},
		},
		{
			name: "check.id no match",
			args: args{
				annotations: map[string]string{
					"ad.datadoghq.com/nginx.check.id":            "nginx-custom",
					"ad.datadoghq.com/nginx-custom.check_names":  "[\"http_check\"]",
					"ad.datadoghq.com/nginx-custom.init_configs": "[{}]",
					"ad.datadoghq.com/nginx-custom.instances":    "[{\"name\": \"My service\", \"url\": \"http://%%host%%\", \"timeout\": 1}]",
				},
				validIDs: map[string]bool{
					"nginx-custom": true,
				},
				containerNames: map[string]bool{
					"not-nginx": true,
				},
				adPrefix: "ad.datadoghq.com/",
			},
			want: []string{
				"annotation ad.datadoghq.com/nginx.check.id is invalid: nginx doesn't match a container identifier",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ValidateAnnotationsMatching(tt.args.annotations, tt.args.validIDs, tt.args.containerNames, tt.args.adPrefix)
			assert.ElementsMatch(t, tt.want, got)
		})
	}
}
