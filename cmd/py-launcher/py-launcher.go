// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/collector/python"
	"github.com/DataDog/datadog-agent/pkg/config"
)

/*
#include "datadog_agent_six.h"
#cgo !windows LDFLAGS: -L../../six/ -ldatadog-agent-six -ldl
#cgo windows LDFLAGS: -L../../six/ -ldatadog-agent-six -lstdc++ -static
*/
import "C"

func main() {
	var conf = flag.String("conf", "", "option path to datadog.yaml")
	var pythonScript = flag.String("py", "", "python script to run")
	var pythonPath = flag.String("path", "", "comma separated list of path to add to PYTHONPATH")
	flag.Parse()

	flag.Usage = func() {
		fmt.Println("This binary execute a python script in the context of the Datadog Agent.\n" +
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
		confErr := config.Load()
		if confErr != nil {
			fmt.Printf("unable to parse Datadog config file, running with env variables: %s", confErr)
		}
	}

	paths := strings.Split(*pythonPath, ",")
	python.Initialize(paths...)

	pySix := python.GetSix()
	six := (*C.six_t)(pySix)
	pythonCode, err := ioutil.ReadFile(*pythonScript)
	if err != nil {
		fmt.Printf("Could not read %s: %s\n", *pythonScript, err)
		os.Exit(1)
	}
	state := C.ensure_gil(six)
	res := C.run_simple_string(six, C.CString(string(pythonCode)))
	C.release_gil(six, state)
	if res == 0 {
		fmt.Printf("Error while running python script: %s\n", C.GoString(C.get_error(six)))
	}

	python.Destroy()
}
