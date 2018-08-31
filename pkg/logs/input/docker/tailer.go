// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build docker

package docker

import (
	"context"
	"fmt"
	"io"
	"reflect"
	"time"

	"github.com/DataDog/datadog-agent/pkg/tagger"
	dockerutil "github.com/DataDog/datadog-agent/pkg/util/docker"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/decoder"
	parser "github.com/DataDog/datadog-agent/pkg/logs/docker"
	"github.com/DataDog/datadog-agent/pkg/logs/message"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
)

const defaultSleepDuration = 1 * time.Second
const tagsUpdatePeriod = 10 * time.Second

// Tailer tails logs coming from stdout and stderr of a docker container
// With docker api, there is no way to know if a log comes from strout or stderr
// so if we want to capture the severity, we need to tail both in two goroutines
type Tailer struct {
	ContainerID   string
	outputChan    chan message.Message
	decoder       *decoder.Decoder
	reader        io.ReadCloser
	cli           *client.Client
	source        *config.LogSource
	containerTags []string

	sleepDuration time.Duration
	shouldStop    bool
	stop          chan struct{}
	done          chan struct{}
}

// NewTailer returns a new Tailer
func NewTailer(cli *client.Client, containerID string, source *config.LogSource, outputChan chan message.Message) *Tailer {
	return &Tailer{
		ContainerID:   containerID,
		outputChan:    outputChan,
		decoder:       decoder.InitializeDecoder(source),
		source:        source,
		cli:           cli,
		sleepDuration: defaultSleepDuration,
		stop:          make(chan struct{}, 1),
		done:          make(chan struct{}, 1),
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
	t.source.RemoveInput(t.ContainerID)
	// wait for the decoder to be flushed
	<-t.done
}

// Start starts tailing from the last log line processed.
// if we see this container for the first time, it will:
// start from now if the container has been created before the agent started
// start from oldest log otherwise
func (t *Tailer) Start(since time.Time) error {
	log.Infof("Start tailing container: %v", containerImage, ShortContainerID(containerID))
	return t.tail(since.Format(config.DateFormat))
}

// setupReader sets up the reader that reads the container's logs
// with the proper configuration
func (t *Tailer) setupReader(since string) (io.ReadCloser, error) {
	options := types.ContainerLogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     true,
		Timestamps: true,
		Details:    false,
		Since:      since,
	}
	return t.cli.ContainerLogs(context.Background(), t.ContainerID, options)
}

// tail sets up and starts the tailer
func (t *Tailer) tail(since string) error {
	reader, err := t.setupReader(since)
	if err != nil {
		// could not start the tailer
		t.source.Status.Error(err)
		return err
	}
	t.source.Status.Success()
	t.source.AddInput(t.ContainerID)

	t.reader = reader

	go t.keepDockerTagsUpdated()
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
			n, err := t.reader.Read(inBuf)
			if err != nil {
				// an error occurred, stop from reading new logs
				if err != io.EOF {
					t.source.Status.Error(err)
					log.Error("Err: ", err)
				}
				return
			}
			if n == 0 {
				// wait for new data to come
				t.wait()
				continue
			}
			t.decoder.InputChan <- decoder.NewInput(inBuf[:n])
		}
	}
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
		dockerMsg, err := parser.ParseMessage(output.Content)
		if err != nil {
			log.Warn(err)
			continue
		}
		if len(dockerMsg.Content) > 0 {
			origin := message.NewOrigin(t.source)
			origin.Offset = dockerMsg.Timestamp
			origin.Identifier = t.Identifier()
			origin.SetTags(t.containerTags)
			t.outputChan <- message.New(dockerMsg.Content, origin, dockerMsg.Status)
		}
	}
}

func (t *Tailer) keepDockerTagsUpdated() {
	t.checkForNewDockerTags()
	ticker := time.NewTicker(tagsUpdatePeriod)
	for range ticker.C {
		if t.shouldStop {
			return
		}
		t.checkForNewDockerTags()
	}
}

func (t *Tailer) checkForNewDockerTags() {
	tags, err := tagger.Tag(dockerutil.ContainerIDToEntityName(t.ContainerID), true)
	if err != nil {
		log.Warn(err)
	} else {
		if !reflect.DeepEqual(tags, t.containerTags) {
			t.containerTags = tags
		}
	}
}

// wait lets the reader sleep for a bit
func (t *Tailer) wait() {
	time.Sleep(t.sleepDuration)
}
