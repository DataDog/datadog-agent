// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package tailer

import (
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"

	log "github.com/cihub/seelog"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/decoder"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

const defaultSleepDuration = 1 * time.Second
const defaultCloseTimeout = 60 * time.Second

// Tailer tails one file and sends messages to an output channel
type Tailer struct {
	path     string
	fullpath string
	file     *os.File

	readOffset        int64
	decodedOffset     int64
	shouldTrackOffset bool

	outputChan chan message.Message
	d          *decoder.Decoder
	source     *config.IntegrationConfigLogSource

	sleepDuration time.Duration
	sleepMutex    sync.Mutex

	closeTimeout time.Duration
	shouldStop   bool
	stopTimer    *time.Timer
	stopMutex    sync.Mutex
}

// NewTailer returns an initialized Tailer
func NewTailer(outputChan chan message.Message, source *config.IntegrationConfigLogSource, path string) *Tailer {
	return &Tailer{
		path:       path,
		outputChan: outputChan,
		d:          decoder.InitializeDecoder(source),
		source:     source,

		readOffset:        0,
		shouldTrackOffset: true,

		sleepDuration: defaultSleepDuration,
		sleepMutex:    sync.Mutex{},
		shouldStop:    false,
		stopMutex:     sync.Mutex{},
		closeTimeout:  defaultCloseTimeout,
	}
}

// Identifier returns a string that uniquely identifies a source
func (t *Tailer) Identifier() string {
	return fmt.Sprintf("file:%s", t.path)
}

// recoverTailing starts the tailing from the last log line processed, or now
// if we tail this file for the first time
func (t *Tailer) recoverTailing(offset int64, whence int) error {
	return t.tailFrom(offset, whence)
}

// Stop lets  the tailer stop
func (t *Tailer) Stop(shouldTrackOffset bool) {
	t.stopMutex.Lock()
	t.shouldStop = true
	t.shouldTrackOffset = shouldTrackOffset
	t.stopTimer = time.NewTimer(t.closeTimeout)
	t.stopMutex.Unlock()
}

// onStop handles the housekeeping when we stop the tailer
func (t *Tailer) onStop() {
	t.stopMutex.Lock()
	t.d.Stop()
	log.Info("Closing ", t.path)
	t.file.Close()
	t.stopTimer.Stop()
	t.stopMutex.Unlock()
}

// tailFrom let's the tailer open a file and tail from whence
func (t *Tailer) tailFrom(offset int64, whence int) error {
	t.d.Start()
	err := t.startReading(offset, whence)
	if err == nil {
		go t.forwardMessages()
	}
	return err
}

// tailFromBeginning lets the tailer start tailing its file
// from the beginning
func (t *Tailer) tailFromBeginning() error {
	return t.tailFrom(0, os.SEEK_SET)
}

// forwardMessages lets the Tailer forward log messages to the output channel
func (t *Tailer) forwardMessages() {
	for output := range t.d.OutputChan {
		if output.ShouldStop {
			return
		}

		fileMsg := message.NewFileMessage(output.Content)
		msgOffset := t.decodedOffset + int64(output.RawDataLen)
		identifier := t.Identifier()
		if !t.shouldTrackOffset {
			msgOffset = 0
			identifier = ""
		}
		t.decodedOffset = msgOffset
		msgOrigin := message.NewOrigin()
		msgOrigin.LogSource = t.source
		msgOrigin.Identifier = identifier
		msgOrigin.Offset = msgOffset
		fileMsg.SetOrigin(msgOrigin)
		t.outputChan <- fileMsg
	}
}

func (t *Tailer) shouldHardStop() bool {
	t.stopMutex.Lock()
	defer t.stopMutex.Unlock()
	if t.stopTimer != nil {
		select {
		case <-t.stopTimer.C:
			return true
		default:
		}
	}
	return false
}

func (t *Tailer) shouldSoftStop() bool {
	t.stopMutex.Lock()
	defer t.stopMutex.Unlock()
	return t.shouldStop
}

func (t *Tailer) incrementReadOffset(n int) {
	atomic.AddInt64(&t.readOffset, int64(n))
}

// GetReadOffset returns the position of the last byte read in file
func (t *Tailer) GetReadOffset() int64 {
	return atomic.LoadInt64(&t.readOffset)
}

// SetReadOffset sets the position of the last byte read in the
// file
func (t *Tailer) SetReadOffset(off int64) {
	atomic.StoreInt64(&t.readOffset, off)
}

// SetDecodedOffset sets the position of the last byte decoded in the
// file
func (t *Tailer) SetDecodedOffset(off int64) {
	atomic.StoreInt64(&t.decodedOffset, off)
}

// wait lets the tailer sleep for a bit
func (t *Tailer) wait() {
	t.sleepMutex.Lock()
	defer t.sleepMutex.Unlock()
	time.Sleep(t.sleepDuration)
}
