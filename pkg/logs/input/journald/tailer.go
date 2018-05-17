// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package journald

import (
	log "github.com/cihub/seelog"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

// NewTailer returns a new tailer.
func NewTailer(source *config.LogSource, outputChan chan message.Message) *Tailer {
	return &Tailer{
		source:     source,
		outputChan: outputChan,
		stop:       make(chan struct{}, 1),
		done:       make(chan struct{}, 1),
	}
}

// journaldIntegration represents the name of the integration,
// it's used to override the source of the message and as a fingerprint to store the journal cursor.
const journaldIntegration = "journald"

// Identifier returns the unique identifier of the current journal being tailed.
func (t *Tailer) Identifier() string {
	return journaldIntegration + ":" + t.journalPath()
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
	t.source.AddInput(t.journalPath())
	log.Info("Start tailing journal ", t.journalPath())
	go t.tail()
	return nil
}

// Stop stops the tailer
func (t *Tailer) Stop() {
	log.Info("Stop tailing journal ", t.journalPath())
	t.stop <- struct{}{}
	t.source.RemoveInput(t.journalPath())
	<-t.done
}

// journalPath returns the path of the journal
func (t *Tailer) journalPath() string {
	if t.source.Config.Path != "" {
		return t.source.Config.Path
	}
	return "default"
}
