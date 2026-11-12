// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cspm

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/kubernetesagentparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	azurekubernetes "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/azure/kubernetes"
)

type aksTestSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
}

func TestAKSCSPM(t *testing.T) {
	e2e.Run(t, &aksTestSuite{}, e2e.WithProvisioner(
		azurekubernetes.AKSProvisioner(
			azurekubernetes.WithAgentOptions(kubernetesagentparams.WithHelmValues(complianceValues)),
		),
	))
}

// TestAzureJSON checks the data behind CIS AKS 3.1.3 and 3.1.4 on an AKS
// node: the node configuration reports azure.json with the ownership and
// permissions stat(1) sees, and keeps its content, the cluster credentials,
// out of the payload.
func (s *aksTestSuite) TestAzureJSON() {
	pod := nodeAgentPod(s.T(), s.Env())
	typ, out := loadNodeConfig(s.T(), s.Env(), pod)
	assert.Equal(s.T(), "azure_aks_worker_node", typ)
	assert.NotContains(s.T(), out, "aadClientSecret")

	var node nodeConfig
	require.NoError(s.T(), json.Unmarshal([]byte(out), &node), out)
	assertFileMeta(s.T(), statFile(s.T(), s.Env(), pod, "/etc/kubernetes/azure.json"), node.AzureJSON)
}
