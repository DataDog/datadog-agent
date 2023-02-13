// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	"flag"
	"fmt"

	systemProbe "github.com/DataDog/datadog-agent/test/new-e2e/system-probe"
)

func main() {
	envNamePtr := flag.String("name", "system-probe", "environment name")
	destroyPtr := flag.Bool("destroy", false, "[optional] should destroy the environment")
	securityGroupsPtr := flag.String("sgs", "", "security groups")
	subnetsPtr := flag.String("subnets", "", "aws subnets")
	x86InstanceTypePtr := flag.String("instance-type-x86", "", "x86_64 instance type")
	armInstanceTypePtr := flag.String("instance-type-arm", "", "arm64 instance type")

	flag.Parse()

	systemProbeEnv, err := systemProbe.NewTestEnv(*envNamePtr, *securityGroupsPtr, *subnetsPtr, *x86InstanceTypePtr, *armInstanceTypePtr)
	if err != nil {
		fmt.Printf("%+v\n", err)
		panic(err)
	}

	if *destroyPtr {
		err = systemProbeEnv.Destroy()
		if err != nil {
			panic(err)
		}
		return
	}

	fmt.Println(systemProbeEnv.ARM64InstanceIP)
	fmt.Println(systemProbeEnv.X86_64InstanceIP)
}
