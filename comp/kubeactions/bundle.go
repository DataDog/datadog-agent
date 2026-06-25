// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

// Package kubeactions implements the "kubeactions" bundle, providing components
// for executing Kubernetes and Helm actions on behalf of the cluster agent.
package kubeactions

import (
	helmactionsfx "github.com/DataDog/datadog-agent/comp/kubeactions/helmactions/fx"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// team: container-integrations

// Bundle defines the fx options for this bundle.
func Bundle() fxutil.BundleOptions {
	return fxutil.Bundle(
		helmactionsfx.Module(),
		// TODO: add kubeactions as well when migrate to PAR
	)
}
