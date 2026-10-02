// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_kubernetes_apps

import (
	"context"
	"encoding/json"
	"testing"

	testhelpers "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundle-support/test"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"github.com/stretchr/testify/suite"
)

var (
	testContext       = context.TODO()
	testOrgID   int64 = 123
)

type AppsTestSuite struct {
	suite.Suite
}

func TestApps(t *testing.T) {
	suite.Run(t, new(AppsTestSuite))
}

func newTestTask(inputs map[string]any) *types.Task {
	return testhelpers.NewTestTask("id", "type", &types.Attributes{
		Name:                  "Test Task",
		BundleID:              "com.test.bundle",
		SecDatadogHeaderValue: "test-header-value",
		Inputs:                inputs,
		OrgId:                 testOrgID,
	})
}

func newTestCredentials() *privateconnection.PrivateCredentials {
	return &privateconnection.PrivateCredentials{
		Type:   privateconnection.TokenAuthType,
		Tokens: []privateconnection.PrivateCredentialsToken{},
	}
}

func (suite *AppsTestSuite) assertJSONPatch(patchBytes []byte, expectedUpdates []map[string]interface{}) {
	var patch map[string]interface{}
	suite.NoError(json.Unmarshal(patchBytes, &patch))

	// Extract containers from patch: spec.template.spec.containers
	containers := patch["spec"].(map[string]interface{})["template"].(map[string]interface{})["spec"].(map[string]interface{})["containers"].([]interface{})
	suite.Len(containers, len(expectedUpdates))

	// Validate each container
	for i, expected := range expectedUpdates {
		container := containers[i].(map[string]interface{})
		suite.Equal(expected["containerName"], container["name"])

		if expectedResources, ok := expected["resources"]; ok {
			suite.Equal(expectedResources, container["resources"])
		}
	}
}
