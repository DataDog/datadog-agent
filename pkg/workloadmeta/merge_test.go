// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package workloadmeta

import (
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/pointer"
	"github.com/stretchr/testify/assert"
)

func TestMerge(t *testing.T) {
	testTime := time.Now()

	fromSource1 := Container{
		EntityID: EntityID{
			Kind: KindContainer,
			ID:   "foo1",
		},
		EntityMeta: EntityMeta{
			Name:      "foo1-name",
			Namespace: "",
		},
		State: ContainerState{
			Running:    true,
			CreatedAt:  testTime,
			StartedAt:  testTime,
			FinishedAt: time.Time{},
		},
	}

	fromSource2 := Container{
		EntityID: EntityID{
			Kind: KindContainer,
			ID:   "foo1",
		},
		EntityMeta: EntityMeta{
			Name:      "foo1-name",
			Namespace: "",
		},
		State: ContainerState{
			CreatedAt:  time.Time{},
			StartedAt:  time.Time{},
			FinishedAt: time.Time{},
			ExitCode:   pointer.UInt32Ptr(100),
		},
	}

	expectedContainer := Container{
		EntityID: EntityID{
			Kind: KindContainer,
			ID:   "foo1",
		},
		EntityMeta: EntityMeta{
			Name:      "foo1-name",
			Namespace: "",
		},
		State: ContainerState{
			Running:    true,
			CreatedAt:  testTime,
			StartedAt:  testTime,
			FinishedAt: time.Time{},
			ExitCode:   pointer.UInt32Ptr(100),
		},
	}

	// Test merging both ways
	err := merge(&fromSource1, &fromSource2)
	assert.NoError(t, err)
	assert.Equal(t, expectedContainer, fromSource1)

	err = merge(&fromSource2, &fromSource1)
	assert.NoError(t, err)
	assert.Equal(t, expectedContainer, fromSource2)
}
