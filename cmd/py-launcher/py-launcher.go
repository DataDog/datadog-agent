// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build python

package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/collector/python"
	"github.com/DataDog/datadog-agent/pkg/config"
)

/*
#include "datadog_agent_rtloader.h"
#cgo !windows LDFLAGS: -L../../rtloader/ -ldatadog-agent-rtloader -ldl
#cgo windows LDFLAGS: -L../../rtloader/ -ldatadog-agent-rtloader -lstdc++ -static
*/
import "C"

func main() {
	var conf = flag.String("conf", "", "option path to datadog.yaml")
	var pythonScript = flag.String("py", "", "python script to run")
	var pythonPath = flag.String("path", "", "comma separated list of path to add to PYTHONPATH")
	flag.Parse()

	flag.Usage = func() {
		// Disable: printf: `fmt.Println` arg list ends with redundant newline (govet)
		fmt.Println("This binary execute a python script in the context of the Datadog Agent.\n" + //nolint:govet
			"This includes synthetic modules (Go module bind to Python), logging facilities, configuration setup, ...\n")

		fmt.Printf("Usage: %s [-conf datadog.yaml] -py PYTHON_FILE -- [ARGS FOR THE PYTHON SCRIPT]...\n", os.Args[0])
		flag.PrintDefaults()
	}

	if *pythonScript == "" {
		flag.Usage()
		os.Exit(1)
	}

	if *conf != "" {
		config.Datadog.SetConfigFile(*conf)
		_, confErr := config.Load()
		if confErr != nil {
			fmt.Printf("unable to parse Datadog config file, running with env variables: %s\n", confErr)
		}
	}

	paths := strings.Split(*pythonPath, ",")
	python.Initialize(paths...) //nolint:errcheck

	pyRtLoader := python.GetRtLoader()
	rtloader := (*C.rtloader_t)(pyRtLoader)
	pythonCode, err := os.ReadFile(*pythonScript)
	if err != nil {
		fmt.Printf("Could not read %s: %s\n", *pythonScript, err)
		os.Exit(1)
	}
	state := C.ensure_gil(rtloader)
	res := C.run_simple_string(rtloader, C.CString(string(pythonCode)))
	C.release_gil(rtloader, state)
	if res == 0 {
		fmt.Printf("Error while running python script: %s\n", C.GoString(C.get_error(rtloader)))
	}
}
