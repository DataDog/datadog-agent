// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

// +build !windows

package container

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"reflect"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/tagger"

	"github.com/DataDog/datadog-agent/pkg/logs/auditor"
	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/decoder"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/docker/docker/api/types"
	"github.com/moby/moby/client"
)

const defaultSleepDuration = 1 * time.Second
const tagsUpdatePeriod = 10 * time.Second

// Length of the docker message header.
// See https://godoc.org/github.com/moby/moby/client#Client.ContainerLogs:
// [8]byte{STREAM_TYPE, 0, 0, 0, SIZE1, SIZE2, SIZE3, SIZE4}[]byte{OUTPUT}
const messageHeaderLength = 8

// DockerTailer tails logs coming from stdout and stderr of a docker container
// With docker api, there is no way to know if a log comes from strout or stderr
// so if we want to capture the severity, we need to tail both in two goroutines
type DockerTailer struct {
	ContainerID   string
	outputChan    chan message.Message
	d             *decoder.Decoder
	reader        io.ReadCloser
	cli           *client.Client
	source        *config.IntegrationConfigLogSource
	containerTags []string
	tagsPayload   []byte

	sleepDuration time.Duration
	shouldStop    bool
}

// NewDockerTailer returns a new DockerTailer
func NewDockerTailer(cli *client.Client, container types.Container, source *config.IntegrationConfigLogSource, outputChan chan message.Message) *DockerTailer {
	return &DockerTailer{
		ContainerID: container.ID,
		outputChan:  outputChan,
		d:           decoder.InitializeDecoder(source),
		source:      source,
		cli:         cli,

		sleepDuration: defaultSleepDuration,
	}
}

// Identifier returns a string that uniquely identifies a source
func (dt *DockerTailer) Identifier() string {
	return fmt.Sprintf("docker:%s", dt.ContainerID)
}

// Stop stops the DockerTailer
func (dt *DockerTailer) Stop() {
	dt.shouldStop = true
	dt.d.Stop()
}

// tailFromBeginning starts the tailing from the beginning
// of the container logs
func (dt *DockerTailer) tailFromBeginning() error {
	return dt.tailFrom(time.Time{}.Format(config.DateFormat))
}

// tailFromEnd starts the tailing from the last line
// of the container logs
func (dt *DockerTailer) tailFromEnd() error {
	return dt.tailFrom(time.Now().UTC().Format(config.DateFormat))
}

// recoverTailing starts the tailing from the last log line processed, or now
// if we see this container for the first time
func (dt *DockerTailer) recoverTailing(a *auditor.Auditor) error {
	return dt.tailFrom(dt.nextLogSinceDate(a.GetLastCommittedTimestamp(dt.Identifier())))
}

// nextLogSinceDate returns the `from` value of the next log line
// for a container.
// In the auditor, we store the date of the last log line processed.
// `ContainerLogs` is not exclusive on `Since`, so if we start again
// from this date, we collect that last log line twice on restart.
// A workaround is to add one nano second, to exclude that last
// log line
func (dt *DockerTailer) nextLogSinceDate(lastTs string) string {
	ts, err := time.Parse(config.DateFormat, lastTs)
	if err != nil {
		return lastTs
	}
	ts = ts.Add(time.Nanosecond)
	return ts.Format(config.DateFormat)
}

// tailFrom starts the tailing from the specified time
func (dt *DockerTailer) tailFrom(from string) error {
	go dt.keepDockerTagsUpdated()
	dt.d.Start()
	go dt.forwardMessages()
	return dt.startReading(from)
}

// startReading starts the reader that reads the container's stdout,
// with proper configuration
func (dt *DockerTailer) startReading(from string) error {
	options := types.ContainerLogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     true,
		Timestamps: true,
		Details:    false,
		Since:      from,
	}
	reader, err := dt.cli.ContainerLogs(context.Background(), dt.ContainerID, options)
	if err != nil {
		return err
	}
	dt.reader = reader
	go dt.readForever()
	return nil
}

// readForever reads from the reader as fast as it can,
// and sleeps when there is nothing to read
func (dt *DockerTailer) readForever() {
	for {

		if dt.shouldStop {
			// this means that we stop reading as soon as we get the stop message,
			// but on the other hand we get it when the container is stopped so it should be fine
			return
		}

		inBuf := make([]byte, 4096)
		n, err := dt.reader.Read(inBuf)
		if err == io.EOF {
			// reader is closed, maybe container stopped running
			// let's close tailer. Scanner will reopen if needed
			dt.shouldStop = true
			continue
		}
		if err != nil {
			log.Println("Err:", err)
			return
		}
		if n == 0 {
			dt.wait()
			continue
		}
		dt.d.InputChan <- decoder.NewInput(inBuf[:n])
	}
}

// forwardMessages forwards decoded messages to the next pipeline,
// adding a bit of meta information
// Note: For docker container logs, we ask for the timestamp
// to store the time of the last processed line.
// As a result, we need to remove this timestamp from the log
// message before forwarding it
func (dt *DockerTailer) forwardMessages() {
	for output := range dt.d.OutputChan {
		if output.ShouldStop {
			return
		}

		ts, sev, updatedMsg, err := dt.parseMessage(output.Content)
		if err != nil {
			log.Println(err)
			continue
		}

		containerMsg := message.NewContainerMessage(updatedMsg)
		msgOrigin := message.NewOrigin()
		msgOrigin.LogSource = dt.source
		msgOrigin.Timestamp = ts
		msgOrigin.Identifier = dt.Identifier()
		containerMsg.SetSeverity(sev)
		containerMsg.SetTagsPayload(dt.tagsPayload)
		containerMsg.SetOrigin(msgOrigin)
		dt.outputChan <- containerMsg
	}
}

func (dt *DockerTailer) keepDockerTagsUpdated() {
	dt.checkForNewDockerTags()
	ticker := time.NewTicker(tagsUpdatePeriod)
	for range ticker.C {
		if dt.shouldStop {
			return
		}
		dt.checkForNewDockerTags()
	}
}

func (dt *DockerTailer) checkForNewDockerTags() {
	tags, err := tagger.Tag(containerIDToEntityName(dt.ContainerID), true)
	if err != nil {
		log.Println(err)
	} else {
		if !reflect.DeepEqual(tags, dt.containerTags) {
			dt.containerTags = tags
			dt.tagsPayload = dt.buildTagsPayload()
		}
	}
}

func (dt *DockerTailer) buildTagsPayload() []byte {
	tagsString := fmt.Sprintf("%s,%s", strings.Join(dt.containerTags, ","), dt.source.Tags)
	return config.BuildTagsPayload(tagsString, dt.source.Source, dt.source.SourceCategory)
}

// containerIDToEntityName returns a prefixed entity name from a container ID
func containerIDToEntityName(cid string) string {
	if cid == "" {
		return ""
	}
	return fmt.Sprintf("docker://%s", cid)
}

// parseMessage extracts the date and the severity from the raw docker message
// see https://godoc.org/github.com/moby/moby/client#Client.ContainerLogs
func (dt *DockerTailer) parseMessage(msg []byte) (string, []byte, []byte, error) {

	// The format of the message should be :
	// [8]byte{STREAM_TYPE, 0, 0, 0, SIZE1, SIZE2, SIZE3, SIZE4}[]byte{OUTPUT}
	// If we don't have at the very least 8 bytes we can consider this message can't be parsed.
	if len(msg) < messageHeaderLength {
		return "", nil, nil, errors.New("Can't parse docker message: expected a 8 bytes header")
	}

	// First byte is 1 for stdout and 2 for stderr
	sev := config.SevInfo
	if msg[0] == 2 {
		sev = config.SevError
	}

	// timestamp goes from byte 8 till first space
	to := bytes.Index(msg[messageHeaderLength:], []byte{' '})
	if to == -1 {
		return "", nil, nil, errors.New("Can't parse docker message: expected a whitespace after header")
	}
	to += messageHeaderLength
	ts := string(msg[messageHeaderLength:to])
	return ts, sev, msg[to+1:], nil
}

// wait lets the reader sleep for a bit
func (dt *DockerTailer) wait() {
	time.Sleep(dt.sleepDuration)
}
