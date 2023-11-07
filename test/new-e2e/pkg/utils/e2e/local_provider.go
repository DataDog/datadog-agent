// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package e2e

import (
	"context"
	"fmt"
	"testing"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/localinfra"
)

var _ InfraProvider[int] = (*LocalProvider[int])(nil)

// LocalProvider creates and manages local testing infrastructure
type LocalProvider[Env any] struct {
	envFactory func(vmManager *localinfra.LocalVMManager) (*Env, error)

	vmManager *localinfra.LocalVMManager
}

// NewLocalProvider returns a new LocalProvider
func NewLocalProvider[Env any](envFactory func(vmManager *localinfra.LocalVMManager) (*Env, error)) *LocalProvider[Env] {
	return &LocalProvider[Env]{
		envFactory: envFactory,
		vmManager:  localinfra.GetLocalVMManager(),
	}
}

// ProvisionInfraAndInitializeEnv uses a local VM manager to provision a VM & pass the resulting SSH connection to
// any clients in the environment which implement the connectionInitializer interface
func (li *LocalProvider[Env]) ProvisionInfraAndInitializeEnv(_ context.Context, t *testing.T, _ string, _ bool) (*Env, error) {
	env, err := li.envFactory(li.vmManager)
	if err != nil {
		return nil, fmt.Errorf("error instantiating env: %s", err)
	}

	connResult, err := li.vmManager.Provision()
	if err != nil {
		return nil, fmt.Errorf("error provisioning local testing infra: %s", err)
	}

	err = client.CallConnectionInitializers(t, env, connResult)
	return env, err
}

// DeleteInfra deletes all local VMs
func (li *LocalProvider[Env]) DeleteInfra(_ context.Context, _ string) error {
	allErrs := li.vmManager.Delete()
	if len(allErrs) > 0 {
		return fmt.Errorf("%d error(s) deleting local testing infra: %v", len(allErrs), allErrs)
	}
	return nil
}
