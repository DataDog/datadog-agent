// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_kubernetes_watermarkpodautoscaler

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"

	testhelpers "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundle-support/test"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

var testContext = context.TODO()

type WatermarkPodAutoscalerTestSuite struct {
	suite.Suite
}

func TestWatermarkPodAutoscaler(t *testing.T) {
	suite.Run(t, new(WatermarkPodAutoscalerTestSuite))
}

func newTestTask(inputs map[string]any) *types.Task {
	return testhelpers.NewTestTask("id", "type", &types.Attributes{
		Name:                  "Test Task",
		BundleID:              "com.datadoghq.kubernetes.watermarkpodautoscaler",
		SecDatadogHeaderValue: "test-header-value",
		Inputs:                inputs,
		OrgId:                 123,
	})
}

func newTestCredentials() *privateconnection.PrivateCredentials {
	return &privateconnection.PrivateCredentials{
		Type:   privateconnection.TokenAuthType,
		Tokens: []privateconnection.PrivateCredentialsToken{},
	}
}
