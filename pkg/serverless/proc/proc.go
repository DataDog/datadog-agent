// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO(SERV) Fix revive linter
package proc

import (
	"bytes"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func getPidList(procPath string) []int {
	files, err := os.ReadDir(procPath)
	pids := []int{}
	if err != nil {
		log.Debug("could not list /proc files")
		return pids
	}
	for _, file := range files {
		if file.IsDir() {
			if processID, err := strconv.Atoi(file.Name()); err == nil {
				pids = append(pids, processID)
			}
		}
	}
	return pids
}

func getEnvVariablesFromPid(procPath string, pid int) map[string]string {
	envVars := map[string]string{}

	bytesRead, err := os.ReadFile(fmt.Sprintf("%s/%d/environ", procPath, pid))
	if err != nil {
		log.Debug("could not list environment variable for proc id %d", pid)
		return envVars
	}

	nullByte := "\000"
	items := bytes.Split(bytesRead, []byte(nullByte))
	for _, item := range items {
		if len(item) > 0 {
			parts := strings.Split(string(item), "=")
			if len(parts) == 2 {
				envVars[parts[0]] = parts[1]
			}
		}
	}

	return envVars
}

// SearchProcsForEnvVariable returns values of the given env variable name
// it returns a slice since a value could be found in more than one process
func SearchProcsForEnvVariable(procPath string, envName string) []string {
	result := []string{}
	pidList := getPidList(procPath)
	for _, pid := range pidList {
		envMap := getEnvVariablesFromPid(procPath, pid)
		if value, ok := envMap[envName]; ok {
			result = append(result, value)
		}
	}
	return result
}
