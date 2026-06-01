// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package common

import (
	datastreamseventplatform "github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/datastreams/eventplatform"
	doeventplatform "github.com/DataDog/datadog-agent/comp/dataobs/queryactions/eventplatform"
	eventplatform "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/def"
	ncmeventplatform "github.com/DataDog/datadog-agent/comp/networkconfigmanagement/eventplatform"
	networkpatheventplatfrom "github.com/DataDog/datadog-agent/comp/networkpath/eventplatfrom"
	softinveventplatform "github.com/DataDog/datadog-agent/comp/softwareinventory/eventplatform"
	syntheticseventplatform "github.com/DataDog/datadog-agent/comp/syntheticstestscheduler/eventplatform"
	kubeactionseventplatform "github.com/DataDog/datadog-agent/pkg/clusteragent/kubeactions/eventplatform"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	containereventplatform "github.com/DataDog/datadog-agent/pkg/containerlifecycle/eventplatform"
	dbmeventplatform "github.com/DataDog/datadog-agent/pkg/databasemonitoring/eventplatform"
	ndmeventplatform "github.com/DataDog/datadog-agent/pkg/networkdevice/metadata/eventplatform"
)

// AllTeamEPDescs returns event platform pipeline descriptions from all team-owned packages.
// Pass the result to eventplatformimpl.Diagnose() or local.Run() so those paths cover every pipeline.
func AllTeamEPDescs(cfg pkgconfigmodel.Reader) []eventplatform.PipelineDesc {
	var descs []eventplatform.PipelineDesc
	descs = append(descs, dbmeventplatform.Descs()...)
	descs = append(descs, ndmeventplatform.Descs()...)
	descs = append(descs, networkpatheventplatfrom.Descs(cfg)...)
	descs = append(descs, ncmeventplatform.Descs(cfg)...)
	descs = append(descs, containereventplatform.Descs()...)
	descs = append(descs, syntheticseventplatform.Descs(cfg)...)
	descs = append(descs, datastreamseventplatform.Descs(cfg)...)
	descs = append(descs, doeventplatform.Descs(cfg)...)
	descs = append(descs, kubeactionseventplatform.Descs()...)
	descs = append(descs, softinveventplatform.Descs(cfg)...)
	return descs
}
