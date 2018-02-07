// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package tailer

import (
	"fmt"
	"os"
	"sync/atomic"
	"time"

	log "github.com/cihub/seelog"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/decoder"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

// DefaultSleepDuration represents the amount of time the tailer waits before reading new data when no data is received
const DefaultSleepDuration = 1 * time.Second

const defaultCloseTimeout = 60 * time.Second

// Tailer tails one file and sends messages to an output channel
type Tailer struct {
	path     string
	fullpath string
	file     *os.File

	readOffset    int64
	decodedOffset int64

	outputChan chan message.Message
	decoder    *decoder.Decoder
	source     *config.LogSource

	sleepDuration time.Duration

	closeTimeout  time.Duration
	shouldStop    bool
	didFileRotate bool
	stop          chan struct{}
	done          chan struct{}
}

// NewTailer returns an initialized Tailer
func NewTailer(outputChan chan message.Message, source *config.LogSource, path string, sleepDuration time.Duration) *Tailer {
	return &Tailer{
		path:          path,
		outputChan:    outputChan,
		decoder:       decoder.InitializeDecoder(source),
		source:        source,
		readOffset:    0,
		sleepDuration: sleepDuration,
		closeTimeout:  defaultCloseTimeout,
		stop:          make(chan struct{}, 1),
		done:          make(chan struct{}, 1),
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

// Stop stops the tailer and returns only when the decoder is flushed
func (t *Tailer) Stop() {
	t.didFileRotate = false
	t.stop <- struct{}{}
	t.source.RemoveInput(t.path)
	// wait for the decoder to be flushed
	<-t.done
}

// StopAfterFileRotation prepares the tailer to stop after a timeout
// to finish reading its file that has been log-rotated
func (t *Tailer) StopAfterFileRotation() {
	t.didFileRotate = true
	go t.startStopTimer()
	t.source.RemoveInput(t.path)
}

// startStopTimer initialises and starts a timer to stop the tailor after the timeout
func (t *Tailer) startStopTimer() {
	stopTimer := time.NewTimer(t.closeTimeout)
	<-stopTimer.C
	t.stop <- struct{}{}
}

// onStop finishes to stop the tailer
func (t *Tailer) onStop() {
	log.Info("Closing ", t.path)
	t.file.Close()
	t.decoder.Stop()
}

// tailFrom let's the tailer open a file and tail from whence
func (t *Tailer) tailFrom(offset int64, whence int) error {
	err := t.setup(offset, whence)
	if err != nil {
		t.source.Status.Error(err)
		return err
	}
	t.source.Status.Success()
	t.source.AddInput(t.path)

	t.decoder.Start()
	go t.forwardMessages()
	go t.readForever()

	return nil
}

// tailFromBeginning lets the tailer start tailing its file
// from the beginning
func (t *Tailer) tailFromBeginning() error {
	return t.tailFrom(0, os.SEEK_SET)
}

// forwardMessages lets the Tailer forward log messages to the output channel
func (t *Tailer) forwardMessages() {
	defer func() {
		// the decoder has successfully been flushed
		t.shouldStop = true
		t.done <- struct{}{}
	}()
	for output := range t.decoder.OutputChan {
		if output.ShouldStop {
			// the decoder has been stopped, there is no more message to forward
			return
		}
		fileMsg := message.NewFileMessage(output.Content)
		msgOffset := t.decodedOffset + int64(output.RawDataLen)
		identifier := t.Identifier()
		if !t.shouldTrackOffset() {
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

// shouldTrackOffset returns whether the tailer should track the file offset or not
func (t *Tailer) shouldTrackOffset() bool {
	if t.didFileRotate {
		return false
	}
	return true
}

// wait lets the tailer sleep for a bit
func (t *Tailer) wait() {
	time.Sleep(t.sleepDuration)
}
