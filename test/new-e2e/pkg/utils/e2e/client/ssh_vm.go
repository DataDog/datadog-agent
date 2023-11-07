// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package client

import (
	"testing"

	"github.com/DataDog/test-infra-definitions/common/utils"
	commonos "github.com/DataDog/test-infra-definitions/components/os"
)

var _ connectionInitializer = (*SSHVM)(nil)
var _ VM = (*SSHVM)(nil)

// SSHVM is a type that implements [e2e.client.VM] and uses an SSH connection
// to setup the connection with the VM.
type SSHVM struct {
	VM

	vmName string
	osType commonos.Type
}

func NewSSHVM(vmName string, osType commonos.Type) *SSHVM {
	return &SSHVM{vmName: vmName, osType: osType}
}

// initFromConnection initializes the instance from an ssh connection to the test VM
// This method is called by [CallConnectionInitializers] using reflection.
//
//lint:ignore U1000 Ignore unused function as this function is called using reflection
func (vm *SSHVM) initFromConnection(t *testing.T, conns map[string]*utils.Connection) error {
	conn := conns[vm.vmName]

	var err error
	vm.VM, err = NewVMClient(t, conn, vm.osType)
	return err
}
