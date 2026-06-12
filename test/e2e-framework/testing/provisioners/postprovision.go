// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package provisioners

import (
	"context"
	"testing"
)

// InstallFunc performs Pulumi-free post-provisioning setup (agent install,
// workload deploy) on a fully-populated environment. It is called by
// BaseSuite.SetupSuite after Pulumi completes and the environment is
// initialized.
type InstallFunc[Env any] func(t *testing.T, env *Env)

type postProvisionWrapper[Env any] struct {
	TypedProvisioner[Env]
	install InstallFunc[Env]
}

func (w *postProvisionWrapper[Env]) PostProvision(t *testing.T, env *Env) {
	if w.install != nil {
		w.install(t, env)
	}
}

// Diagnose forwards to the wrapped provisioner so provisioning failures are
// still diagnosed correctly.
func (w *postProvisionWrapper[Env]) Diagnose(ctx context.Context, stackName string) (string, error) {
	if d, ok := w.TypedProvisioner.(Diagnosable); ok {
		return d.Diagnose(ctx, stackName)
	}
	return "", nil
}

// WithPostProvision decorates a Pulumi provisioner with a Pulumi-free install
// step that runs after the environment is populated. The install func receives
// the fully-initialized *Env and may call installer packages (hostagent,
// helmagent, workloads, …) without touching Pulumi.
//
// Usage:
//
//	provisioners.WithPostProvision(pulumiProv, func(t *testing.T, env *environments.Host) {
//	    hostagent.Install(t, env, agentparams.WithAgentConfig("log_level: debug"))
//	})
func WithPostProvision[Env any](p TypedProvisioner[Env], install InstallFunc[Env]) TypedProvisioner[Env] {
	return &postProvisionWrapper[Env]{TypedProvisioner: p, install: install}
}
