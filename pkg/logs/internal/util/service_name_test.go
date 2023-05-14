// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package util

import (
	"errors"
	"testing"
)

func TestServiceNameFromTags(t *testing.T) {
	tests := []struct {
		name         string
		tFunc        func(string) ([]string, error)
		ctrName      string
		taggerEntity string
		want         string
	}{
		{
			name: "nominal case",
			tFunc: func(e string) ([]string, error) {
				return []string{"env:foo", "service:bar"}, nil
			},
			ctrName:      "ctr-name",
			taggerEntity: "ctr entity",
			want:         "bar",
		},
		{
			name: "tagger error",
			tFunc: func(e string) ([]string, error) {
				return nil, errors.New("err")
			},
			ctrName:      "ctr-name",
			taggerEntity: "ctr entity",
			want:         "",
		},
		{
			name: "not found",
			tFunc: func(e string) ([]string, error) {
				return []string{"env:foo", "version:bar"}, nil
			},
			ctrName:      "ctr-name",
			taggerEntity: "ctr entity",
			want:         "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			taggerFunc = tt.tFunc
			if got := ServiceNameFromTags(tt.ctrName, tt.taggerEntity); got != tt.want {
				t.Errorf("ServiceNameFromTags() = %v, want %v", got, tt.want)
			}
		})
	}
}
