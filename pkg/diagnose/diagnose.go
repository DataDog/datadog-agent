// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package diagnose

import (
	"fmt"
	"io"

	"github.com/DataDog/datadog-agent/pkg/diagnose/diagnosis"
	"github.com/fatih/color"
)

// Diagnose runs some connectivity checks, output it in writer
// and returns the number of failed diagnosis
func Diagnose(w io.Writer) int {
	errorCounter := 0
	if w != color.Output {
		color.NoColor = true
	}

	for name, d := range diagnosis.DefaultCatalog {
		fmt.Fprintln(w, fmt.Sprintf("=== Running %s diagnosis ===", color.BlueString(name)))
		err := d.Diagnose()
		if err != nil {
			errorCounter++
		}
		statusString := color.GreenString("PASS")
		if err != nil {
			statusString = color.RedString("FAIL")
		}
		fmt.Fprintln(w, fmt.Sprintf("===> %s\n", statusString))
	}

	return errorCounter
}
