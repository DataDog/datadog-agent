// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018-present Datadog, Inc.

//go:build python

package python

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
)

var (
	linterTimeout = time.Duration(config.Datadog.GetInt("python3_linter_timeout")) * time.Second
)

type warning struct {
	Line      int    `json:"line"`
	Column    int    `json:"column"`
	Message   string `json:"message"`
	Path      string `json:"path"`
	Symbol    string `json:"symbol"`
	MessageID string `json:"message-id"`
}

// validatePython3 checks that a check can run on python 3.
func validatePython3(moduleName string, modulePath string) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), linterTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, pythonBinPath, "-m", "pylint", "-f", "json", "--py3k", "-d", "W1618", "--persistent", "no", "--exit-zero", modulePath)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("error running the linter on (%s): %s", err, stderr.String())
	}

	res := []string{}
	if stdout.Len() == 0 {
		// No warning
		return res, nil
	}

	var warnings []warning
	if err := json.Unmarshal(stdout.Bytes(), &warnings); err != nil {
		return nil, fmt.Errorf("could not Unmarshal warnings from Python3 linter: %s", err)
	}

	// no post processing needed for now, we just retrieve every messages
	for _, warn := range warnings {
		message := fmt.Sprintf("%s:%d:%d : %s (%s, %s)", filepath.Base(warn.Path), warn.Line, warn.Column, warn.Message, warn.Symbol, warn.MessageID)
		res = append(res, message)
	}

	return res, nil
}
