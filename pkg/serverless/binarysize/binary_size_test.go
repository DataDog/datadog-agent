// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package binarysize

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func packageBlockList() []string {
	return []string{
		"github.com/h2non/filetype",
		// more to come
	}
}

func buildImportList() []string {
	run := "go"
	arg0 := "list"
	arg1 := "-json"
	arg2 := "-tags"
	arg3 := "serverless"
	arg4 := "github.com/DataDog/datadog-agent/cmd/serverless"
	cmd := exec.Command(run, arg0, arg1, arg2, arg3, arg4)
	stdout, err := cmd.Output()
	if err != nil {
		panic("could not build the import list")
	}
	return strings.Split(string(stdout), "\n")
}

func isPackageIncluded(packageName string, packageList []string) bool {
	for _, p := range packageList {
		if strings.Contains(p, packageName) {
			return true
		}
	}
	return false
}

func TestImportPackage(t *testing.T) {
	packageList := buildImportList()
	packageBlockList := packageBlockList()
	for _, blockedPackage := range packageBlockList {
		assert.False(t, isPackageIncluded(blockedPackage, packageList), "package %s is included in the serverless build", blockedPackage)
	}
}
