// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package infraattributesprocessor

import (
	taggerconsts "github.com/DataDog/datadog-agent/comp/core/tagger/taggerimpl/collectors/constants"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
)

// testTaggerClient is used to store sample tags for testing purposes
type testTaggerClient struct {
	tagMap map[string][]string
}

// newTestTaggerClient creates and returns a new testTaggerClient with an empty string map
func newTestTaggerClient() *testTaggerClient {
	return &testTaggerClient{
		tagMap: make(map[string][]string),
	}
}

// Tag mocks taggerimpl.Tag functionality for the purpose of testing, removing dependency on Taggerimpl
func (t *testTaggerClient) Tag(entityID string, _ types.TagCardinality) ([]string, error) {
	return t.tagMap[entityID], nil
}

// GlobalTags mocks taggerimpl.GlobalTags functionality for purpose of testing, removing dependency on Taggerimpl
func (t *testTaggerClient) GlobalTags(_ types.TagCardinality) ([]string, error) {
	return t.tagMap[taggerconsts.GlobalEntityID], nil
}
