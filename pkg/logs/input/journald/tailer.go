// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package journald

import (
	"fmt"

	log "github.com/cihub/seelog"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

// JournalConfig enables to configure the tailer:
// - IncludeUnits: the units to filter in
// - ExcludeUnits: the units to filter out
// - Path: the path of the journal
type JournalConfig struct {
	IncludeUnits []string
	ExcludeUnits []string
	Path         string
}

// TailError represents a fatal error causing the agent to stop tailing a journal
type TailError struct {
	journalID string
	err       error
}

// NewTailError returns a new TailError
func NewTailError(journalID string, err error) TailError {
	return TailError{
		journalID: journalID,
		err:       err,
	}
}

// Error returns the message of the TailError
func (e *TailError) Error() string {
	return fmt.Sprintf("cant't tail journal: %s", e.err)
}

// NewTailer returns a new tailer.
func NewTailer(config JournalConfig, source *config.LogSource, outputChan chan message.Message, errHandler chan TailError) *Tailer {
	return &Tailer{
		config:     config,
		source:     source,
		outputChan: outputChan,
		errHandler: errHandler,
		stop:       make(chan struct{}, 1),
		done:       make(chan struct{}, 1),
	}
}

// Identifier returns the unique identifier of the current journal being tailed.
func (t *Tailer) Identifier() string {
	return "journald:" + t.journalName()
}

// Start starts tailing the journal from a given offset.
func (t *Tailer) Start(cursor string) error {
	if err := t.setup(); err != nil {
		t.source.Status.Error(err)
		return err
	}
	if err := t.seek(cursor); err != nil {
		t.source.Status.Error(err)
		return err
	}
	t.source.Status.Success()
	t.source.AddInput(t.journalName())
	log.Info("Start tailing journal")
	go t.tail()
	return nil
}

// Stop stops the tailer
func (t *Tailer) Stop() {
	log.Info("Stop tailing journal")
	t.stop <- struct{}{}
	t.source.RemoveInput(t.journalName())
	<-t.done
}

// journalName returns the name of the journal
func (t *Tailer) journalName() string {
	if t.config.Path != "" {
		return t.config.Path
	}
	return "default"
}
