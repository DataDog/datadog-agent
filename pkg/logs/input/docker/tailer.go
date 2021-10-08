// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build docker

package docker

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	dockerutil "github.com/DataDog/datadog-agent/pkg/util/docker"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/decoder"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/tag"

	"github.com/docker/docker/api/types"
)

const defaultSleepDuration = 1 * time.Second

type dockerContainerLogInterface interface {
	ContainerLogs(ctx context.Context, container string, options types.ContainerLogsOptions) (io.ReadCloser, error)
}

// Tailer tails logs coming from stdout and stderr of a docker container
// Logs from stdout and stderr are multiplexed into a single channel and needs to be demultiplexed later one.
// To multiplex logs, docker adds a header to all logs with format '[SEV][TS] [MSG]'.
type Tailer struct {
	ContainerID string
	outputChan  chan *message.Message
	decoder     *decoder.Decoder
	reader      *safeReader
	dockerutil  dockerContainerLogInterface
	source      *config.LogSource
	tagProvider tag.Provider

	readTimeout        time.Duration
	sleepDuration      time.Duration
	shouldStop         bool
	stop               chan struct{}
	done               chan struct{}
	erroredContainerID chan string
	cancelFunc         context.CancelFunc
	lastSince          string
	mutex              sync.Mutex
}

// NewTailer returns a new Tailer
func NewTailer(cli *dockerutil.DockerUtil, containerID string, source *config.LogSource, outputChan chan *message.Message, erroredContainerID chan string, readTimeout time.Duration) *Tailer {
	return &Tailer{
		ContainerID:        containerID,
		outputChan:         outputChan,
		decoder:            InitializeDecoder(source, containerID),
		source:             source,
		tagProvider:        tag.NewProvider(dockerutil.ContainerIDToTaggerEntityName(containerID)),
		dockerutil:         cli,
		readTimeout:        readTimeout,
		sleepDuration:      defaultSleepDuration,
		stop:               make(chan struct{}, 1),
		done:               make(chan struct{}, 1),
		erroredContainerID: erroredContainerID,
		reader:             newSafeReader(),
	}
}

// Identifier returns a string that uniquely identifies a source
func (t *Tailer) Identifier() string {
	return fmt.Sprintf("docker:%s", t.ContainerID)
}

// Stop stops the tailer from reading new container logs,
// this call blocks until the decoder is completely flushed
func (t *Tailer) Stop() {
	log.Infof("Stop tailing container: %v", ShortContainerID(t.ContainerID))
	t.stop <- struct{}{}

	t.reader.Close()
	// no-op if already closed because of a timeout
	t.cancelFunc()
	t.source.RemoveInput(t.ContainerID)
	// wait for the decoder to be flushed
	<-t.done
}

// Start starts tailing from the last log line processed.
// if we see this container for the first time, it will:
// start from now if the container has been created before the agent started
// start from oldest log otherwise
func (t *Tailer) Start(since time.Time) error {
	log.Debugf("Start tailing container: %v", ShortContainerID(t.ContainerID))
	return t.tail(since.Format(config.DateFormat))
}

func (t *Tailer) getLastSince() string {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	since, err := time.Parse(config.DateFormat, t.lastSince)
	if err != nil {
		since = time.Now().UTC()
	} else {
		// To avoid sending the last recorded log we add a nanosecond
		// to the offset
		since = since.Add(time.Nanosecond)
	}
	return since.Format(config.DateFormat)
}

func (t *Tailer) setLastSince(since string) {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	t.lastSince = since
}

// setupReader sets up the reader that reads the container's logs
// with the proper configuration
func (t *Tailer) setupReader() error {
	options := types.ContainerLogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     true,
		Timestamps: true,
		Details:    false,
		Since:      t.getLastSince(),
	}
	ctx, cancelFunc := context.WithCancel(context.Background())
	reader, err := t.dockerutil.ContainerLogs(ctx, t.ContainerID, options)
	t.reader.setUnsafeReader(reader)
	t.cancelFunc = cancelFunc

	return err
}

func (t *Tailer) tryRestartReader(reason string) error {
	log.Debugf("%s for container %v", reason, ShortContainerID(t.ContainerID))
	t.wait()
	err := t.setupReader()
	if err != nil {
		log.Warnf("Could not restart the docker reader for container %v: %v:", ShortContainerID(t.ContainerID), err)
		t.erroredContainerID <- t.ContainerID
	}
	return err
}

// tail sets up and starts the tailer
func (t *Tailer) tail(since string) error {
	t.setLastSince(since)
	err := t.setupReader()
	if err != nil {
		// could not start the tailer
		t.source.Status.Error(err)
		return err
	}
	t.source.Status.Success()
	t.source.AddInput(t.ContainerID)

	go t.forwardMessages()
	t.decoder.Start()
	go t.readForever()

	return nil
}

// readForever reads from the reader as fast as it can,
// and sleeps when there is nothing to read
func (t *Tailer) readForever() {
	defer t.decoder.Stop()
	for {
		select {
		case <-t.stop:
			// stop reading new logs from container
			return
		default:
			inBuf := make([]byte, 4096)
			n, err := t.read(inBuf, t.readTimeout)

			// Since `container_collect_all` reports all docker logs as a single source (even though the source is overridden internally),
			// we need to report the byte count to the parent source used to populate the status page.
			if t.source.ParentSource != nil {
				t.source.ParentSource.BytesRead.Add(int64(n))
			} else {
				t.source.BytesRead.Add(int64(n))
			}
			if err != nil { // an error occurred, stop from reading new logs
				switch {
				case isReaderClosed(err):
					// The reader has been closed during the shut down process
					// of the tailer, stop reading
					return
				case isContextCanceled(err):
					// Note that it could happen that the docker daemon takes a lot of time gathering timestamps
					// before starting to send any data when it has stored several large log files.
					// Increasing the docker_client_read_timeout could help avoiding such a situation.
					if err := t.tryRestartReader("Restarting reader after a read timeout"); err != nil {
						return
					}
					continue
				case isClosedConnError(err):
					// This error is raised when the agent is stopping
					return
				case isFileAlreadyClosed(err):
					// This error seems to be returned by Docker for Windows
					// See: https://github.com/microsoft/go-winio/blob/master/file.go
					// We can probably just wait to get more data
					continue
				case err == io.EOF:
					// This error is raised when:
					// * the container is stopping.
					// * when the container has not started to output logs yet.
					// * during a file rotation.
					// restart the reader (restartReader() include 1second wait)
					t.source.Status.Error(fmt.Errorf("log decoder returns an EOF error that will trigger a Reader restart, container: %v", ShortContainerID(t.ContainerID)))
					if err := t.tryRestartReader("log decoder returns an EOF error that will trigger a Reader restart"); err != nil {
						return
					}
					continue
				default:
					t.source.Status.Error(err)
					log.Errorf("Could not tail logs for container %v: %v", ShortContainerID(t.ContainerID), err)
					t.erroredContainerID <- t.ContainerID
					return
				}
			}
			t.source.Status.Success()
			if n == 0 {
				// wait for new data to come
				t.wait()
				continue
			}
			t.decoder.InputChan <- decoder.NewInput(inBuf[:n])
		}
	}
}

// read implement a timeout on t.reader.Read() because it can be blocking (it's a
// wrapper over an HTTP call). If read timeouts, the tailer will be restarted.
func (t *Tailer) read(buffer []byte, timeout time.Duration) (int, error) {
	var n int
	var err error
	doneReading := make(chan struct{})
	go func() {
		n, err = t.reader.Read(buffer)
		close(doneReading)
	}()

	select {
	case <-doneReading:
	case <-time.After(timeout):
		// Cancel the docker socket reader context
		t.cancelFunc()
		<-doneReading
	}

	return n, err
}

// forwardMessages forwards decoded messages to the next pipeline,
// adding a bit of meta information
// Note: For docker container logs, we ask for the timestamp
// to store the time of the last processed line.
// As a result, we need to remove this timestamp from the log
// message before forwarding it
func (t *Tailer) forwardMessages() {
	defer func() {
		// the decoder has successfully been flushed
		t.shouldStop = true
		t.done <- struct{}{}
	}()
	for output := range t.decoder.OutputChan {
		if len(output.Content) > 0 {
			origin := message.NewOrigin(t.source)
			origin.Offset = output.Timestamp
			t.setLastSince(output.Timestamp)
			origin.Identifier = t.Identifier()
			origin.SetTags(t.tagProvider.GetTags())
			t.outputChan <- message.NewMessage(output.Content, origin, output.Status, output.IngestionTimestamp)
		}
	}
}

// wait lets the reader sleep for a bit
func (t *Tailer) wait() {
	time.Sleep(t.sleepDuration)
}

// isConnClosedError returns true if the error is related to a closed connection,
// for more details, see: https://golang.org/src/internal/poll/fd.go#L18.
func isClosedConnError(err error) bool {
	return strings.Contains(err.Error(), "use of closed network connection")
}

// isContextCanceled returns true if the error is related to a canceled context,
func isContextCanceled(err error) bool {
	return err == context.Canceled
}

// isReaderClosed returns true if a reader has been closed.
func isReaderClosed(err error) bool {
	return strings.Contains(err.Error(), "http: read on closed response body")
}

// isFileAlreadyClosed returns true if file is already closing
func isFileAlreadyClosed(err error) bool {
	return strings.Contains(err.Error(), "file has already been closed")
}
