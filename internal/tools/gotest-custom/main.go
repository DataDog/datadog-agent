// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.
package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
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
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Printf("Error getting current working directory: %v\n", err)
		os.Exit(1)
	}

	for idx, binary := range binaries {
		go func(idx int, binary string) {
			defer wg.Done()
			binaryPath := path.Join(cwd, binary)

			// Build the base command
			args := []string{"tool", "test2json", "-p", fmt.Sprintf("github.com/DataDog/datadog-agent/test/new-e2e/%s", pkgNames[idx]), "-t", binaryPath, "-test.v=test2json"}

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
			command.Dir = filepath.Join(cwd, "test/new-e2e", pkgNames[idx])
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
	binariesPath := "test-binaries.tar.gz"
	manifestPath := "manifest.json"
	extractPath := "test-binaries"

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

	// Create extraction directory if it doesn't exist
	if err := os.MkdirAll(extractPath, 0755); err != nil {
		return nil, nil, fmt.Errorf("failed to create extraction directory: %v", err)
	}

	var binaries []string
	var matchedPackages []string
	var targetBinaries = make(map[string]bool)

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
					binaryPath := filepath.Join(extractPath, binaryInfo.Binary)
					binaries = append(binaries, binaryPath)
					matchedPackages = append(matchedPackages, binaryInfo.Package)
					targetBinaries[binaryInfo.Binary] = true
				}
			}
		}
	}

	// Open and extract tar.gz file
	file, err := os.Open(binariesPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to open archive: %v", err)
	}
	defer file.Close()

	gzr, err := gzip.NewReader(file)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create gzip reader: %v", err)
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)

	// Extract only the targeted binaries
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, nil, fmt.Errorf("failed to read tar header: %v", err)
		}

		// Skip if not a targeted binary
		baseName := filepath.Base(header.Name)
		if !targetBinaries[baseName] {
			continue
		}

		// Create the file
		outPath := filepath.Join(extractPath, baseName)
		outFile, err := os.OpenFile(outPath, os.O_CREATE|os.O_RDWR, os.FileMode(header.Mode))
		if err != nil {
			return nil, nil, fmt.Errorf("failed to create output file %s: %v", outPath, err)
		}

		if _, err := io.Copy(outFile, tr); err != nil {
			outFile.Close()
			return nil, nil, fmt.Errorf("failed to write output file %s: %v", outPath, err)
		}
		outFile.Close()
	}

	return binaries, matchedPackages, nil
}
