// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package localinfra

import (
	"encoding/json"
	"fmt"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/localinfra/localvmparams"
	"io"
	"math/rand"
	"os"
	"strconv"
	"time"

	"github.com/DataDog/test-infra-definitions/common/utils"
	commonos "github.com/DataDog/test-infra-definitions/components/os"
)

// LocalVM represents a local virtual machine
type LocalVM struct {
	params *localvmparams.Params
	name   string

	// VM state
	isUp bool
	conn *utils.Connection
}

// NewLocalVM instantiates a new LocalVM definition
func NewLocalVM(options ...localvmparams.Option) (*LocalVM, error) {
	params, err := localvmparams.NewParams(options...)
	if err != nil {
		return nil, err
	}

	// name is a pseudo-random string which can be used to uniquely identify this VM
	seededRand := rand.New(rand.NewSource(time.Now().UnixNano()))
	name := strconv.Itoa(seededRand.Int())

	vm := &LocalVM{params: params, name: name, isUp: false}

	if params.JSONFile != "" {
		err := vm.initFromJSONFile(params.JSONFile)
		if err != nil {
			return nil, err
		}
	}

	return vm, nil
}

// Name returns the unique name of this VM
func (v *LocalVM) Name() string {
	return v.name
}

// OSType returns the os type of this VM
func (v *LocalVM) OSType() commonos.Type {
	return v.params.OSType
}

func (v *LocalVM) initFromJSONFile(jsonFile string) error {
	type localEnvSSHConfig struct {
		User    string `json:"ssh_user"`
		Address string `json:"ssh_address"`
	}

	file, err := os.Open(jsonFile)
	if err != nil {
		return fmt.Errorf("error opening %s: %s", jsonFile, err)
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		return fmt.Errorf("error reading %s: %s", jsonFile, err)
	}

	var sshConfig localEnvSSHConfig
	err = json.Unmarshal(data, &sshConfig)
	if err != nil {
		return fmt.Errorf("error unmarshalling data in %s: %s", jsonFile, err)
	}

	v.conn = &utils.Connection{User: sshConfig.User, Host: sshConfig.Address}
	v.isUp = true
	return nil
}

func (v *LocalVM) getConnection() (*utils.Connection, error) {
	if !v.isUp {
		return nil, fmt.Errorf("error getting SSH connection to VM: VM is not running")
	}
	if v.conn == nil {
		return nil, fmt.Errorf("error getting SSH connection to VM: VM is up, but SSH connection has not been established")
	}
	return v.conn, nil
}
