// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !systemd
// +build !systemd

package journald

import (
	"github.com/DataDog/datadog-agent/pkg/logs/auditor"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/launchers"
	tailer "github.com/DataDog/datadog-agent/pkg/logs/internal/tailers/journald"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
)

// SDJournalFactory a JounralFactory implementation that produces sdjournal instances
type SDJournalFactory struct{}

func (s *SDJournalFactory) NewJournal() (tailer.Journal, error) {
	return nil, nil
}

func (s *SDJournalFactory) NewJournalFromPath(path string) (tailer.Journal, error) {
	return nil, nil
}

// Launcher is not supported on no systemd environment.
type Launcher struct{}

// NewLauncher returns a new Launcher
func NewLauncher(journalFactory *SDJournalFactory) *Launcher {
	return &Launcher{}
}

// Start does nothing
func (l *Launcher) Start(sources launchers.SourceProvider, pipelineProvider pipeline.Provider, registry auditor.Registry) {
}

// Stop does nothing
func (l *Launcher) Stop() {}
