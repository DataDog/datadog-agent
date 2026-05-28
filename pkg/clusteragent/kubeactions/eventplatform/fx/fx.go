// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package fx contributes the container integrations team's Kubernetes Actions
// event platform pipeline description to the event platform forwarder via the
// "ep_pipeline_descs" fx group.
package fx

// team: container-integrations

import (
	"go.uber.org/fx"

	kubeactionseventplatform "github.com/DataDog/datadog-agent/pkg/clusteragent/kubeactions/eventplatform"
)

// Module returns the fx module that registers the Kube Actions pipeline description.
//
// The slice returned by kubeactionseventplatform.Descs() is flattened into the fx
// group. Descs() returns nil when kubeactions.enabled is false, so no pipeline is
// registered in that case.
func Module() fx.Option {
	return fx.Module(
		"pkg/clusteragent/kubeactions/eventplatform",
		fx.Provide(fx.Annotate(
			kubeactionseventplatform.Descs,
			fx.ResultTags(`group:"ep_pipeline_descs,flatten"`),
		)),
	)
}
