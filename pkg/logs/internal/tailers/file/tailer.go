// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package file

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"sync/atomic"
	"time"

	coreConfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/logs/decoder"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/tag"
)

// Tailer tails one file and sends messages to an output channel
type Tailer struct {
	readOffset    int64
	decodedOffset int64
	bytesRead     int64

	// File contains the logs configuration for the File to parse (path, source, ...)
	// If you are looking for the os.File use to read on the FS, see osFile.
	File *File

	fullpath string
	osFile   *os.File
	tags     []string

	OutputChan  chan *message.Message
	decoder     *decoder.Decoder
	tagProvider tag.Provider

	sleepDuration time.Duration

	closeTimeout  time.Duration
	ShouldStop    int32
	didFileRotate int32
	stop          chan struct{}
	done          chan struct{}

	forwardContext context.Context
	stopForward    context.CancelFunc
}

// NewTailer returns an initialized Tailer
func NewTailer(outputChan chan *message.Message, file *File, sleepDuration time.Duration, decoder *decoder.Decoder) *Tailer {

	var tagProvider tag.Provider
	if file.Source.Config.Identifier != "" {
		tagProvider = tag.NewProvider(containers.BuildTaggerEntityName(file.Source.Config.Identifier))
	} else {
		tagProvider = tag.NewLocalProvider([]string{})
	}

	forwardContext, stopForward := context.WithCancel(context.Background())
	closeTimeout := coreConfig.Datadog.GetDuration("logs_config.close_timeout") * time.Second

	return &Tailer{
		File:           file,
		OutputChan:     outputChan,
		decoder:        decoder,
		tagProvider:    tagProvider,
		readOffset:     0,
		sleepDuration:  sleepDuration,
		closeTimeout:   closeTimeout,
		stop:           make(chan struct{}, 1),
		done:           make(chan struct{}, 1),
		forwardContext: forwardContext,
		stopForward:    stopForward,
	}
}

// Identifier returns a string that uniquely identifies a source.
// This is the identifier used in the registry.
// FIXME(remy): during container rotation, this Identifier() method could return
// the same value for different tailers. It is happening during container rotation
// where the dead container still has a tailer running on the log file, and the tailer
// of the freshly spawned container starts tailing this file as well.
func (t *Tailer) Identifier() string {
	return fmt.Sprintf("file:%s", t.File.Path)
}

// Start let's the tailer open a file and tail from whence
func (t *Tailer) Start(offset int64, whence int) error {
	err := t.setup(offset, whence)
	if err != nil {
		t.File.Source.Status.Error(err)
		return err
	}
	t.File.Source.Status.Success()
	t.File.Source.AddInput(t.File.Path)

	go t.forwardMessages()
	t.decoder.Start()
	go t.readForever()

	return nil
}

// DidRotate returns true if the tailer's file has been log-rotated.
// When a log rotation occurs, the file can be either:
// - renamed and recreated
// - removed and recreated
// - truncated
// readForever lets the tailer tail the content of a file
// until it is closed or the tailer is stopped.
func (t *Tailer) DidRotate() (bool, error) {
	return DidRotate(t.osFile, t.GetReadOffset())
}

func (t *Tailer) readForever() {
	defer t.onStop()
	for {
		n, err := t.read()
		if err != nil {
			return
		}
		t.recordBytes(int64(n))

		select {
		case <-t.stop:
			if n != 0 && atomic.LoadInt32(&t.didFileRotate) == 1 {
				log.Warn("Tailer stopped after rotation close timeout with remaining unread data")
			}
			// stop reading data from file
			return
		default:
			if n == 0 {
				// wait for new data to come
				t.wait()
			}
		}
	}
}

// buildTailerTags groups the file tag, directory (if wildcard path) and user tags
func (t *Tailer) buildTailerTags() []string {
	tags := []string{fmt.Sprintf("filename:%s", filepath.Base(t.File.Path))}
	if t.File.IsWildcardPath {
		tags = append(tags, fmt.Sprintf("dirname:%s", filepath.Dir(t.File.Path)))
	}
	return tags
}

// StartFromBeginning lets the tailer start tailing its file
// from the beginning
func (t *Tailer) StartFromBeginning() error {
	return t.Start(0, io.SeekStart)
}

// Stop stops the tailer and returns only when the decoder is flushed
func (t *Tailer) Stop() {
	atomic.StoreInt32(&t.didFileRotate, 0)
	t.stop <- struct{}{}
	t.File.Source.RemoveInput(t.File.Path)
	// wait for the decoder to be flushed
	<-t.done
}

// StopAfterFileRotation prepares the tailer to stop after a timeout
// to finish reading its file that has been log-rotated
func (t *Tailer) StopAfterFileRotation() {
	atomic.StoreInt32(&t.didFileRotate, 1)
	go t.startStopTimer()
	t.File.Source.RemoveInput(t.File.Path)
}

// startStopTimer initializes and starts a timer to stop the tailor after the timeout
func (t *Tailer) startStopTimer() {
	stopTimer := time.NewTimer(t.closeTimeout)
	<-stopTimer.C
	t.stopForward()
	t.stop <- struct{}{}
}

// onStop finishes to stop the tailer
func (t *Tailer) onStop() {
	t.osFile.Close()
	t.decoder.Stop()
	log.Info("Closed", t.File.Path, "for tailer key", t.File.GetScanKey(), "read", t.bytesRead, "bytes and", t.decoder.GetLineCount(), "lines")
}

// forwardMessages lets the Tailer forward log messages to the output channel
func (t *Tailer) forwardMessages() {
	defer func() {
		// the decoder has successfully been flushed
		atomic.StoreInt32(&t.ShouldStop, 1)
		close(t.done)
	}()
	for output := range t.decoder.OutputChan {
		offset := t.decodedOffset + int64(output.RawDataLen)
		identifier := t.Identifier()
		if !t.shouldTrackOffset() {
			offset = 0
			identifier = ""
		}
		t.decodedOffset = offset
		origin := message.NewOrigin(t.File.Source)
		origin.Identifier = identifier
		origin.Offset = strconv.FormatInt(offset, 10)
		origin.SetTags(append(t.tags, t.tagProvider.GetTags()...))
		// Ignore empty lines once the registry offset is updated
		if len(output.Content) == 0 {
			continue
		}
		// Make the write to the output chan cancellable to be able to stop the tailer
		// after a file rotation when it is stuck on it.
		// We don't return directly to keep the same shutdown sequence that in the
		// normal case.
		select {
		case t.OutputChan <- message.NewMessage(output.Content, origin, output.Status, output.IngestionTimestamp):
		case <-t.forwardContext.Done():
		}
	}
}

func (t *Tailer) incrementReadOffset(n int) {
	atomic.AddInt64(&t.readOffset, int64(n))
}

// SetReadOffset sets the position of the last byte read in the
// file
func (t *Tailer) SetReadOffset(off int64) {
	atomic.StoreInt64(&t.readOffset, off)
}

// GetReadOffset returns the position of the last byte read in file
func (t *Tailer) GetReadOffset() int64 {
	return atomic.LoadInt64(&t.readOffset)
}

// SetDecodedOffset sets the position of the last byte decoded in the
// file
func (t *Tailer) SetDecodedOffset(off int64) {
	atomic.StoreInt64(&t.decodedOffset, off)
}

// GetDetectedPattern returns a regexp if a pattern was detected
func (t *Tailer) GetDetectedPattern() *regexp.Regexp {
	return t.decoder.GetDetectedPattern()
}

// shouldTrackOffset returns whether the tailer should track the file offset or not
func (t *Tailer) shouldTrackOffset() bool {
	if atomic.LoadInt32(&t.didFileRotate) != 0 {
		return false
	}
	return true
}

// wait lets the tailer sleep for a bit
func (t *Tailer) wait() {
	time.Sleep(t.sleepDuration)
}

func (t *Tailer) recordBytes(n int64) {
	t.bytesRead += n
	t.File.Source.BytesRead.Add(n)
	if t.File.Source.ParentSource != nil {
		t.File.Source.ParentSource.BytesRead.Add(n)
	}
}
