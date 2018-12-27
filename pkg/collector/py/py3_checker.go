// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build cpython

package py

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/util/executable"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func init() {
	here, _ := executable.Folder()
	py3LinterPath = filepath.Join(here, py3LinterPath)
	if _, err := os.Stat(py3LinterPath); err == nil {
		log.Debugf("python3 linter found (%s): enabling linter", py3LinterPath)
	} else {
		log.Warnf("could not find python3 linter '%s': disabling linter", py3LinterPath)
		py3LinterPath = ""
	}
}

type warning struct {
	Message string
}

// verifyPython3 checks that a check can run on python 3
func validatePython3(moduleName string, modulePath string) ([]string, error) {
	if py3LinterPath == "" {
		return nil, fmt.Errorf("no Python3 linter found")
	}

	cmd := exec.Command(py3LinterPath, modulePath)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		return nil, fmt.Errorf("error running the linter on (%s): %s", err, stderr.String())
	}

	var warnings []warning
	if err = json.Unmarshal(stdout.Bytes(), &warnings); err != nil {
		return nil, fmt.Errorf("could not Unmarshal warnings from Python3 linter: %s", err)
	}

	res := []string{}
	// no post processing needed for now, we just retrieve every messages
	for _, warn := range warnings {
		log.Warnf("check '%s' is not Python3 compatible: %s", moduleName, warn.Message)
		res = append(res, warn.Message)
	}

	return res, nil
}
