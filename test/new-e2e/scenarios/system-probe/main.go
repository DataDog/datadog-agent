// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	"flag"
	"fmt"
	"log"
	"time"

	systemProbe "github.com/DataDog/datadog-agent/test/new-e2e/system-probe"
)

func run(envName, x86InstanceType, armInstanceType string, destroy bool, opts *systemProbe.SystemProbeEnvOpts) error {
	systemProbeEnv, err := systemProbe.NewTestEnv(envName, x86InstanceType, armInstanceType, opts)
	if err != nil {
		return err
	}

	fmt.Println(systemProbeEnv.ARM64InstanceIP)
	fmt.Println(systemProbeEnv.X86_64InstanceIP)

	if destroy {
		return systemProbeEnv.Destroy()
	}

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
	uploadDependenciesPtr := flag.Bool("upload-dependencies", false, "upload test dependencies to microvms")

	flag.Parse()

	var failOnMissing bool
	if *destroyPtr || *uploadDependenciesPtr {
		failOnMissing = true
	}

	opts := systemProbe.SystemProbeEnvOpts{
		X86AmiID:           *x86AmiIDPtr,
		ArmAmiID:           *armAmiIDPtr,
		ShutdownPeriod:     time.Duration(*shutdownPtr) * time.Minute,
		Provision:          *toProvisionPtr,
		FailOnMissing:      failOnMissing,
		UploadDependencies: *uploadDependenciesPtr,
	}

	fmt.Printf("shutdown period: %s\n", opts.ShutdownPeriod)

	err := run(*envNamePtr, *x86InstanceTypePtr, *armInstanceTypePtr, *destroyPtr, &opts)
	if err != nil {
		log.Fatal(err)
	}
}
