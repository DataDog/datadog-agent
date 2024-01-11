// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flare

import (
	"os"
	"sync"

	flaretypes "github.com/DataDog/datadog-agent/comp/core/flare/types"
)

type FlareController struct {
	mu       sync.Mutex
	allFiles []string
}

// NewFlareController ...
func NewFlareController() *FlareController {
	return &FlareController{}
}

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

// SetAllFiles ...
func (fc *FlareController) SetAllFiles(files []string) {
	fc.mu.Lock()
	defer fc.mu.Unlock()

	fc.allFiles = files
}
