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

// NewMutatorWithFilter handles the dependency injection for the mutator. If a targets list is defined, it will return
// a target mutator, otherwise it will return a namespace mutator.
func NewMutatorWithFilter(c *Config, wmeta workloadmeta.Component, imageResolver ImageResolver) (mutatecommon.MutatorWithFilter, error) {
	if len(c.Instrumentation.Targets) > 0 {
		log.Debug("Using target mutator")
		return NewTargetMutator(c, wmeta, imageResolver)
	}

	log.Debug("Using namespace mutator")
	return NewNamespaceMutator(c, wmeta, imageResolver)
}
