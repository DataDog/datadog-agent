// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tags

import (
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/clusteragent"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/clusterinfo"
)

// setStartupEKSEvidence gives TestStartupProbe the local EKS evidence of an EKS node (or none),
// with the production budgets.
func setStartupEKSEvidence(t *testing.T, onEKS bool) {
	t.Helper()
	clusteragent.ResetGlobalClusterAgentClientForTest()
	clusterinfo.SetupStartupForTest(t, clusterinfo.StartupRetryBudget, time.Second, onEKS)
}
