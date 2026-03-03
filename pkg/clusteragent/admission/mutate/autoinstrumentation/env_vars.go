// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package autoinstrumentation

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// EnvNames
	instrumentationInstallTypeEnvVarName = "DD_INSTRUMENTATION_INSTALL_TYPE"
	instrumentationInstallTimeEnvVarName = "DD_INSTRUMENTATION_INSTALL_TIME"
	instrumentationInstallIDEnvVarName   = "DD_INSTRUMENTATION_INSTALL_ID"

	// Values for Env variable DD_INSTRUMENTATION_INSTALL_TYPE
	singleStepInstrumentationInstallType   = "k8s_single_step"
	localLibraryInstrumentationInstallType = "k8s_lib_injection"
)

// envVar is a containerMutator that can append/prepend an
// [[corev1.EnvVar]] to a container.
//
// This is different from using mutate/common.InjectEnv:
//  1. InjectEnv applies to _all_ containers in a pod.
//  2. InjectEnv has no mechamism to merge values from an existing
//     [[corev1.EnvVar]], while we need that for [[instrumentationV1]]
//     for the time being.
//  3. Legacy benavior here is _append_ while InjectEnv is _prepend_.
//  4. [[envVar]] supports both behaviors via the [[envVar.prepend]]
//     flag.
type envVar struct {
	// key is the name of the env var, strictly matching to [[corev1.EnvVar.Name]].
	key string

	// valFunc is used to merge environment variable values, with existing being
	// provided as an argument to [[envValFunc]].
	valFunc envValFunc

	// rawEnvVar, if provided will supercede [[valFunc]] for merging.
	rawEnvVar *corev1.EnvVar

	// isEligibleToInject gives the envVar a containerFilter (used for dotnet) in
	// [[instrumentationV1]].
	isEligibleToInject containerFilter

	// prepend, if set to true will prepend the env var instead of appending
	// it to the container Env slice.
	prepend bool

	// dontOverwrite, if set, if the existing env var is found we will
	// not overwrite it. This keeps parity with the InjectEnv implementation.
	dontOverwrite bool
}

// updateEnvVar provides the current corev1.EnvVar to set
// with whatever transform we've added to the [[envVar]]
// mutator.
func (e envVar) updateEnvVar(out *corev1.EnvVar) error {
	if e.rawEnvVar != nil {
		*out = *e.rawEnvVar
		return nil
	}

	if out.ValueFrom != nil {
		return fmt.Errorf("%q is defined via ValueFrom, update not supported", e.key)
	}

	if e.valFunc == nil {
		log.Warnf("skipping update of env var %q, no value provided", e.key)
		return nil
	}

	out.Value = e.valFunc(out.Value)
	return nil
}

// mutateContainer implements containerMutator for envVar.
func (e envVar) mutateContainer(c *corev1.Container) error {
	if e.isEligibleToInject != nil && !e.isEligibleToInject(c) {
		return nil
	}

	for idx, env := range c.Env {
		if env.Name != e.key {
			continue
		}
		if e.dontOverwrite {
			return nil
		}
		if err := e.updateEnvVar(&env); err != nil {
			return err
		}
		c.Env[idx] = env
		return nil
	}

	env := corev1.EnvVar{Name: e.key}
	if err := e.updateEnvVar(&env); err != nil {
		return err
	}

	c.Env = appendOrPrepend(env, c.Env, e.prepend)
	return nil
}

// envValFunc is a callback used in [[envVar]] to merge existing
// values in environment values with previous ones if they were set.
//
// The input value to this callback function is the original env.Value
// and will be empty string if there is no previous value.
type envValFunc func(string) string

func identityValFunc(s string) envValFunc {
	return func(string) string { return s }
}

func valueOrZero[T any](pointer *T) T {
	var val T
	if pointer != nil {
		val = *pointer
	}
	return val
}
