// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive
package flare

import (
	"fmt"
	"os"
	"sync"
	"time"

	flaretypes "github.com/DataDog/datadog-agent/comp/core/flare/types"
)

// FlareController is a type that contains information needed to insert into a
// flare from the logs agent.
type FlareController struct {
	mu           sync.Mutex
	allFiles     []string
	journalFiles []string
}

// NewFlareController creates a new FlareController
func NewFlareController() *FlareController {
	return &FlareController{}
}

// FillFlare is the callback function for the flare where information in the
// FlareController can be printed.
func (fc *FlareController) FillFlare(fb flaretypes.FlareBuilder) error {
	fc.mu.Lock()
	defer fc.mu.Unlock()

	// Don't add to the flare if there are no logs files
	if len(fc.allFiles) == 0 && len(fc.journalFiles) == 0 {
		return nil
	}
	fb.AddFileFromFunc("logs_file_permissions.log", func() ([]byte, error) { //nolint:errcheck
		var writer []byte
		var fileInfo string
		// Timer to prevent function from running too long in the event that the
		// agent takes a long time to os.Stat() the files it detects
		timer := time.NewTimer(15 * time.Second)
		combinedFiles := append(fc.allFiles, fc.journalFiles...)

		for _, file := range combinedFiles {
			select {
			case t := <-timer.C:
				fileInfo = fmt.Sprintf("Timed out on %s while getting file permissions\n", t)
				writer = append(writer, []byte(fileInfo)...)
				return writer, nil
			default:
				fi, err := os.Stat(file)
				if err != nil {
					fileInfo = fmt.Sprintf("%s\n", err.Error())
				} else {
					fileInfo = fmt.Sprintf("%s %s\n", file, fi.Mode().String())
				}
				writer = append(writer, []byte(fileInfo)...)
			}
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

// SetAllJournalFiles assigns the journalFiles parameter of FlareController
func (fc *FlareController) AddToJournalFiles(files []string) {
	fc.mu.Lock()
	defer fc.mu.Unlock()

	fc.journalFiles = append(fc.journalFiles, files...)
}
