// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package helmCharts

import (
	"context"
	"fmt"
	"testing"

	"github.com/DataDog/datadog-agent/test/new-e2e/runner"
	"github.com/DataDog/datadog-agent/test/new-e2e/utils/infra"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/eks"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/stretchr/testify/require"
)

func Test_E2E_AgentOnEKS(t *testing.T) {
	// Create pulumi EKS stack
	stackConfig := runner.ConfigMap{
		"ddinfra:aws/eks/linuxBottlerocketNodeGroup": auto.ConfigValue{Value: "false"},
		"ddinfra:aws/eks/windowsNodeGroup":           auto.ConfigValue{Value: "false"},
		"pulumi:disable-default-providers":           auto.ConfigValue{Value: "[]"},
		"aws:skipCredentialsValidation":              auto.ConfigValue{Value: "true"},
		"aws:skipMetadataApiCheck":                   auto.ConfigValue{Value: "false"},
	}

	_, stackOutput, err := infra.GetStackManager().GetStack(context.Background(), "eks-e2e", stackConfig, eks.Run, false)

	if stackOutput.Outputs["kubeconfig"].Value != nil {
		kc := stackOutput.Outputs["kubeconfig"].Value.(map[string]interface{})
		fmt.Println("Kubeconfig: ", kc)
	}

	require.NoError(t, err)
}
