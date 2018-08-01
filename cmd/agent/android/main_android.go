// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build android

package ddandroid

import (
	"fmt"
	"io/ioutil"
	"log"

	ddapp "github.com/DataDog/datadog-agent/cmd/agent/app"
	"github.com/DataDog/datadog-agent/pkg/status"
	"golang.org/x/mobile/asset"
	yaml "gopkg.in/yaml.v2"
)

type androidEnv struct {
	Cfgpath string `yaml:cfgpath`
}

func (ae *androidEnv) read() *androidEnv {
	yamlFile, err := readAsset("android.yaml")
	if err == nil {
		//		log.Printf("read android config")

		err = yaml.Unmarshal(yamlFile, ae)
		if err == nil {
			return ae
		}
	}
	return ae

}

func readAsset(name string) ([]byte, error) {
	f, errOpen := asset.Open(name)
	//var f *os.File
	//var errOpen error

	if errOpen != nil {
		return nil, errOpen
	}
	defer f.Close()
	buf, errRead := ioutil.ReadAll(f)
	if errRead != nil {
		return nil, errRead
	}
	return buf, nil
}

func AndroidMain() {
	readAsset("android.yaml")
	// read the android-specific config in `assets`, which allows us
	// to override config rather than using environment variables

	var ae androidEnv
	ae.read()
	if len(ae.Cfgpath) != 0 {
		log.Printf("Setting config path to %s", ae.Cfgpath)
		ddapp.SetCfgPath(ae.Cfgpath)
	}
	//ddapp.SetCfgPath("/data/datadog-agent")
	ddapp.StartAgent()
}

func GetStatus() string {
	ret := ""

	s, err := status.GetStatus()
	if err != nil {
		return fmt.Sprintf("Error getting status %v", err)
	}

	//statusJSON, err := json.Marshal(s)
	//if err != nil {
	//		return fmt.Sprintf("Error marshalling status %v", err)
	//	}
	ret = fmt.Sprintf("Agent (v%s)\n", s["version"])
	/*
		ret += "Runner stats\n"
		checkstats := s["runnerStats"]["Checks"]
		// checkstats is map of name->interface of stats
		switch x := checkstats.(type) {
		case map[string]interface{}:
			for name, _ := range x {
				ret += fmt.Printf("Check: %s\n", name)
			}
		}*/
	return ret
}
