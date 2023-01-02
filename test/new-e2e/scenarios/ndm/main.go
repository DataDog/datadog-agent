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
	envNamePtr := flag.String("name", "snmp", "environment name")
	destroyPtr := flag.Bool("destroy", false, "[optional] should destroy the environment")
	keyPairNamePtr := flag.String("keyPairName", "", "ssh key pair name. Should be uploaded on the cloud provider")
	apiKeyPtr := flag.String("apiKey", "", "Datadog API key")
	appKeyPtr := flag.String("appKey", "", "Datadog APP key")

	flag.Parse()

	snmpEnv, err := snmp.NewTestEnv(*envNamePtr, *keyPairNamePtr, *apiKeyPtr, *appKeyPtr)
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
