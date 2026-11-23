// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package registry

import (
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors"
	k8sProcessors "github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors/k8s"
	utilTypes "github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/util"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
)

// The functions below are the handlers and the default selection of the registry entries that do not use the defaults.

func setting(key string) func() bool {
	return func() bool { return pkgconfigsetup.Datadog().GetBool(key) }
}

// Never is the default selection of the resources that are only collected when the check configuration selects them.
func Never() bool { return false }

var ootbEnabled = setting("orchestrator_explorer.custom_resources.ootb.enabled")

func gatewayAPIEnabled() bool {
	return ootbEnabled() && pkgconfigsetup.Datadog().GetBool("orchestrator_explorer.custom_resources.ootb.gateway_api")
}

func serviceMeshEnabled() bool {
	return ootbEnabled() && pkgconfigsetup.Datadog().GetBool("orchestrator_explorer.custom_resources.ootb.service_mesh")
}

func ingressControllersEnabled() bool {
	return ootbEnabled() && pkgconfigsetup.Datadog().GetBool("orchestrator_explorer.custom_resources.ootb.ingress_controllers")
}

func podHandlers(d Dependencies, _ string) processors.Handlers {
	return k8sProcessors.NewPodHandlers(d.Config, d.Store, d.Tagger)
}

func cronJobHandlers(d Dependencies, version string) processors.Handlers {
	if version == utilTypes.CronJobVersionV1Beta1 {
		return k8sProcessors.NewCronJobV1Beta1Handlers(d.Tagger)
	}
	return k8sProcessors.NewCronJobV1Handlers(d.Tagger)
}

func crdHandlers(Dependencies, string) processors.Handlers {
	return new(k8sProcessors.CRDHandlers)
}
