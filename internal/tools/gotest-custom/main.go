// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/google/shlex"
)

func main() {
	flag.Parse()
	packages, gotestArgs, testArgs := parseArguments(flag.Args())

	binaries, pkgNames, err := getBinariesFromPackages(packages)
	if err != nil {
		fmt.Printf("Error getting binaries from packages: %v\n", err)
		os.Exit(1)
	}

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

			// Build the base command
			args := []string{"tool", "test2json", "-p", pkgNames[idx], "-t", "./" + binary, "-test.v=test2json"}

			parsedGoTestArgs, err := shlex.Split(strings.Join(gotestArgs, " "))
			if err != nil {
				fmt.Printf("Error parsing go test args: %v\n", err)
				errChannel <- err
				return
			}
			args = append(args, parsedGoTestArgs...)

			parsedTestArgs, err := shlex.Split(strings.Join(testArgs, " "))
			if err != nil {
				fmt.Printf("Error parsing test args: %v\n", err)
				errChannel <- err
				return
			}
			args = append(args, parsedTestArgs...)

			command := exec.Command("go", args...)
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

// parseArguments separates command line arguments into three categories:
// 1. Packages (everything before first flag starting with -)
// 2. Gotest args (flags that start with -test.)
// 3. Test args (everything after -args)
func parseArguments(args []string) (packages []string, gotestArgs []string, testArgs []string) {
	state := "packages"    // packages -> gotest -> testargs
	nextIsTestArg := false // Used to handle case where -test.run is followed by its value, should not beed set if the -test.count=1 format is used
	for _, arg := range args {
		switch state {
		case "packages":
			if arg == "-args" {
				state = "testargs"
			} else if strings.HasPrefix(arg, "-") {
				state = "gotest"
				// Only add to gotestArgs if it starts with -test.
				if strings.HasPrefix(arg, "-test.") {
					gotestArgs = append(gotestArgs, arg)
					if !strings.Contains(arg, "=") {
						nextIsTestArg = true
					}
				}
			} else {
				packages = append(packages, arg)
			}
		case "gotest":
			if arg == "-args" {
				state = "testargs"
			} else if strings.HasPrefix(arg, "-test.") {
				gotestArgs = append(gotestArgs, arg)
				if !strings.Contains(arg, "=") {
					nextIsTestArg = true
				}
			} else if nextIsTestArg {
				if !strings.HasPrefix(arg, "-") {
					gotestArgs = append(gotestArgs, arg)
				}
				nextIsTestArg = false
			}
		case "testargs":
			testArgs = append(testArgs, arg)
		}
	}

	return packages, gotestArgs, testArgs
}

// BinaryInfo is the information about a binary that is built for a package
type BinaryInfo struct {
	Binary  string `json:"binary"`
	Package string `json:"package"`
}

// Manifest is the manifest of the binaries that are built for the packages
type Manifest struct {
	Binaries []BinaryInfo `json:"binaries"`
}

func getBinariesFromPackages(packages []string) ([]string, []string, error) {
	binariesPath := "test-binaries"
	manifestPath := filepath.Join(binariesPath, "manifest.json")

	// Check if manifest exists
	if _, err := os.Stat(manifestPath); os.IsNotExist(err) {
		return nil, nil, fmt.Errorf("manifest path %s does not exist", manifestPath)
	}

	// Read manifest file
	manifestData, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read manifest: %v", err)
	}

	// Parse manifest
	var manifest Manifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		return nil, nil, fmt.Errorf("failed to parse manifest: %v", err)
	}

	var binaries []string
	var matchedPackages []string

	// For each target package, find matching binaries
	for _, target := range packages {
		targetPackages, err := exec.Command("go", "list", target).Output()
		if err != nil {
			return nil, nil, fmt.Errorf("failed to get target packages: %v", err)
		}
		for _, pkg := range strings.Split(string(targetPackages), "\n") {
			pkg = strings.TrimPrefix(pkg, "github.com/DataDog/datadog-agent/test/new-e2e/")
			for _, binaryInfo := range manifest.Binaries {
				if binaryInfo.Package == pkg {
					binaryPath := filepath.Join(binariesPath, binaryInfo.Binary)
					binaries = append(binaries, binaryPath)
					matchedPackages = append(matchedPackages, binaryInfo.Package)
				}
			}
		}
	}
	return binaries, matchedPackages, nil
}
