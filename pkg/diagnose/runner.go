// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package diagnose

import (
	"fmt"
	"io"
	"sort"

	"github.com/DataDog/datadog-agent/pkg/diagnose/diagnosis"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/cihub/seelog"
	"github.com/fatih/color"
)

// RunAll runs all registered connectivity checks, output it in writer
func RunAll(w io.Writer) error {
	if w != color.Output {
		color.NoColor = true
	}

	// Use temporarily a custom logger to our Writer
	customLogger, err := seelog.LoggerFromWriterWithMinLevelAndFormat(w, seelog.DebugLvl, "[%LEVEL] %FuncShort: %Msg - %Ns%n")
	if err != nil {
		return err
	}
	log.RegisterAdditionalLogger("diagnose", customLogger)
	defer log.UnregisterAdditionalLogger("diagnose")

	var sortedDiagnosis []string
	for name := range diagnosis.DefaultCatalog {
		sortedDiagnosis = append(sortedDiagnosis, name)
	}
	sort.Strings(sortedDiagnosis)

	for _, name := range sortedDiagnosis {
		fmt.Fprintln(w, fmt.Sprintf("=== Running %s diagnosis ===", color.BlueString(name)))
		err := diagnosis.DefaultCatalog[name]()
		statusString := color.GreenString("PASS")
		if err != nil {
			statusString = color.RedString("FAIL")
			log.Infof("diagnosis error for %s: %v", name, err)
		}
		log.Flush()
		fmt.Fprintln(w, fmt.Sprintf("===> %s\n", statusString))
	}

	return nil
}
