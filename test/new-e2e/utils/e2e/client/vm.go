package client

import (
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/test/new-e2e/utils/clients"
	commonvm "github.com/DataDog/test-infra-definitions/common/vm"
	"golang.org/x/crypto/ssh"
)

var _ stackInitializer = (*VM)(nil)

// A client VM that is connected to a VM defined in test-infra-definition.
type VM struct {
	*UpResultDeserializer[commonvm.VMData]
	client *ssh.Client
}

// Create a new instance of VM
func NewVM(infraVM commonvm.VM) *VM {
	vm := &VM{}
	vm.UpResultDeserializer = NewUpResultDeserializer(infraVM.GetConnectionDeserializer(), vm.initConnection)
	return vm
}

func (vm *VM) initConnection(auth *Authentification, vmData *commonvm.VMData) error {
	var err error
	vm.client, _, err = clients.GetSSHClient(
		vmData.Connection.User,
		fmt.Sprintf("%s:%d", vmData.Connection.Host, 22),
		auth.SSHKey,
		2*time.Second, 5)
	return err
}

// Execute a command
func (vm *VM) Execute(command string) (string, error) {
	return clients.ExecuteCommand(vm.client, command)
}
