// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive
package agent

import (
	"encoding/json"
	"fmt"
	"sync"

	flaretypes "github.com/DataDog/datadog-agent/comp/core/flare/types"
	"github.com/DataDog/datadog-agent/pkg/process/checks"
	"github.com/fatih/color"
)

// FlareHelper
type FlareHelper struct {
	m      sync.Mutex
	Checks []checks.Check
}

// NewNewFlareHelper
func NewFlareHelper(checks []checks.Check) *FlareHelper {
	return &FlareHelper{Checks: checks}
}

// FillFlare is the callback function for the flare.
func (fh *FlareHelper) FillFlare(fb flaretypes.FlareBuilder) error {
	fh.m.Lock()
	defer fh.m.Unlock()

	for _, check := range fh.Checks {
		checkName := check.Name()
		filename := fmt.Sprintf("%s_check_output.json", checkName)
		fb.AddFileFromFunc(filename, func() ([]byte, error) {
			var out []byte
			checkOutput, ok := checks.GetCheckOutput(checkName)
			if !ok {
				out = append(out, []byte(checkName+" check is not running or has not been scheduled yet\n")...)
				return out, nil
			}
			checkJSON, err := json.MarshalIndent(checkOutput, "", "  ")
			if err != nil {
				out = append(out, []byte(fmt.Sprintf("error: %v", err.Error()))...)
				return out, err
			}
			return checkJSON, nil
		})
	}

	return nil
}
