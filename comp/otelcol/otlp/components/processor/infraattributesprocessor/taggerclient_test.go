// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package infraattributesprocessor

import (
	"github.com/DataDog/datadog-agent/comp/core/tagger/taggerimpl/collectors"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
)

type testTaggerClient struct {
	m map[string][]string
}

func (t *testTaggerClient) Tag(entityID string, _ types.TagCardinality) ([]string, error) {
	return t.m[entityID], nil
}
func (t *testTaggerClient) GlobalTags(_ types.TagCardinality) ([]string, error) {
	return t.m[collectors.GlobalEntityID], nil
}
