// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// This combination of build tags ensures that this file is only included in the Cluster Agent
//go:build clusterchecks && kubeapiserver

// Package commonchecks contains the checks for the Cluster Agent
package commonchecks

import (
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/flare"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	traceroute "github.com/DataDog/datadog-agent/comp/networkpath/traceroute/def"
	"github.com/DataDog/datadog-agent/comp/remote-config/rcclient"
	snmpscanmanager "github.com/DataDog/datadog-agent/comp/snmpscanmanager/def"
	corecheckLoader "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/helm"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/ksm"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/kubernetesapiserver"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator"
)

// RegisterChecks registers the checks that can run in the Cluster Agent
func RegisterChecks(store workloadmeta.Component, _ workloadfilter.Component, tagger tagger.Component, cfg config.Component,
	_ telemetry.Component, _ rcclient.Component, _ flare.Component, _ snmpscanmanager.Component, _ traceroute.Component) {
	corecheckLoader.RegisterCheck(kubernetesapiserver.CheckName, kubernetesapiserver.Factory(tagger))
	corecheckLoader.RegisterCheck(ksm.CheckName, ksm.Factory(tagger, store))
	corecheckLoader.RegisterCheck(helm.CheckName, helm.Factory())
	corecheckLoader.RegisterCheck(orchestrator.CheckName, orchestrator.Factory(store, cfg, tagger))
}
