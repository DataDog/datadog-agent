// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package diagnose

import (
	"fmt"
	"io"

	"github.com/DataDog/datadog-agent/pkg/diagnose/diagnosis"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/cihub/seelog"
	"github.com/fatih/color"
)

// RunAll runs all registered connectivity checks, output it in writer
// and returns the number of failed diagnosis
func RunAll(w io.Writer) (int, error) {
	errorCounter := 0
	if w != color.Output {
		color.NoColor = true
	}

	// Use temporarily a custom logger to our Writer
	customLogger, err := seelog.LoggerFromWriterWithMinLevelAndFormat(w, seelog.DebugLvl, "[%LEVEL] %FuncShort: %Msg - %Ns%n")
	if err != nil {
		return -1, nil
	}
	log.RegisterAdditionalLogger("diagnose", customLogger)
	defer log.UnregisterAdditionalLogger("diagnose")

	for name, f := range diagnosis.DefaultCatalog {
		fmt.Fprintln(w, fmt.Sprintf("=== Running %s diagnosis ===", color.BlueString(name)))
		err := f()
		if err != nil {
			errorCounter++
		}
		statusString := color.GreenString("PASS")
		if err != nil {
			statusString = color.RedString("FAIL")
		}
		fmt.Fprintln(w, fmt.Sprintf("===> %s\n", statusString))
	}

	return errorCounter, nil
}
