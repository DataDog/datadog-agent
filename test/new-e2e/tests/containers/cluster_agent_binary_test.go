// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package containers

import (
	"context"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
)

// TestSecretGenericConnectorPresenceAndPermissions verifies that the
// secret-generic-connector binary is present at the expected path in the
// cluster agent image with the correct permissions (550) and ownership
// (root:secret-manager).
func (suite *k8sSuite) TestSecretGenericConnectorPresenceAndPermissions() {
	pods, err := suite.Env().KubernetesCluster.Client().CoreV1().Pods("datadog").List(context.Background(), metav1.ListOptions{
		LabelSelector: fields.OneTermEqualSelector("app", suite.Env().Agent.LinuxClusterAgent.LabelSelectors["app"]).String(),
		Limit:         1,
	})
	suite.Require().NoError(err)
	suite.Require().NotEmpty(pods.Items, "cluster-agent pod not found in datadog namespace")

	clusterAgentPodName := pods.Items[0].Name

	// stat -c '%a %U %G' prints: <octal-perms> <owner> <group>
	stdout, stderr, err := suite.Env().KubernetesCluster.KubernetesClient.
		PodExec("datadog", clusterAgentPodName, "cluster-agent",
			[]string{"stat", "-c", "%a %U %G", "/opt/datadog-agent/bin/secret-generic-connector"})
	suite.Require().NoError(err, "stat failed (binary may be missing): %s", stderr)

	parts := strings.Fields(strings.TrimSpace(stdout))
	suite.Require().Lenf(parts, 3, "expected 'perms owner group' from stat, got: %q", stdout)

	suite.Equal("550", parts[0], "unexpected permissions")
	suite.Equal("root", parts[1], "unexpected owner")
	suite.Equal("secret-manager", parts[2], "unexpected group")
}
