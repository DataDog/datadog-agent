// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	"flag"
	"fmt"

	"github.com/DataDog/datadog-agent/test/new-e2e/ndm/snmp"
)

func main() {
	destroyPtr := flag.Bool("destroy", false, "[optional] should destroy the environment")

	flag.Parse()

	snmpEnv, err := snmp.NewTestEnv()
	if err != nil {
		panic(err)
	}

	if *destroyPtr {
		err = snmpEnv.Destroy()
		if err != nil {
			panic(err)
		}
		return
	}

	fmt.Println(snmpEnv.InstanceIP)
}
