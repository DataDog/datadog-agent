// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package agent holds agent related files
package agent

import (
	"embed"
	"io"

	"github.com/DataDog/datadog-agent/comp/core/status"
)

//go:embed status_templates
var templatesFS embed.FS

type statusProvider struct {
	agent *RuntimeSecurityAgent
}

// StatusProvider returns the runtime security agent status provider
func (rsa *RuntimeSecurityAgent) StatusProvider() status.Provider {
	return statusProvider{
		agent: rsa,
	}
}

// Name returns the name
func (statusProvider) Name() string {
	return "Runtime Security"
}

// Section return the section
func (statusProvider) Section() string {
	return "Runtime Security"
}

func (s statusProvider) populateStatus(stats map[string]interface{}) {
	base := map[string]interface{}{
		"connected":            s.agent.connected.Load(),
		"eventReceived":        s.agent.eventReceived.Load(),
		"activityDumpReceived": s.agent.activityDumpReceived.Load(),
	}

	if s.agent.endpoints != nil {
		base["endpoints"] = s.agent.endpoints.GetStatus()
	}

	if s.agent.client != nil {
		cfStatus, err := s.agent.client.GetStatus()
		if err == nil {
			if cfStatus.Environment != nil {
				environment := map[string]interface{}{
					"warnings":       cfStatus.Environment.Warnings,
					"kernelLockdown": cfStatus.Environment.KernelLockdown,
					"mmapableMaps":   cfStatus.Environment.UseMmapableMaps,
					"ringBuffer":     cfStatus.Environment.UseRingBuffer,
				}
				if cfStatus.Environment.Constants != nil {
					environment["constantFetchers"] = cfStatus.Environment.Constants
				}
				base["environment"] = environment
			}
			if cfStatus.SelfTests != nil {
				selfTests := map[string]interface{}{
					"LastTimestamp": cfStatus.SelfTests.LastTimestamp,
					"Success":       cfStatus.SelfTests.Success,
					"Fails":         cfStatus.SelfTests.Fails,
				}
				base["selfTests"] = selfTests
			}
			base["policiesStatus"] = cfStatus.PoliciesStatus
		}
	}

	stats["runtimeSecurityStatus"] = base
}

func (s statusProvider) getStatus() map[string]interface{} {
	stats := make(map[string]interface{})

	s.populateStatus(stats)

	return stats
}

// JSON populates the status map
func (s statusProvider) JSON(_ bool, stats map[string]interface{}) error {
	s.populateStatus(stats)

	return nil
}

// Text renders the text output
func (s statusProvider) Text(_ bool, buffer io.Writer) error {
	return status.RenderText(templatesFS, "runtimesecurity.tmpl", buffer, s.getStatus())
}

// HTML renders the html output
func (statusProvider) HTML(_ bool, _ io.Writer) error {
	return nil
}
