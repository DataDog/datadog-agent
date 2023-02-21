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
