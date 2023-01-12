// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows && kubeapiserver
// +build !windows,kubeapiserver

package check

import (
	"fmt"
	"os"

	"github.com/DataDog/datadog-agent/cmd/security-agent/app/common"
	"github.com/DataDog/datadog-agent/pkg/compliance/event"
	"github.com/DataDog/datadog-agent/pkg/compliance/utils"
	coreconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/startstop"
)

// RunCheckReporter represents a reporter used for reporting RunChecks
type RunCheckReporter struct {
	reporter        event.Reporter
	events          map[string][]*event.Event
	dumpReportsPath string
}

// NewCheckReporter creates a new RunCheckReporter
func NewCheckReporter(stopper startstop.Stopper, report bool, dumpReportsPath string) (*RunCheckReporter, error) {
	r := &RunCheckReporter{}

	if report {
		endpoints, dstContext, err := common.NewLogContextCompliance()
		if err != nil {
			return nil, err
		}

		runPath := coreconfig.Datadog.GetString("compliance_config.run_path")
		reporter, err := event.NewLogReporter(stopper, "compliance-agent", "compliance", runPath, endpoints, dstContext)
		if err != nil {
			return nil, fmt.Errorf("failed to set up compliance log reporter: %w", err)
		}

		r.reporter = reporter
	}

	r.events = make(map[string][]*event.Event)
	r.dumpReportsPath = dumpReportsPath

	return r, nil
}

// Report reports the event
func (r *RunCheckReporter) Report(event *event.Event) {
	r.events[event.AgentRuleID] = append(r.events[event.AgentRuleID], event)

	eventJSON, err := utils.PrettyPrintJSON(event, "  ")
	if err != nil {
		log.Errorf("Failed to marshal rule event: %v", err)
		return
	}

	r.ReportRaw(eventJSON, "")

	if r.reporter != nil {
		r.reporter.Report(event)
	}
}

// ReportRaw reports the raw content
func (r *RunCheckReporter) ReportRaw(content []byte, service string, tags ...string) {
	fmt.Println(string(content))
}

func (r *RunCheckReporter) dumpReports() error {
	if r.dumpReportsPath != "" {
		reportsJSON, err := utils.PrettyPrintJSON(r.events, "\t")
		if err != nil {
			return err
		}

		return os.WriteFile(r.dumpReportsPath, reportsJSON, 0644)
	}
	return nil
}
