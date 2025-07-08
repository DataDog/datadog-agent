// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet || docker

package container

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker/api/types/container"

	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	auditor "github.com/DataDog/datadog-agent/comp/logs/auditor/def"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/framer"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/parsers/dockerstream"
	status "github.com/DataDog/datadog-agent/pkg/logs/status/utils"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/decoder"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/tag"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const defaultSleepDuration = 1 * time.Second

// DockerContainerLogInterface is an interface that exposes only the required function from DockerUtil
// located at pkg/util/docker/docker_util.go
type DockerContainerLogInterface interface {
	ContainerLogs(ctx context.Context, container string, options container.LogsOptions) (io.ReadCloser, error)
}

func newAPILogReader(client kubelet.KubeUtilInterface, namespace string, podName string, containerName string) func(context.Context, time.Time) (io.ReadCloser, error) {
	return func(ctx context.Context, since time.Time) (io.ReadCloser, error) {
		options := &kubelet.StreamLogOptions{
			Follow:     true,
			Timestamps: true,
			SinceTime:  &kubelet.Time{Time: since},
		}
		return client.StreamLogs(ctx, namespace, podName, containerName, options)
	}
}

func newDockerLogReader(docker DockerContainerLogInterface, containerID string) func(context.Context, time.Time) (io.ReadCloser, error) {
	return func(ctx context.Context, since time.Time) (io.ReadCloser, error) {
		options := container.LogsOptions{
			ShowStdout: true,
			ShowStderr: true,
			Follow:     true,
			Timestamps: true,
			Details:    false,
			Since:      since.Format(config.DateFormat),
		}
		return docker.ContainerLogs(ctx, containerID, options)
	}
}

// Tailer tails logs coming from stdout and stderr of a docker container
// Logs from stdout and stderr are multiplexed into a single channel and needs to be demultiplexed later on.
// To multiplex logs, docker adds a header to all logs with format '[SEV][TS] [MSG]'.
//
// This tailer contains three components, communicating with channels:
//   - readForever
//   - decoder
//   - message forwarder
type Tailer struct {
	// ContainerID is the ID of the container this tailer is tailing.
	ContainerID string

	outputChan      chan *message.Message
	decoder         *decoder.Decoder
	unsafeLogReader func(context.Context, time.Time) (io.ReadCloser, error)
	Source          *sources.LogSource
	tagProvider     tag.Provider

	readTimeout   time.Duration
	sleepDuration time.Duration

	// stop: writing a value to this channel will cause the readForever component to stop
	stop chan struct{}

	// done: a value is written to this channel when the message-forwarder
	// component is finished.
	done chan struct{}

	erroredContainerID chan string

	// reader is the io.Reader reading chunks of log data from the Docker API.
	reader *safeReader

	// registry records the progress of the tailer
	registry auditor.Registry

	// readerCancelFunc is the context cancellation function for the ongoing
	// docker-API reader.  Calling this function will cancel any pending Read
	// calls, which will return context.Canceled
	readerCancelFunc context.CancelFunc

	lastSince string
	mutex     sync.Mutex
}

// NewAPITailer returns a new Tailer that streams logs by querying the Kubelet's API
func NewAPITailer(
	client kubelet.KubeUtilInterface,
	containerID, containerName, podName, podNamespace string,
	source *sources.LogSource,
	outputChan chan *message.Message,
	erroredContainerID chan string,
	readTimeout time.Duration,
	tagger tagger.Component,
	registry auditor.Registry,
) *Tailer {
	return &Tailer{
		ContainerID:        containerID,
		outputChan:         outputChan,
		decoder:            decoder.NewDecoderWithFraming(sources.NewReplaceableSource(source), dockerstream.New(containerID), framer.DockerStream, nil, status.NewInfoRegistry()),
		Source:             source,
		tagProvider:        tag.NewProvider(types.NewEntityID(types.ContainerID, containerID), tagger),
		unsafeLogReader:    newAPILogReader(client, podNamespace, podName, containerName),
		readTimeout:        readTimeout,
		sleepDuration:      defaultSleepDuration,
		stop:               make(chan struct{}, 1),
		done:               make(chan struct{}, 1),
		erroredContainerID: erroredContainerID,
		reader:             newSafeReader(),
		registry:           registry,
	}
}

// NewDockerTailer returns a new Tailer that streams logs by connecting directly to the Docker socket
func NewDockerTailer(
	cli DockerContainerLogInterface,
	containerID string,
	source *sources.LogSource,
	outputChan chan *message.Message,
	erroredContainerID chan string,
	readTimeout time.Duration,
	tagger tagger.Component,
	registry auditor.Registry,
) *Tailer {
	return &Tailer{
		ContainerID:        containerID,
		outputChan:         outputChan,
		decoder:            decoder.NewDecoderWithFraming(sources.NewReplaceableSource(source), dockerstream.New(containerID), framer.DockerStream, nil, status.NewInfoRegistry()),
		Source:             source,
		tagProvider:        tag.NewProvider(types.NewEntityID(types.ContainerID, containerID), tagger),
		unsafeLogReader:    newDockerLogReader(cli, containerID),
		readTimeout:        readTimeout,
		sleepDuration:      defaultSleepDuration,
		stop:               make(chan struct{}, 1),
		done:               make(chan struct{}, 1),
		erroredContainerID: erroredContainerID,
		reader:             newSafeReader(),
		registry:           registry,
	}
}

// Identifier returns a string that uniquely identifies a source
func (t *Tailer) Identifier() string {
	return fmt.Sprintf("docker:%s", t.ContainerID)
}

// Stop stops the tailer from reading new container logs,
// this call blocks until the decoder is completely flushed
func (t *Tailer) Stop() {
	log.Infof("Stop tailing container: %v", t.ContainerID)

	t.registry.SetTailed(t.Identifier(), false)

	// signal the readForever component to stop
	t.stop <- struct{}{}

	// signal the reader itself to close.
	t.reader.Close()

	// signal the reader to stop a third way, by cancelling its context.  no-op
	// if already closed because of a timeout
	t.readerCancelFunc()

	t.Source.RemoveInput(t.ContainerID)

	// the closed readForever component will eventually close its channel to the decoder,
	// which will eventually close its channel to the message-forwarder component,
	// which will indicate it's done with this channel.
	<-t.done
}

// Start starts tailing from the last log line processed.
// if we see this container for the first time, it will:
// start from now if the container has been created before the agent started
// start from oldest log otherwise
func (t *Tailer) Start(since time.Time) error {
	log.Debugf("Start tailing container: %v", t.ContainerID)
	t.registry.SetTailed(t.Identifier(), true)
	return t.tail(since.Format(config.DateFormat))
}

func (t *Tailer) getLastSince() time.Time {
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
	return since
}

func (t *Tailer) setLastSince(since string) {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	t.lastSince = since
}

// setupReader sets up the reader that reads the container's logs
// with the proper configuration
func (t *Tailer) setupReader() error {
	ctx, cancelFunc := context.WithCancel(context.Background())
	reader, err := t.unsafeLogReader(ctx, t.getLastSince())
	t.reader.setUnsafeReader(reader)
	t.readerCancelFunc = cancelFunc

	return err
}

func (t *Tailer) tryRestartReader(reason string) error {
	log.Debugf("%s for container %v", reason, t.ContainerID)
	t.wait()
	err := t.setupReader()
	if err != nil {
		log.Warnf("Could not restart the docker reader for container %v: %v:", t.ContainerID, err)
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
		t.Source.Status.Error(err)
		return err
	}
	t.Source.Status.Success()
	t.Source.AddInput(t.ContainerID)

	// Start (in reverse order) the three actor components of this tailer, each
	// in dedicated goroutines:
	// - readForever, which reads data from the docker API and passes it to..
	// - the decoder, which runs in its own goroutine(s) and passes messages to..
	// - forwardMessage, which writes messages to t.outputChan.
	go t.forwardMessages()
	t.decoder.Start()
	go t.readForever()

	return nil
}

// readForever reads from the reader as fast as it can,
// and sleeps when there is nothing to read
func (t *Tailer) readForever() {
	// close the decoder's input channel when this function returns, causing it to
	// flush and close its output channel
	defer t.decoder.Stop()

	for {
		select {
		case <-t.stop:
			// stop reading new logs from container
			return
		default:
			inBuf := make([]byte, 4096)
			n, err := t.read(inBuf, t.readTimeout)

			t.Source.RecordBytes(int64(n))
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
					t.Source.Status.Error(fmt.Errorf("log decoder returns an EOF error that will trigger a Reader restart, container: %v", t.ContainerID))
					if err := t.tryRestartReader("log decoder returns an EOF error that will trigger a Reader restart"); err != nil {
						return
					}
					continue
				default:
					t.Source.Status.Error(err)
					log.Errorf("Could not tail logs for container %v: %v", t.ContainerID, err)
					t.erroredContainerID <- t.ContainerID
					return
				}
			}
			t.Source.Status.Success()
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
// wrapper over an HTTP call). If read timeouts, this function returns context.Canceled.
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
		t.readerCancelFunc()
		// wait for the Read call to return, likely with
		// context.Canceled
		<-doneReading
	}

	return n, err
}

// forwardMessages forwards decoded messages to the next pipeline,
// adding a bit of meta information
// Note: For docker container logs, we ask for the timestamp
// to store the time of the last processed line, it's part of the ParsingExtra
// struct of the message.Message.
// As a result, we need to remove this timestamp from the log
// message before forwarding it
func (t *Tailer) forwardMessages() {
	defer func() {
		// the decoder has successfully been flushed
		t.done <- struct{}{}
	}()
	for output := range t.decoder.OutputChan {
		if len(output.GetContent()) > 0 {
			origin := message.NewOrigin(t.Source)
			origin.Offset = output.ParsingExtra.Timestamp
			t.setLastSince(output.ParsingExtra.Timestamp)
			origin.Identifier = t.Identifier()
			tags := []string{}
			tags = append(tags, output.ParsingExtra.Tags...)
			tags = append(tags, t.tagProvider.GetTags()...)
			origin.SetTags(tags)
			// XXX(remy): is it OK recreating a message here?
			// Preserve ParsingExtra information from decoder output (including IsTruncated flag)
			msg := message.NewMessageWithParsingExtra(output.GetContent(), origin, output.Status, output.IngestionTimestamp, output.ParsingExtra)
			t.outputChan <- msg
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
