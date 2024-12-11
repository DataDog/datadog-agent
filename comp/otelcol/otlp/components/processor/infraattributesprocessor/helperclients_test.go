// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package infraattributesprocessor

import (
	"fmt"

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
func (t *testTaggerClient) Tag(entityID types.EntityID, _ types.TagCardinality) ([]string, error) {
	return t.tagMap[entityID.String()], nil
}

// GlobalTags mocks taggerimpl.GlobalTags functionality for purpose of testing, removing dependency on Taggerimpl
func (t *testTaggerClient) GlobalTags(_ types.TagCardinality) ([]string, error) {
	return t.tagMap[types.NewEntityID("internal", "global-entity-id").String()], nil
}

type testGenerateIDClient struct{}

func newTestGenerateIDClient() *testGenerateIDClient {
	return &testGenerateIDClient{}
}

func (t *testGenerateIDClient) generateID(group, resource, namespace, name string) string {
	return fmt.Sprintf("%s/%s/%s/%s", group, resource, namespace, name)
}
