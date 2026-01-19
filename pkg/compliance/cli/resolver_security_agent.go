// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build unix && !kubeapiserver

package cli

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/compliance"
)

var complianceKubernetesProvider compliance.KubernetesProvider

// Not used in the security agent. This is only for the Cluster Agent
func startComplianceReflectorStore(context.Context) *compliance.ReflectorStore {
	return nil
}
