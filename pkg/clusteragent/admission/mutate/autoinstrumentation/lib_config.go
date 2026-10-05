// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package autoinstrumentation

import (
	"encoding/json"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/common"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/autoinstrumentation/annotation"
	mutatecommon "github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/common"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/patch"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
)

// basicConfig returns the default tracing config to inject into application pods
// when no other config has been provided.
func basicConfig() common.LibConfig {
	return common.LibConfig{
		Tracing:        pointer.Ptr(true),
		LogInjection:   pointer.Ptr(true),
		HealthMetrics:  pointer.Ptr(true),
		RuntimeMetrics: pointer.Ptr(true),
	}
}

type basicLibConfigInjector struct{}

func (basicLibConfigInjector) planPod(session *patch.PodSession) error {
	libConfig := basicConfig()
	for _, env := range libConfig.ToEnvs() {
		if _, err := mutatecommon.PatchInjectEnv(session, env); err != nil {
			return err
		}
	}

	return nil
}

// containerMutator returns a containerMutator that injects the basic lib config env vars.
// This can be used with filteredContainerMutator to apply container filtering.
func (basicLibConfigInjector) containerMutator() containerMutator {
	libConfig := basicConfig()
	envs := libConfig.ToEnvs()

	var mutators containerMutators
	for _, env := range envs {
		mutators = append(mutators, envVarMutator(env))
	}
	return mutators
}

type libConfigInjector struct{}

func (l *libConfigInjector) podMutator(lang language) podMutator {
	return podMutatorFunc(func(session *patch.PodSession) error {
		pod, err := session.Snapshot()
		if err != nil {
			return err
		}
		config, found := annotation.Get(pod, annotation.LibraryConfigV1.Format(string(lang)))
		if !found {
			return nil
		}

		c, err := parseConfigJSON(config)
		if err != nil {
			return err
		}

		for _, env := range c.ToEnvs() {
			if _, err := mutatecommon.PatchInjectEnv(session, env); err != nil {
				return err
			}
		}

		return nil
	})
}

func parseConfigJSON(in string) (common.LibConfig, error) {
	var c common.LibConfig
	return c, json.Unmarshal([]byte(in), &c)
}
