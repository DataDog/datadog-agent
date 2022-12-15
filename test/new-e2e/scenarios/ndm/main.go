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

	snmpEnv, err := snmp.NewTestEnv(*envNamePtr, *keyPairNamePtr, *apiKeyPtr, *appKeyPtr, *destroyPtr)
	if err != nil {
		panic(err)
	}

	if !*destroyPtr {
		fmt.Println(snmpEnv.InstanceIP)
	}
}
