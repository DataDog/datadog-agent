// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package statusimpl adds a Process Manager section to agent status.
//
// It reuses coat.Collector.Report, the same SupportReport path the flare provider
// writes to procmgr/state.json, so operators can see dd-procmgrd supervision
// without collecting a flare.
package statusimpl

import (
	"context"
	"embed"
	"io"
	"time"

	config "github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/status"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	"github.com/DataDog/datadog-agent/pkg/procmgr/coat"
)

//go:embed status_templates
var templatesFS embed.FS

// These are the process-agent settings for process-argument privacy. An operator who set either
// one means it for status too, matching the flare provider.
const (
	customSensitiveWordsSetting = "process_config.custom_sensitive_words"
	stripProcArgumentsSetting   = "process_config.strip_proc_arguments"
)

// statusCollectionBudget is how long status may spend asking dd-procmgrd about itself.
// It is deliberately shorter than the flare's 8s budget: status must stay snappy, and a
// partial report with errors is still useful content.
const statusCollectionBudget = 2 * time.Second

// Requires specifies the dependencies of the constructor.
type Requires struct {
	compdef.In

	Config config.Component
}

// Provides specifies the types returned by the constructor.
type Provides struct {
	compdef.Out

	Status status.InformationProvider
}

// reporter is the subset of coat.Collector this component needs, so tests can substitute a fake
// rather than requiring a live dd-procmgrd.
type reporter interface {
	Report(ctx context.Context, opts coat.ScrubOptions) coat.SupportReport
}

type statusProvider struct {
	reporter reporter
	scrub    coat.ScrubOptions
}

// NewComponent returns a status provider that shows dd-procmgrd supervision state.
func NewComponent(reqs Requires) Provides {
	return newProvides(coat.NewCollector(), scrubOptionsFromConfig(reqs.Config))
}

func scrubOptionsFromConfig(cfg config.Component) coat.ScrubOptions {
	return coat.ScrubOptions{
		CustomSensitiveWords: cfg.GetStringSlice(customSensitiveWordsSetting),
		StripArguments:       cfg.GetBool(stripProcArgumentsSetting),
	}
}

func newProvides(r reporter, scrub coat.ScrubOptions) Provides {
	return Provides{
		Status: status.NewInformationProvider(statusProvider{
			reporter: r,
			scrub:    scrub,
		}),
	}
}

// Name returns the name
func (s statusProvider) Name() string {
	return "Process Manager"
}

// Section returns the section
func (s statusProvider) Section() string {
	return "Process Manager"
}

// JSON populates the status map
func (s statusProvider) JSON(_ bool, stats map[string]interface{}) error {
	s.populateStatus(stats)
	return nil
}

// Text renders the text output
func (s statusProvider) Text(_ bool, buffer io.Writer) error {
	return status.RenderText(templatesFS, "process_manager.tmpl", buffer, s.getStatusInfo())
}

// HTML renders the html output
func (s statusProvider) HTML(_ bool, buffer io.Writer) error {
	return status.RenderHTML(templatesFS, "process_managerHTML.tmpl", buffer, s.getStatusInfo())
}

func (s statusProvider) getStatusInfo() map[string]interface{} {
	stats := make(map[string]interface{})
	s.populateStatus(stats)
	return stats
}

func (s statusProvider) populateStatus(stats map[string]interface{}) {
	ctx, cancel := context.WithTimeout(context.Background(), statusCollectionBudget)
	defer cancel()

	report := s.reporter.Report(ctx, s.scrub)
	// Report has already scrubbed with the same settings; repeating it costs nothing and covers
	// a reporter that returns a report it assembled rather than collected.
	report.Scrub(s.scrub)
	stats["processManager"] = report
}
