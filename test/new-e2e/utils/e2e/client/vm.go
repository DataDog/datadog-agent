// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package client

import (
	commonvm "github.com/DataDog/test-infra-definitions/common/vm"
)

var _ stackInitializer = (*VM)(nil)

// A client VM that is connected to a VM defined in test-infra-definition.
type VM struct {
	*UpResultDeserializer[commonvm.VMData]
	*sshClient
}

// Create a new instance of VM
func NewVM(infraVM commonvm.VM) *VM {
	vm := &VM{}
	vm.UpResultDeserializer = NewUpResultDeserializer(infraVM.GetClientDataDeserializer(), vm.init)
	return vm
}

func (vm *VM) init(auth *Authentification, vmData *commonvm.VMData) error {
	var err error
	vm.sshClient, err = newSSHClient(auth, &vmData.Connection)
	return err
}
