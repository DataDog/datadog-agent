// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build systemd

// Package journald provides journald-based log launchers (no-op for non-systemd builds)
package journald

import (
	"os"

	"github.com/coreos/go-systemd/v22/sdjournal"

	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	storedef "github.com/DataDog/datadog-agent/comp/healthplatform/store/def"
	"github.com/DataDog/datadog-agent/comp/logs-library/pipeline"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	flareController "github.com/DataDog/datadog-agent/comp/logs/agent/flare"
	auditor "github.com/DataDog/datadog-agent/comp/logs/auditor/def"
	"github.com/DataDog/datadog-agent/pkg/logs/launchers"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	"github.com/DataDog/datadog-agent/pkg/logs/tailers"
	tailer "github.com/DataDog/datadog-agent/pkg/logs/tailers/journald"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/option"
	"github.com/DataDog/datadog-agent/pkg/util/startstop"
)

// SDJournalFactory is a JournalFactory implementation that produces sdjournal instances
type SDJournalFactory struct{}

// NewJournal creates a new sdjournal instance
func (s *SDJournalFactory) NewJournal() (tailer.Journal, error) {
	return sdjournal.NewJournal()
}

// NewJournalFromPath creates a new sdjournal instance from the supplied path
func (s *SDJournalFactory) NewJournalFromPath(path string) (tailer.Journal, error) {
	return sdjournal.NewJournalFromDir(path)
}

// Launcher is in charge of starting and stopping new journald tailers
type Launcher struct {
	sources          chan *sources.LogSource
	pipelineProvider pipeline.Provider
	registry         auditor.Registry
	tailers          map[string]*tailer.Tailer
	stop             chan struct{}
	journalFactory   tailer.JournalFactory
	fc               *flareController.FlareController
	tagger           tagger.Component
	healthPlatform   option.Option[storedef.Component]
}

// NewLauncher returns a new Launcher.
func NewLauncher(fc *flareController.FlareController, tagger tagger.Component, hp option.Option[storedef.Component]) *Launcher {
	return NewLauncherWithFactory(&SDJournalFactory{}, fc, tagger, hp)
}

// NewLauncherWithFactory returns a new Launcher.
func NewLauncherWithFactory(journalFactory tailer.JournalFactory, fc *flareController.FlareController, tagger tagger.Component, hp option.Option[storedef.Component]) *Launcher {
	return &Launcher{
		tailers:        make(map[string]*tailer.Tailer),
		stop:           make(chan struct{}),
		journalFactory: journalFactory,
		fc:             fc,
		tagger:         tagger,
		healthPlatform: hp,
	}
}

// Start starts the launcher.
func (l *Launcher) Start(sourceProvider launchers.SourceProvider, pipelineProvider pipeline.Provider, registry auditor.Registry, _ *tailers.TailerTracker) {
	l.sources = sourceProvider.GetAddedForType(config.JournaldType)
	l.pipelineProvider = pipelineProvider
	l.registry = registry
	go l.run()
}

// run starts new tailers.
func (l *Launcher) run() {
	var allJournalSources []string

	for {
		select {
		case source := <-l.sources:
			identifier := tailer.Identifier(source.Config)
			if _, exists := l.tailers[identifier]; exists {
				log.Warn(identifier, " is already tailed. Use config_id to tail the same journal more than once")
				continue
			}

			if source.Config.Path != "" {
				// Add path to flare if specified in configuration
				allJournalSources = append(allJournalSources, source.Config.Path)
			} else {
				// Check default locations otherwise
				if _, err := os.Stat("/var/log/journal"); err == nil {
					allJournalSources = append(allJournalSources, "/var/log/journal")
				} else if _, err := os.Stat("/run/log/journal"); err == nil {
					allJournalSources = append(allJournalSources, "/run/log/journal")
				}
			}

			// Detect multi_line rules on journald sources — silently ignored since
			// journald entries are pre-framed; report to health platform if configured.
			for _, rule := range source.Config.ProcessingRules {
				if rule.Type == config.MultiLine {
					log.Warnf("multi_line aggregation is not supported for journald log sources (source: %s) — rules will be ignored", identifier)
					l.reportMultiLineIssue(identifier)
					break
				}
			}

			tailer, err := l.setupTailer(source)
			if err != nil {
				log.Warn("Could not set up journald tailer: ", err)
			} else {
				l.tailers[identifier] = tailer
			}

			l.fc.AddToJournalFiles(allJournalSources)
		case <-l.stop:
			return
		}
	}
}

// Stop stops all active tailers
func (l *Launcher) Stop() {
	l.stop <- struct{}{}
	stopper := startstop.NewParallelStopper()
	for identifier, tailer := range l.tailers {
		stopper.Add(tailer)
		delete(l.tailers, identifier)
	}
	stopper.Stop()
}

// setupTailer configures and starts a new tailer,
// returns the tailer or an error.
func (l *Launcher) setupTailer(source *sources.LogSource) (*tailer.Tailer, error) {
	var journal tailer.Journal
	var err error

	if source.Config.Path == "" {
		// open the default journal
		journal, err = l.journalFactory.NewJournal()
	} else {
		journal, err = l.journalFactory.NewJournalFromPath(source.Config.Path)
	}
	if err != nil {
		return nil, err
	}

	tailer := tailer.NewTailer(source, l.pipelineProvider.NextPipelineChan(), journal, source.Config.ShouldProcessRawMessage(), l.tagger, l.registry)
	cursor := l.registry.GetOffset(tailer.Identifier())

	err = tailer.Start(cursor)
	if err != nil {
		return nil, err
	}
	return tailer, nil
}

// reportMultiLineIssue reports a multi_line-on-journald misconfiguration to the health platform.
func (l *Launcher) reportMultiLineIssue(sourceName string) {
	store, ok := l.healthPlatform.Get()
	if !ok {
		return
	}
	if err := store.ReportIssue(storedef.IssueReport{
		IssueID:   multiLineJournaldIssueID,
		IssueType: multiLineJournaldIssueID,
		Source:    "logs",
		Context:   map[string]string{"source": sourceName, "sourceType": "journald"},
		Tags:      []string{"logs", "journald", "multiline"},
	}); err != nil {
		log.Warnf("Failed to report multiline-journald issue to health platform: %v", err)
	}
}

// multiLineJournaldIssueID mirrors the constant from the logs-multiline-journald
// issue package, inlined here to avoid an import cycle.
const multiLineJournaldIssueID = "logs-multiline-journald-unsupported"
