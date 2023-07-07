// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows
// +build !windows

package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	systemProbe "github.com/DataDog/datadog-agent/test/new-e2e/system-probe"
)

var DD_AGENT_TESTING_DIR = os.Getenv("DD_AGENT_TESTING_DIR")
var defaultVMConfigPath = filepath.Join(".", "system-probe", "config", "vmconfig.json")

func run(envName, x86InstanceType, armInstanceType string, destroy bool, opts *systemProbe.SystemProbeEnvOpts) error {
	if destroy {
		return systemProbe.Destroy(envName)
	}

	systemProbeEnv, err := systemProbe.NewTestEnv(envName, x86InstanceType, armInstanceType, opts)
	if err != nil {
		return err
	}

	fmt.Println(systemProbeEnv.ARM64InstanceIP)
	fmt.Println(systemProbeEnv.X86_64InstanceIP)

	return nil
}

func main() {
	envNamePtr := flag.String("name", "system-probe", "environment name")
	destroyPtr := flag.Bool("destroy", false, "[optional] should destroy the environment")
	x86InstanceTypePtr := flag.String("instance-type-x86", "", "x86_64 instance type")
	armInstanceTypePtr := flag.String("instance-type-arm", "", "arm64 instance type")
	x86AmiIDPtr := flag.String("x86-ami-id", "", "x86 ami for metal instance")
	armAmiIDPtr := flag.String("arm-ami-id", "", "arm ami for metal instance")
	toProvisionPtr := flag.Bool("run-provision", true, "run provision step for metal instance")
	shutdownPtr := flag.Int("shutdown-period", 0, "shutdown after specified interval in minutes")
	sshKeyFile := flag.String("ssh-key-path", "", "path of private ssh key for ec2 instances")
	sshKeyName := flag.String("ssh-key-name", "", "name of ssh key pair to use for ec2 instances")
	infraEnv := flag.String("infra-env", "", "name of infra env to use")
	dependenciesDirectoryPtr := flag.String("dependencies-dir", DD_AGENT_TESTING_DIR, "directory where dependencies package is present")
	subnetsPtr := flag.String("subnets", "", "list of subnets to use")
	vmconfigPathPtr := flag.String("vmconfig", defaultVMConfigPath, "vmconfig path")
	local := flag.Bool("local", false, "is scenario running locally")

	flag.Parse()

	var failOnMissing bool
	if *destroyPtr {
		failOnMissing = true
	}

	opts := systemProbe.SystemProbeEnvOpts{
		X86AmiID:              *x86AmiIDPtr,
		ArmAmiID:              *armAmiIDPtr,
		ShutdownPeriod:        *shutdownPtr,
		Provision:             *toProvisionPtr,
		FailOnMissing:         failOnMissing,
		SSHKeyPath:            *sshKeyFile,
		SSHKeyName:            *sshKeyName,
		InfraEnv:              *infraEnv,
		DependenciesDirectory: *dependenciesDirectoryPtr,
		Subnets:               *subnetsPtr,
		VMConfigPath:          *vmconfigPathPtr,
		Local:                 *local,
	}

	fmt.Printf("shutdown period: %d\n", opts.ShutdownPeriod)

	err := run(*envNamePtr, *x86InstanceTypePtr, *armInstanceTypePtr, *destroyPtr, &opts)
	if err != nil {
		log.Fatal(err)
	}
}
