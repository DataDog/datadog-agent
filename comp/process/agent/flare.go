// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive
package agent

import (
	"encoding/json"
	"fmt"

	flaretypes "github.com/DataDog/datadog-agent/comp/core/flare/types"
	"github.com/DataDog/datadog-agent/pkg/process/checks"
)

// FlareHelper is a helper struct to fill the flare with check output.
type FlareHelper struct {
	Checks []checks.Check
}

// NewNewFlareHelper creates a new FlareHelper to be provided by the process agent component.
func NewFlareHelper(checks []checks.Check) *FlareHelper {
	return &FlareHelper{Checks: checks}
}

// FillFlare is the callback function for the flare.
func (fh *FlareHelper) FillFlare(fb flaretypes.FlareBuilder) error {
	for _, check := range fh.Checks {
		if check.Realtime() {
			continue
		}

		checkName := check.Name()
		filename := fmt.Sprintf("%s_check_output.json", checkName)
		fb.AddFileFromFunc(filename, func() ([]byte, error) { //nolint:errcheck
			checkOutput, ok := checks.GetCheckOutput(checkName)
			if !ok {
				return []byte(checkName + " check is not running or has not been scheduled yet\n"), nil
			}
			checkJSON, err := json.MarshalIndent(checkOutput, "", "  ")
			if err != nil {
				return []byte(fmt.Sprintf("error: %s", err.Error())), err
			}
			return checkJSON, nil
		})
	}

	return nil
}
