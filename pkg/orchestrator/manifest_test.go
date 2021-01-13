// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2021 Datadog, Inc.

package orchestrator

import (
	"testing"

	model "github.com/DataDog/agent-payload/process"
	"github.com/stretchr/testify/assert"
)

func TestChunkManifests(t *testing.T) {
	manifests := []*model.Manifest{
		{
			Uid: "1",
		},
		{
			Uid: "2",
		},
		{
			Uid: "3",
		},
		{
			Uid: "4",
		},
		{
			Uid: "5",
		},
	}
	expected := [][]*model.Manifest{
		{{
			Uid: "1",
		},
			{
				Uid: "2",
			}},
		{{
			Uid: "3",
		},
			{
				Uid: "4",
			}},
		{{
			Uid: "5",
		}},
	}
	actual := chunkManifests(manifests, 3, 2)
	assert.ElementsMatch(t, expected, actual)
}
