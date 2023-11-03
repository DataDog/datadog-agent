// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package client contains clients used to communicate with the remote service
package client

import (
	"testing"

	"github.com/DataDog/test-infra-definitions/common/utils"
	commonos "github.com/DataDog/test-infra-definitions/components/os"
	commonvm "github.com/DataDog/test-infra-definitions/components/vm"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
)

var _ pulumiStackInitializer = (*PulumiStackVM)(nil)
var _ VM = (*PulumiStackVM)(nil)

// PulumiStackVM is a type that implements [VM] and uses the pulumi stack filled by
// [component.VM] to setup the connection with the VM.
//
// [component.VM]: https://pkg.go.dev/github.com/DataDog/test-infra-definitions@main/components/vm
type PulumiStackVM struct {
	deserializer utils.RemoteServiceDeserializer[commonvm.ClientData]
	VM
	os commonos.OS
}

// NewPulumiStackVM creates a new instance of PulumiStackVM
func NewPulumiStackVM(infraVM commonvm.VM) *PulumiStackVM {
	return &PulumiStackVM{deserializer: infraVM, os: infraVM.GetOS()}
}

// initFromPulumiStack initializes the instance from the data stored in the pulumi stack.
// This method is called by [CallStackInitializers] using reflection.
//
//lint:ignore U1000 Ignore unused function as this function is called using reflection
func (vm *PulumiStackVM) initFromPulumiStack(t *testing.T, stackResult auto.UpResult) error {
	clientData, err := vm.deserializer.Deserialize(stackResult)
	if err != nil {
		return err
	}

	vm.VM, err = NewVMClient(t, &clientData.Connection, vm.os.GetType())
	return err
}

// GetOS is a temporary method while NewVMClient and AgentNewClient require an instance of OS.
func (vm *PulumiStackVM) GetOS() commonos.OS {
	return vm.os
}
