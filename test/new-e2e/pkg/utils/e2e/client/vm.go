// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package client contains clients used to communicate with the remote service
package client

import (
	"os"
	"testing"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner/parameters"
	"github.com/DataDog/test-infra-definitions/common/utils"
	commonvm "github.com/DataDog/test-infra-definitions/components/vm"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
)

var _ stackInitializer = (*VM)(nil)

// VM is a client VM that is connected to a VM defined in test-infra-definition.
type VM struct {
	deserializer utils.RemoteServiceDeserializer[commonvm.ClientData]
	*vmClient
}

// NewVM creates a new instance of VM
func NewVM(infraVM commonvm.VM) *VM {
	return &VM{deserializer: infraVM}
}

//lint:ignore U1000 Ignore unused function as this function is called using reflection
func (vm *VM) setStack(t *testing.T, stackResult auto.UpResult) error {
	clientData, err := vm.deserializer.Deserialize(stackResult)
	if err != nil {
		return err
	}

	var privateSSHKey []byte

	privateKeyPath, err := runner.GetProfile().ParamStore().GetWithDefault(parameters.PrivateKeyPath, "")
	if err != nil {
		return err
	}

	if privateKeyPath != "" {
		privateSSHKey, err = os.ReadFile(privateKeyPath)
		if err != nil {
			return err
		}
	}

	vm.vmClient, err = newVMClient(t, privateSSHKey, &clientData.Connection)
	return err
}
