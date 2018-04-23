// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build !systemd

package journald

import (
	log "github.com/cihub/seelog"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

// Tailer collects logs from a journal.
type Tailer struct {
	config     JournalConfig
	source     *config.LogSource
	outputChan chan message.Message
	stop       chan struct{}
	done       chan struct{}
}

// setup does nothing
func (t *Tailer) setup() error {
	if len(t.config.Units) > 0 {
		log.Info("Journald is not supported on your system.")
	}
	return nil
}

// seek does nothing
func (t *Tailer) seek(cursor string) error {
	return nil
}

// tail waits for message stop
func (t *Tailer) tail() {
	<-t.stop
	t.done <- struct{}{}
}
