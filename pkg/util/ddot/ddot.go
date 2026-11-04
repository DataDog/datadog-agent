// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package ddot tells which mode the DDOT Collector (otel-agent) runs in, for
// code shared with the other agents. It has no Kubernetes dependency, so that
// any binary can link it.
package ddot

import (
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
)

// IsStandalone reports whether this is otel-agent running in DDOT standalone
// mode, without a core Agent to get its configuration, hostname or cluster name
// from.
func IsStandalone(cfg pkgconfigmodel.Reader) bool {
	return flavor.GetFlavor() == flavor.OTelAgent && cfg.GetBool("otel_standalone")
}

// KubeletDisabled reports whether cfg leaves the agent without kubelet access,
// so that the local node's name must come from the environment rather than from
// the kubelet: otel-agent in DDOT standalone mode with
// otelcollector.standalone.use_kubelet_collector set to false, which watches pods
// through the API server with the nodefilter workloadmeta collector instead, and
// whose deployment lacks the kubelet API access and settings the kubelet needs.
func KubeletDisabled(cfg pkgconfigmodel.Reader) bool {
	return IsStandalone(cfg) && !cfg.GetBool("otelcollector.standalone.use_kubelet_collector")
}
