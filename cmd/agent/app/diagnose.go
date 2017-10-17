// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package app

import (
	"fmt"
	"io"
	"os"

	"github.com/DataDog/datadog-agent/pkg/config"
	log "github.com/cihub/seelog"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

func init() {
	AgentCmd.AddCommand(diagnoseCommand)
}

var diagnoseCommand = &cobra.Command{
	Use:   "diagnose",
	Short: "Execute some connectivity diagnosis on your system",
	Long:  ``,
	RunE:  doDiagnose,
}

func doDiagnose(cmd *cobra.Command, args []string) error {
	if flagNoColor {
		color.NoColor = true
	}

	// The diagnose command reports error directly to the console
	config.SetupLogger("debug", "", "", false, false, "", true)

	Diagnose(os.Stdout)
	return nil
}

// Diagnose runs some connectivity checks
func Diagnose(w io.Writer) {
	fmt.Fprintln(w, "*** Diagnose Begin ***")
	log.Warnf("logs enabled")
	fmt.Fprintf(w, "Colors: %s\n", color.GreenString("enabled"))
}
