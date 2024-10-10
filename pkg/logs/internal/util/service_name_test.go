// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package util

import (
	"errors"
	"testing"

	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
)

func TestServiceNameFromTags(t *testing.T) {
	tests := []struct {
		name         string
		tFunc        func(types.EntityID) ([]string, error)
		ctrName      string
		taggerEntity types.EntityID
		want         string
	}{
		{
			name: "nominal case",
			tFunc: func(types.EntityID) ([]string, error) {
				return []string{"env:foo", "service:bar"}, nil
			},
			ctrName:      "ctr-name",
			taggerEntity: types.NewEntityID(types.ContainerID, "ctrId"),
			want:         "bar",
		},
		{
			name: "tagger error",
			tFunc: func(types.EntityID) ([]string, error) {
				return nil, errors.New("err")
			},
			ctrName:      "ctr-name",
			taggerEntity: types.NewEntityID(types.ContainerID, "ctrId"),
			want:         "",
		},
		{
			name: "not found",
			tFunc: func(types.EntityID) ([]string, error) {
				return []string{"env:foo", "version:bar"}, nil
			},
			ctrName:      "ctr-name",
			taggerEntity: types.NewEntityID(types.ContainerID, "ctrId"),
			want:         "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ServiceNameFromTags(tt.ctrName, tt.taggerEntity, tt.tFunc); got != tt.want {
				t.Errorf("ServiceNameFromTags() = %v, want %v", got, tt.want)
			}
		})
	}
}
