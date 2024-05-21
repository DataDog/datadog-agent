// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test && linux

package main

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

const erroMsg = `
The %s_dependencies_%s.txt file is out of date. 
Update the file locally with this content: 
%s
`

func buildDependencyList() (string, error) {
	run := "go"
	arg0 := "list"
	arg1 := "-f"
	arg2 := `"{{ join .Deps "\n"}}"`
	arg3 := "-tags"
	arg4 := "serverless,otlp"
	arg5 := "github.com/DataDog/datadog-agent/cmd/serverless"
	cmd := exec.Command(run, arg0, arg1, arg2, arg3, arg4, arg5)

	stdout, err := cmd.Output()
	return string(stdout), err
}

// This test is here to add friction to the process of adding dependencies to the serverless binary.
// If you are adding a dependency to the serverless binary, you must update the dependencies.txt file.
// Same for when you remove a dependency.
// Having this test also allow us to better track additions and removals of dependencies for the serverless binary.
func TestImportPackage(t *testing.T) {
	dependencyList, err := buildDependencyList()
	assert.NoError(t, err)
	file := fmt.Sprintf("%s_dependencies_%s.txt", runtime.GOOS, runtime.GOARCH)

	data, err := os.ReadFile(file)
	assert.NoError(t, err)

	cleanDependencyList := strings.TrimLeft(dependencyList, "\"")
	cleanDependencyList = strings.TrimRight(cleanDependencyList, "\"\n")
	cleanDependencyList += "\n"
	assert.Equal(t, string(data), cleanDependencyList, fmt.Sprintf(erroMsg, runtime.GOOS, runtime.GOARCH, cleanDependencyList))
}
