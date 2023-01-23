// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	"flag"
	"fmt"

	"github.com/DataDog/datadog-agent/test/new-e2e/systemProbe"
)

func main() {
	envNamePtr := flag.String("name", "system-probe", "environment name")
	destroyPtr := flag.Bool("destroy", false, "[optional] should destroy the environment")
	securityGroupsPtr := flag.String("sgs", "", "security groups")
	subnetPtr := flag.String("subnet", "", "aws subnet")
	x86InstanceTypePtr := flag.String("instance-type-x86", "", "x86_64 instance type")
	armInstanceTypePtr := flag.String("instance-type-arm", "", "arm64 instance type")

	flag.Parse()

	fmt.Printf("%s %s\n", *securityGroupsPtr, *subnetPtr)
	systemProbeEnv, err := systemProbe.NewTestEnv(*envNamePtr, *securityGroupsPtr, *subnetPtr, *x86InstanceTypePtr, *armInstanceTypePtr)
	if err != nil {
		panic(err)
	}

	if *destroyPtr {
		err = systemProbeEnv.Destroy()
		if err != nil {
			panic(err)
		}
		return
	}

	fmt.Println(systemProbeEnv.Arm64InstanceIP)
	fmt.Println(systemProbeEnv.X86_64InstanceIP)
}
