// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

// Package independent-lint checks a go.mod file at a given path specified by the -path argument
// to ensure that it does not import the list of modules specified by the -deny argument. If
// the module is found, it exits with status code 1 and logs an error.

package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
)

var (
	binariesFlag string
	pkgNamesFlag string
	extraFlag    string
)

func main() {
	flag.StringVar(&binariesFlag, "binaries", "", "Comma separated list of binaries to test")
	flag.StringVar(&pkgNamesFlag, "pkgnames", "", "Comma separated list of package names")
	flag.StringVar(&extraFlag, "extra", "", "Extra flags to pass to the test")

	flag.Parse()
	binaries := strings.Split(binariesFlag, ",")
	pkgNames := strings.Split(pkgNamesFlag, ",")

	if len(binaries) != len(pkgNames) {
		fmt.Println("Number of binaries and package names should be the same")
		os.Exit(1)
	}

	wg := sync.WaitGroup{}
	wg.Add(len(binaries))
	outputs := make([]*bytes.Buffer, len(binaries))
	for idx := range outputs {
		outputs[idx] = &bytes.Buffer{}
	}
	errChannel := make(chan error, len(binaries))
	// printLock := sync.Mutex{}
	for idx, binary := range binaries {
		go func(idx int, binary string) {
			defer wg.Done()

			cmd := fmt.Sprintf("tool test2json -p %s -t ./%s -test.v=test2json %s", pkgNames[idx], binary, extraFlag)
			command := exec.Command("go", strings.Split(cmd, " ")...)
			command.Stdout = os.Stdout
			command.Stderr = os.Stdout
			errCmd := command.Run()
			errChannel <- errCmd
		}(idx, binary)
	}
	wg.Wait()
	close(errChannel)
	for err := range errChannel {
		if err != nil {
			os.Exit(1)
		}
	}
}
