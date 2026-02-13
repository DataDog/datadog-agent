// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package rofspermissions

import (
	"fmt"

	"github.com/DataDog/datadog-agent/comp/core/config"
	healthplatform "github.com/DataDog/datadog-agent/comp/healthplatform/def"
	"github.com/DataDog/datadog-agent/pkg/util/filesystem"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Only check on Linux/Unix for now, as ROFS is primarily a container concept
func Check(cfg config.Component) (*healthplatform.IssueReport, error) {
	writeDirs := []string{
		cfg.GetString("conf_path"),
		cfg.GetString("run_path"),
	}

	for _, dir := range writeDirs {
		if dir == "" {
			continue
		}

		err := filesystem.CheckWritable(dir)
		if err != nil {
			log.Warnf("Write permission check failed for %s: %v", dir, err)

			// Report to Health Platform
			report := &healthplatform.IssueReport{
				IssueID: "read-only-filesystem-error",
				Context: map[string]string{
					"directory": dir,
				},
			}
			// Use the directory as part of the check ID to allow multiple reports
			checkID := fmt.Sprintf("rofs-permissions-%s", dir)
			hp.ReportIssue(checkID, "ROFS Permissions Check", report)
		}
	}
}
