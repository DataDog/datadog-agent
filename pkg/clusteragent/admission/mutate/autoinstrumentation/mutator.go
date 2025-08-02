// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package autoinstrumentation

import (
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	mutatecommon "github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/common"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// NewMutatorWithFilter returns a NamespaceMutator when autoinstrumentation is disabled to ensure annotation-based injection still works.
// Otherwise returns a TargetMutator for target-based workload selection.
func NewMutatorWithFilter(c *Config, wmeta workloadmeta.Component) (mutatecommon.MutatorWithFilter, error) {
	if !c.Instrumentation.Enabled {
		log.Debug("Using namespace mutator")
		return NewNamespaceMutator(c, wmeta)
	}
	log.Debug("Using target mutator")
	return NewTargetMutator(c, wmeta)
}
