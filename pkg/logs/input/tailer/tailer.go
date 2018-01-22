// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build !windows

package tailer

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	log "github.com/cihub/seelog"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/decoder"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

// DefaultWaitDuration represents the amount of time the tailer waits when no more data can be read
const DefaultWaitDuration = 1 * time.Second

const defaultCloseTimeout = 60 * time.Second

// Tailer tails one file and sends messages to an output channel
type Tailer struct {
	path string
	file *os.File

	readOffset    int64
	decodedOffset int64

	outputChan chan message.Message
	d          *decoder.Decoder
	source     *config.IntegrationConfigLogSource

	sleepDuration time.Duration

	closeTimeout  time.Duration
	didFileRotate bool
	shouldStop    bool
	stopTimer     *time.Timer
	mu            *sync.Mutex
	wg            *sync.WaitGroup
}

// NewTailer returns an initialized Tailer
func NewTailer(outputChan chan message.Message, source *config.IntegrationConfigLogSource, path string, sleepDuration time.Duration, wg *sync.WaitGroup) *Tailer {
	return &Tailer{
		path:       path,
		outputChan: outputChan,
		d:          decoder.InitializeDecoder(source),
		source:     source,

		readOffset: 0,

		sleepDuration: sleepDuration,

		closeTimeout: defaultCloseTimeout,
		mu:           &sync.Mutex{},
		wg:           wg,
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

// Stop lets the tailer stop
func (t *Tailer) Stop(didFileRotate bool) {
	t.mu.Lock()
	t.shouldStop = true
	t.didFileRotate = didFileRotate
	t.stopTimer = time.NewTimer(t.closeTimeout)
	t.mu.Unlock()
}

// onStop handles the housekeeping when we stop the tailer
func (t *Tailer) onStop() {
	t.d.Stop()
	log.Info("Closing ", t.path)
	t.file.Close()
	t.stopTimer.Stop()
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

func (t *Tailer) startReading(offset int64, whence int) error {
	fullpath, err := filepath.Abs(t.path)
	if err != nil {
		t.source.Tracker.TrackError(err)
		return err
	}
	log.Info("Opening ", t.path)
	f, err := os.Open(fullpath)
	if err != nil {
		t.source.Tracker.TrackError(err)
		return err
	}
	t.source.Tracker.TrackSuccess()

	ret, _ := f.Seek(offset, whence)
	t.file = f
	t.readOffset = ret
	t.decodedOffset = ret

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
	for output := range t.d.OutputChan {
		if output.ShouldStop {
			t.wg.Done()
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

// readForever lets the tailer tail the content of a file
// until it is closed.
func (t *Tailer) readForever() {
	for {
		t.mu.Lock()
		if t.shouldHardStop() {
			t.onStop()
			t.mu.Unlock()
			return
		}

		inBuf := make([]byte, 4096)
		n, err := t.file.Read(inBuf)
		if err == io.EOF {
			if t.shouldSoftStop() {
				t.onStop()
				t.mu.Unlock()
				return
			}
			t.wait()
			t.mu.Unlock()
			continue
		}
		if err != nil {
			t.source.Tracker.TrackError(err)
			log.Error("Err: ", err)
			t.onStop()
			t.mu.Unlock()
			return
		}
		if n == 0 {
			t.wait()
			t.mu.Unlock()
			continue
		}
		t.d.InputChan <- decoder.NewInput(inBuf[:n])
		t.incrementReadOffset(n)
		t.mu.Unlock()
	}
}

func (t *Tailer) shouldHardStop() bool {
	if !t.shouldStop {
		return false
	}
	if !t.didFileRotate {
		return true
	}
	select {
	case <-t.stopTimer.C:
		return true
	default:
		return false
	}
}

func (t *Tailer) shouldSoftStop() bool {
	return t.shouldStop
}

func (t *Tailer) shouldTrackOffset() bool {
	return !t.didFileRotate
}

func (t *Tailer) incrementReadOffset(n int) {
	atomic.AddInt64(&t.readOffset, int64(n))
}

// GetReadOffset returns the position of the last byte read in file
func (t *Tailer) GetReadOffset() int64 {
	return atomic.LoadInt64(&t.readOffset)
}

// wait lets the tailer sleep for a bit
func (t *Tailer) wait() {
	time.Sleep(t.sleepDuration)
}
