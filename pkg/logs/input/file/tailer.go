// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package file

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"time"

	lineParser "github.com/DataDog/datadog-agent/pkg/logs/parser"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/decoder"
	"github.com/DataDog/datadog-agent/pkg/logs/input/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/tag"
)

// DefaultSleepDuration represents the amount of time the tailer waits before reading new data when no data is received
const DefaultSleepDuration = 1 * time.Second

const defaultCloseTimeout = 60 * time.Second

// Tailer tails one file and sends messages to an output channel
type Tailer struct {
	path           string
	fullpath       string
	file           *os.File
	isWildcardPath bool
	tags           []string

	readOffset    int64
	decodedOffset int64

	outputChan  chan *message.Message
	decoder     *decoder.Decoder
	source      *config.LogSource
	tagProvider tag.Provider

	sleepDuration time.Duration

	closeTimeout  time.Duration
	shouldStop    int32
	didFileRotate int32
	stop          chan struct{}
	done          chan struct{}
}

// NewTailer returns an initialized Tailer
func NewTailer(outputChan chan *message.Message, source *config.LogSource, path string, sleepDuration time.Duration, isWildcardPath bool) *Tailer {
	// TODO: remove those checks and add to source a reference to a tagProvider and a lineParser.
	var dc *decoder.Decoder
	var contentLenLimit = decoder.DefaultContentLenLimit
	var flushTimeout = decoder.DefaultFlushTimeout
	if source.GetSourceType() == config.KubernetesSourceType {
		var outputChan = make(chan *decoder.Output)
		var lineHandlerRunner = decoder.NewLineHandler(outputChan, kubernetes.Parser, source, contentLenLimit)
		var lineHandlerWrapper = kubernetes.NewLineHandler(lineHandlerRunner, flushTimeout, contentLenLimit)
		dc = decoder.NewDecoderWithLineHandlerRunner(outputChan, lineHandlerWrapper, contentLenLimit)
	} else {
		dc = decoder.InitializeDecoder(source, lineParser.NoopParser, contentLenLimit)
	}
	var tagProvider tag.Provider
	if source.Config.Identifier != "" {
		tagProvider = tag.NewProvider(source.Config.Identifier)
	} else {
		tagProvider = tag.NoopProvider
	}
	return &Tailer{
		path:           path,
		outputChan:     outputChan,
		decoder:        dc,
		source:         source,
		tagProvider:    tagProvider,
		readOffset:     0,
		sleepDuration:  sleepDuration,
		closeTimeout:   defaultCloseTimeout,
		stop:           make(chan struct{}, 1),
		done:           make(chan struct{}, 1),
		isWildcardPath: isWildcardPath,
	}
}

// Identifier returns a string that uniquely identifies a source
func (t *Tailer) Identifier() string {
	return fmt.Sprintf("file:%s", t.path)
}

// Start let's the tailer open a file and tail from whence
func (t *Tailer) Start(offset int64, whence int) error {
	err := t.setup(offset, whence)
	if err != nil {
		t.source.Status.Error(err)
		return err
	}
	t.source.Status.Success()
	t.source.AddInput(t.path)

	t.tagProvider.Start()
	go t.forwardMessages()
	t.decoder.Start()
	go t.readForever()

	return nil
}

// setup sets up the file tailer
func (t *Tailer) setup(offset int64, whence int) error {
	fullpath, err := filepath.Abs(t.path)
	if err != nil {
		return err
	}

	// adds metadata to enable users to filter logs by filename
	t.tags = t.buildTailerTags()

	log.Info("Opening ", t.path)
	f, err := openFile(fullpath)
	if err != nil {
		return err
	}

	t.file = f
	ret, _ := f.Seek(offset, whence)
	t.readOffset = ret
	t.decodedOffset = ret

	return nil
}

// buildTailerTags groups the file tag, directory (if wildcard path) and user tags
func (t *Tailer) buildTailerTags() []string {
	tags := []string{fmt.Sprintf("filename:%s", filepath.Base(t.path))}
	if t.isWildcardPath {
		tags = append(tags, fmt.Sprintf("dirname:%s", filepath.Dir(t.path)))
	}
	return tags
}

// readForever lets the tailer tail the content of a file
// until it is closed or the tailer is stopped.
func (t *Tailer) readForever() {
	defer t.onStop()
	for {
		select {
		case <-t.stop:
			// stop reading data from file
			return
		default:
			// keep reading data from file
			inBuf := make([]byte, 4096)
			n, err := t.file.Read(inBuf)
			if err != nil && err != io.EOF {
				// an unexpected error occurred, stop the tailor
				t.source.Status.Error(err)
				log.Error("Unexpected error occurred while reading file: ", err)
				return
			}
			if n == 0 {
				// wait for new data to come
				t.wait()
				continue
			}
			t.decoder.InputChan <- decoder.NewInput(inBuf[:n])
			t.incrementReadOffset(n)
		}
	}
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
	t.tagProvider.Stop()
	t.source.RemoveInput(t.path)
	// wait for the decoder to be flushed
	<-t.done
}

// StopAfterFileRotation prepares the tailer to stop after a timeout
// to finish reading its file that has been log-rotated
func (t *Tailer) StopAfterFileRotation() {
	atomic.StoreInt32(&t.didFileRotate, 1)
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

// forwardMessages lets the Tailer forward log messages to the output channel
func (t *Tailer) forwardMessages() {
	defer func() {
		// the decoder has successfully been flushed
		atomic.StoreInt32(&t.shouldStop, 1)
		t.done <- struct{}{}
	}()
	for output := range t.decoder.OutputChan {
		offset := t.decodedOffset + int64(output.RawDataLen)
		identifier := t.Identifier()
		if !t.shouldTrackOffset() {
			offset = 0
			identifier = ""
		}
		t.decodedOffset = offset
		origin := message.NewOrigin(t.source)
		origin.Identifier = identifier
		origin.Offset = strconv.FormatInt(offset, 10)
		origin.SetTags(append(t.tags, t.tagProvider.GetTags()...))
		t.outputChan <- message.NewMessage(output.Content, origin, output.Status)
	}
}

func (t *Tailer) incrementReadOffset(n int) {
	atomic.AddInt64(&t.readOffset, int64(n))
}

// GetReadOffset returns the position of the last byte read in file
func (t *Tailer) GetReadOffset() int64 {
	return atomic.LoadInt64(&t.readOffset)
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
