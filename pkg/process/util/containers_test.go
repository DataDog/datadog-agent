// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package util

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/util/containers"
)

func Test_dedupeContainers(t *testing.T) {
	tests := []struct {
		name string
		ctrs []*containers.Container
		want []*containers.Container
	}{
		{
			name: "dedupe",
			ctrs: []*containers.Container{
				{
					ID: "ctr1",
				},
				{
					ID: "ctr2",
				},
				{
					ID: "ctr2",
				},
			},
			want: []*containers.Container{
				{
					ID: "ctr1",
				},
				{
					ID: "ctr2",
				},
			},
		},
		{
			name: "no dedupe",
			ctrs: []*containers.Container{
				{
					ID: "ctr1",
				},
				{
					ID: "ctr2",
				},
			},
			want: []*containers.Container{
				{
					ID: "ctr1",
				},
				{
					ID: "ctr2",
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.ElementsMatch(t, tt.want, dedupeContainers(tt.ctrs))
		})
	}
}
