// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && orchestrator
// +build kubeapiserver,orchestrator

package processors

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

type Item struct {
	UID string
}

func TestChunkItems(t *testing.T) {
	items := []interface{}{
		Item{UID: "1"},
		Item{UID: "2"},
		Item{UID: "3"},
		Item{UID: "4"},
		Item{UID: "5"},
	}
	expected := [][]interface{}{
		{
			Item{UID: "1"},
			Item{UID: "2"},
		},
		{
			Item{UID: "3"},
			Item{UID: "4"},
		},
		{
			Item{UID: "5"},
		},
	}

	actual := chunkResources(items, 3, 2)
	assert.ElementsMatch(t, expected, actual)
}
