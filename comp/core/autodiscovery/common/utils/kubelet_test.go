// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateAnnotationsMatching(t *testing.T) {
	type args struct {
		annotations    map[string]string
		validIDs       map[string]struct{}
		containerNames map[string]struct{}
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
				validIDs: map[string]struct{}{
					"nginx": {},
				},
				containerNames: map[string]struct{}{
					"nginx": {},
				},
			},
			want: []error{},
		},
		{
			name: "No match",
			args: args{
				annotations: map[string]string{
					"ad.datadoghq.com/nginx.check_names":                "[\"http_check\"]",
					"ad.datadoghq.com/not-nginx.not-nginx.init_configs": "[{}]",
					"ad.datadoghq.com/nginx.instances":                  "[{\"name\": \"My service\", \"url\": \"http://%%host%%\", \"timeout\": 1}]",
				},
				validIDs: map[string]struct{}{
					"not-nginx": {},
				},
				containerNames: map[string]struct{}{
					"nginx": {},
				},
			},
			want: []error{
				errors.New("annotation ad.datadoghq.com/nginx.check_names is invalid: nginx doesn't match a container identifier [not-nginx]"),
				errors.New("annotation ad.datadoghq.com/not-nginx.not-nginx.init_configs is invalid: not-nginx.not-nginx doesn't match a container identifier [not-nginx]"),
				errors.New("annotation ad.datadoghq.com/nginx.instances is invalid: nginx doesn't match a container identifier [not-nginx]"),
			},
		},
		{
			name: "No errors for pod tags",
			args: args{
				annotations: map[string]string{
					"ad.datadoghq.com/tags": `[{"service":"datadog"}]`,
				},
				validIDs: map[string]struct{}{
					"another-container": {},
				},
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
				validIDs: map[string]struct{}{
					"nginx-custom": {},
				},
				containerNames: map[string]struct{}{
					"nginx": {},
				},
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
				validIDs: map[string]struct{}{
					"nginx-custom": {},
				},
				containerNames: map[string]struct{}{
					"not-nginx":      {},
					"also-not-nginx": {},
				},
			},
			want: []error{
				errors.New("annotation ad.datadoghq.com/nginx.check.id is invalid: nginx doesn't match a container identifier [also-not-nginx not-nginx]"),
			},
		},
		{
			name: "Legacy annotations are ignored",
			args: args{
				annotations: map[string]string{
					"service-discovery.datadoghq.com/nginx.check.id":     "nginx-custom",
					"service-discovery.datadoghq.com/nginx.check_names":  "[\"http_check\"]",
					"service-discovery.datadoghq.com/nginx.init_configs": "[{}]",
					"service-discovery.datadoghq.com/nginx.instances":    "[{\"name\": \"My service\", \"url\": \"http://%%host%%\", \"timeout\": 1}]",
				},
				validIDs: map[string]struct{}{
					"not-nginx": {},
				},
				containerNames: map[string]struct{}{
					"not-nginx": {},
				},
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
				validIDs: map[string]struct{}{
					"nginx-custom": {},
					"apache":       {},
				},
				containerNames: map[string]struct{}{
					"nginx":  {},
					"apache": {},
				},
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
				validIDs: map[string]struct{}{
					"nginx-custom": {},
				},
				containerNames: map[string]struct{}{
					"nginx": {},
				},
			},
			want: []error{
				errors.New("annotation ad.datadoghq.com/not-nginx.check_names is invalid: not-nginx doesn't match a container identifier [nginx-custom]"),
				errors.New("annotation ad.datadoghq.com/not-nginx.init_configs is invalid: not-nginx doesn't match a container identifier [nginx-custom]"),
				errors.New("annotation ad.datadoghq.com/not-nginx.instances is invalid: not-nginx doesn't match a container identifier [nginx-custom]"),
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
				validIDs: map[string]struct{}{
					"nginx-custom": {},
				},
				containerNames: map[string]struct{}{
					"nginx": {},
				},
			},
			want: []error{
				errors.New("annotation ad.datadoghq.com/nginx.check_names is invalid: nginx doesn't match a container identifier [nginx-custom]"),
				errors.New("annotation ad.datadoghq.com/nginx.init_configs is invalid: nginx doesn't match a container identifier [nginx-custom]"),
				errors.New("annotation ad.datadoghq.com/nginx.instances is invalid: nginx doesn't match a container identifier [nginx-custom]"),
			},
		},
		{
			name: "Incorrect autodiscovery annotation prefixes",
			args: args{
				annotations: map[string]string{
					"ads.datadoghq.com/not-nginx.check.id":     "nginx-custom",
					"ad.datadoghq.com/.check_names":            "[\"http_check\"]",
					"test.ad.datadoghq.com/nginx.init_configs": "[{}]",
					"ad.datadoghq..com/nginx.instances":        "[{\"name\": \"My service\", \"url\": \"http://%%host%%\", \"timeout\": 1}]",
				},
				validIDs: map[string]struct{}{
					"nginx-custom": {},
				},
				containerNames: map[string]struct{}{
					"nginx": {},
				},
			},
			want: []error{
				errors.New("annotation ad.datadoghq.com/.check_names is invalid:  doesn't match a container identifier [nginx-custom]"),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ValidateAnnotationsMatching(tt.args.annotations, tt.args.validIDs, tt.args.containerNames)
			assert.Equal(t, len(tt.want), len(got))
			assert.ElementsMatch(t, tt.want, got)
		})
	}
}
