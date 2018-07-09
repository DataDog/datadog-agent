// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build docker

package container

import (
	"context"
	"fmt"
	"io"
	"reflect"
	"time"

	"github.com/DataDog/datadog-agent/pkg/tagger"
	dockerutil "github.com/DataDog/datadog-agent/pkg/util/docker"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/logs/auditor"
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
		ContainerID: containerID,
		outputChan:  outputChan,
		decoder:     decoder.InitializeDecoder(source),
		source:      source,
		cli:         cli,

		sleepDuration: defaultSleepDuration,
		stop:          make(chan struct{}, 1),
		done:          make(chan struct{}, 1),
	}
}

// Identifier returns a string that uniquely identifies a source
func (dt *Tailer) Identifier() string {
	return fmt.Sprintf("docker:%s", dt.ContainerID)
}

// Stop stops the tailer from reading new container logs,
// this call blocks until the decoder is completely flushed
func (dt *Tailer) Stop() {
	log.Info("Stop tailing container ", dt.ContainerID[:12])
	dt.stop <- struct{}{}
	dt.reader.Close()
	dt.source.RemoveInput(dt.ContainerID)
	// wait for the decoder to be flushed
	<-dt.done
}

// tailFromBeginning starts the tailing from the beginning
// of the container logs
func (dt *Tailer) tailFromBeginning() error {
	return dt.tailFrom(time.Time{}.Format(config.DateFormat))
}

// tailFromEnd starts the tailing from the last line
// of the container logs
func (dt *Tailer) tailFromEnd() error {
	return dt.tailFrom(time.Now().UTC().Format(config.DateFormat))
}

// recoverTailing starts the tailing from the last log line processed, or now
// if we see this container for the first time
func (dt *Tailer) recoverTailing(a *auditor.Auditor) error {
	return dt.tailFrom(dt.nextLogSinceDate(a.GetLastCommittedOffset(dt.Identifier())))
}

// nextLogSinceDate returns the `from` value of the next log line
// for a container.
// In the auditor, we store the date of the last log line processed.
// `ContainerLogs` is not exclusive on `Since`, so if we start again
// from this date, we collect that last log line twice on restart.
// A workaround is to add one nano second, to exclude that last
// log line
func (dt *Tailer) nextLogSinceDate(lastTs string) string {
	ts, err := time.Parse(config.DateFormat, lastTs)
	if err != nil {
		return lastTs
	}
	ts = ts.Add(time.Nanosecond)
	return ts.Format(config.DateFormat)
}

// setupReader sets up the reader that reads the container's logs
// with the proper configuration
func (dt *Tailer) setupReader(from string) (io.ReadCloser, error) {
	options := types.ContainerLogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     true,
		Timestamps: true,
		Details:    false,
		Since:      from,
	}
	return dt.cli.ContainerLogs(context.Background(), dt.ContainerID, options)
}

// tailFrom sets up and starts the tailer
func (dt *Tailer) tailFrom(from string) error {
	reader, err := dt.setupReader(from)
	if err != nil {
		// could not start the tailer
		dt.source.Status.Error(err)
		return err
	}
	dt.source.Status.Success()
	dt.source.AddInput(dt.ContainerID)

	dt.reader = reader

	go dt.keepDockerTagsUpdated()
	go dt.forwardMessages()
	dt.decoder.Start()
	go dt.readForever()

	return nil
}

// readForever reads from the reader as fast as it can,
// and sleeps when there is nothing to read
func (dt *Tailer) readForever() {
	defer dt.decoder.Stop()
	for {
		select {
		case <-dt.stop:
			// stop reading new logs from container
			return
		default:
			inBuf := make([]byte, 4096)
			n, err := dt.reader.Read(inBuf)
			if err != nil {
				// an error occurred, stop from reading new logs
				if err != io.EOF {
					dt.source.Status.Error(err)
					log.Error("Err: ", err)
				}
				return
			}
			if n == 0 {
				// wait for new data to come
				dt.wait()
				continue
			}
			dt.decoder.InputChan <- decoder.NewInput(inBuf[:n])
		}
	}
}

// forwardMessages forwards decoded messages to the next pipeline,
// adding a bit of meta information
// Note: For docker container logs, we ask for the timestamp
// to store the time of the last processed line.
// As a result, we need to remove this timestamp from the log
// message before forwarding it
func (dt *Tailer) forwardMessages() {
	defer func() {
		// the decoder has successfully been flushed
		dt.shouldStop = true
		dt.done <- struct{}{}
	}()
	for output := range dt.decoder.OutputChan {
		dockerMsg, err := parser.ParseMessage(output.Content)
		if err != nil {
			log.Warn(err)
			continue
		}
		origin := message.NewOrigin(dt.source)
		origin.Offset = dockerMsg.Timestamp
		origin.Identifier = dt.Identifier()
		origin.SetTags(dt.containerTags)
		dt.outputChan <- message.New(dockerMsg.Content, origin, dockerMsg.Status)
	}
}

func (dt *Tailer) keepDockerTagsUpdated() {
	dt.checkForNewDockerTags()
	ticker := time.NewTicker(tagsUpdatePeriod)
	for range ticker.C {
		if dt.shouldStop {
			return
		}
		dt.checkForNewDockerTags()
	}
}

func (dt *Tailer) checkForNewDockerTags() {
	tags, err := tagger.Tag(dockerutil.ContainerIDToEntityName(dt.ContainerID), true)
	if err != nil {
		log.Warn(err)
	} else {
		if !reflect.DeepEqual(tags, dt.containerTags) {
			dt.containerTags = tags
		}
	}
}

// wait lets the reader sleep for a bit
func (dt *Tailer) wait() {
	time.Sleep(dt.sleepDuration)
}
