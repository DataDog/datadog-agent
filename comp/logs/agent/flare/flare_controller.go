// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive
package flare

import (
	"os"
	"sync"

	flaretypes "github.com/DataDog/datadog-agent/comp/core/flare/types"
)

// FlareController is an type that contains information needed needed to insert
// into a flare from the logs agent.
type FlareController struct {
	mu       sync.Mutex
	allFiles []string
}

// NewFlareController creates a new FlareController
func NewFlareController() *FlareController {
	return &FlareController{}
}

// FillFlare is the callback function for the flare where information in the
// FlareController can be printed.
func (fc *FlareController) FillFlare(fb flaretypes.FlareBuilder) error {
	// Don't add to the flare if there are no logs files
	if len(fc.allFiles) == 0 {
		return nil
	}
	fb.AddFileFromFunc("logs_file_permissions.log", func() ([]byte, error) {
		var writer []byte

		for _, file := range fc.allFiles {
			fi, err := os.Stat(file)
			if err != nil {
				return nil, err
			}
			writer = append(writer, []byte(file+" "+fi.Mode().String()+"\n")...)
		}

		return writer, nil
	})

	return nil
}

// SetAllFiles assigns the allFiles parameter of FlareController
func (fc *FlareController) SetAllFiles(files []string) {
	fc.mu.Lock()
	defer fc.mu.Unlock()

	fc.allFiles = files
}
