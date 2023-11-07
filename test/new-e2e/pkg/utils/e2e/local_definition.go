// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package e2e

import (
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/localinfra"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/localinfra/localvmparams"
)

// LocalVMDef creates a test environment containing a local virtual machine.
// See [localvmparams.Params] for available options.
func LocalVMDef(options ...localvmparams.Option) InfraProvider[VMEnv] {
	return NewLocalProvider(
		func(vmManager *localinfra.LocalVMManager) (*VMEnv, error) {
			vm, err := localinfra.NewLocalVM(options...)
			if err != nil {
				return nil, err
			}
			vmManager.AddVM(vm)

			return &VMEnv{
				VM: client.NewSSHVM(vm.Name(), vm.OSType()),
			}, nil
		},
	)
}
