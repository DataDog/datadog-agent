// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"fmt"
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
		want []error
	}{
		{
			name: "Match",
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
			want: []error{},
		},
		{
			name: "No match",
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
			want: []error{
				fmt.Errorf("annotation ad.datadoghq.com/nginx.check_names is invalid: nginx doesn't match a container identifier"),
				fmt.Errorf("annotation ad.datadoghq.com/nginx.init_configs is invalid: nginx doesn't match a container identifier"),
				fmt.Errorf("annotation ad.datadoghq.com/nginx.instances is invalid: nginx doesn't match a container identifier"),
			},
		},
		{
			name: "No errors for pod tags",
			args: args{
				annotations: map[string]string{
					"ad.datadoghq.com/tags": `[{"service":"datadog"}]`,
				},
				validIDs: map[string]bool{
					"another-container": true,
				},
				adPrefix: "ad.datadoghq.com/",
			},
			want: []error{},
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
			want: []error{},
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
			want: []error{
				fmt.Errorf("annotation ad.datadoghq.com/nginx.check.id is invalid: nginx doesn't match a container identifier"),
			},
		},
		{
			name: "Legacy annotations",
			args: args{
				annotations: map[string]string{
					"service-discovery.datadoghq.com/nginx.check_names":  "[\"http_check\"]",
					"service-discovery.datadoghq.com/nginx.init_configs": "[{}]",
					"service-discovery.datadoghq.com/nginx.instances":    "[{\"name\": \"My service\", \"url\": \"http://%%host%%\", \"timeout\": 1}]",
				},
				validIDs: map[string]bool{
					"nginx": true,
				},
				containerNames: map[string]bool{
					"nginx": true,
				},
				adPrefix: "service-discovery.datadoghq.com/",
			},
			want: []error{},
		},
		{
			name: "No errors with multiple containers and only one using check.id",
			args: args{
				annotations: map[string]string{
					"ad.datadoghq.com/nginx.check.id":            "nginx-custom",
					"ad.datadoghq.com/nginx-custom.check_names":  "[\"http_check\"]",
					"ad.datadoghq.com/nginx-custom.init_configs": "[{}]",
					"ad.datadoghq.com/nginx-custom.instances":    "[{\"name\": \"Service\", \"url\": \"http://%%host_external%%\", \"timeout\": 1}]",
					"ad.datadoghq.com/apache.check_names":        "[\"http_check\"]",
					"ad.datadoghq.com/apache.init_configs":       "[{}]",
					"ad.datadoghq.com/apache.instances":          "[{\"name\": \"Other service\", \"url\": \"http://%%host_external%%\", \"timeout\": 1}]",
				},
				validIDs: map[string]bool{
					"nginx-custom": true,
					"apache":       true,
				},
				containerNames: map[string]bool{
					"nginx":  true,
					"apache": true,
				},
				adPrefix: "ad.datadoghq.com/",
			},
			want: []error{},
		},
		{
			name: "check.id with non-matching container name or identifier",
			args: args{
				annotations: map[string]string{
					"ad.datadoghq.com/nginx.check.id":         "nginx-custom",
					"ad.datadoghq.com/not-nginx.check_names":  "[\"http_check\"]",
					"ad.datadoghq.com/not-nginx.init_configs": "[{}]",
					"ad.datadoghq.com/not-nginx.instances":    "[{\"name\": \"My service\", \"url\": \"http://%%host%%\", \"timeout\": 1}]",
				},
				validIDs: map[string]bool{
					"nginx-custom": true,
				},
				containerNames: map[string]bool{
					"nginx": true,
				},
				adPrefix: "ad.datadoghq.com/",
			},
			want: []error{
				fmt.Errorf("annotation ad.datadoghq.com/not-nginx.check_names is invalid: not-nginx doesn't match a container identifier"),
				fmt.Errorf("annotation ad.datadoghq.com/not-nginx.init_configs is invalid: not-nginx doesn't match a container identifier"),
				fmt.Errorf("annotation ad.datadoghq.com/not-nginx.instances is invalid: not-nginx doesn't match a container identifier"),
			},
		},
		{
			name: "check.id with non-matching container identifier and matching container name",
			args: args{
				annotations: map[string]string{
					"ad.datadoghq.com/nginx.check.id":     "nginx-custom",
					"ad.datadoghq.com/nginx.check_names":  "[\"http_check\"]",
					"ad.datadoghq.com/nginx.init_configs": "[{}]",
					"ad.datadoghq.com/nginx.instances":    "[{\"name\": \"My service\", \"url\": \"http://%%host%%\", \"timeout\": 1}]",
				},
				validIDs: map[string]bool{
					"nginx-custom": true,
				},
				containerNames: map[string]bool{
					"nginx": true,
				},
				adPrefix: "ad.datadoghq.com/",
			},
			want: []error{
				fmt.Errorf("annotation ad.datadoghq.com/nginx.check_names is invalid: nginx doesn't match a container identifier"),
				fmt.Errorf("annotation ad.datadoghq.com/nginx.init_configs is invalid: nginx doesn't match a container identifier"),
				fmt.Errorf("annotation ad.datadoghq.com/nginx.instances is invalid: nginx doesn't match a container identifier"),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ValidateAnnotationsMatching(tt.args.annotations, tt.args.validIDs, tt.args.containerNames, tt.args.adPrefix)
			assert.Equal(t, len(tt.want), len(got))
			assert.ElementsMatch(t, tt.want, got)
		})
	}
}
