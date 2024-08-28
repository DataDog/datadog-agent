// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package stream has api stream utility methods that components can use for directing output to a stream receiver
package stream

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	apiutils "github.com/DataDog/datadog-agent/comp/api/api/utils"
	logsAgent "github.com/DataDog/datadog-agent/comp/logs/agent"
	"github.com/DataDog/datadog-agent/pkg/logs/diagnostic"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// LogParams represents the parameters for streaming logs
type LogParams struct {
	// FilePath represents the output file path to write the log stream to.
	FilePath string

	// Duration represents the duration of the log stream.
	Duration time.Duration
}

// MessageReceiver is an exported interface for a valid receiver of streamed output
type MessageReceiver interface {
	SetEnabled(e bool) bool
	Filter(filters *diagnostic.Filters, done <-chan struct{}) <-chan string
}

// GetStreamFunc returns a handlerfunc that handles request to stream output to the desired receiver
func GetStreamFunc(messageReceiverFunc func() MessageReceiver, streamType, agentType string) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Infof("Got a request to stream %s.", streamType)
		w.Header().Set("Transfer-Encoding", "chunked")

		messageReceiver := messageReceiverFunc()

		flusher, ok := w.(http.Flusher)
		if !ok {
			log.Errorf("Expected a Flusher type, got: %v", w)
			return
		}

		if messageReceiver == nil {
			http.Error(w, fmt.Sprintf("The %s is not running", agentType), 405)
			flusher.Flush()
			log.Infof("The %s is not running - can't stream %s", agentType, streamType)
			return
		}

		if !messageReceiver.SetEnabled(true) {
			http.Error(w, fmt.Sprintf("Another client is already streaming %s.", streamType), 405)
			flusher.Flush()
			log.Infof("%s are already streaming. Dropping connection.", streamType)
			return
		}
		defer messageReceiver.SetEnabled(false)

		var filters diagnostic.Filters

		if r.Body != http.NoBody {
			body, err := io.ReadAll(r.Body)
			if err != nil {
				http.Error(w, log.Errorf("Error while reading HTTP request body: %s", err).Error(), 500)
				return
			}

			if err := json.Unmarshal(body, &filters); err != nil {
				http.Error(w, log.Errorf("Error while unmarshaling JSON from request body: %s", err).Error(), 500)
				return
			}
		}

		// Reset the `server_timeout` deadline for this connection as streaming holds the connection open.
		conn := apiutils.GetConnection(r)
		_ = conn.SetDeadline(time.Time{})

		done := make(chan struct{})
		defer close(done)
		logChan := messageReceiver.Filter(&filters, done)
		flushTimer := time.NewTicker(time.Second)
		defer flushTimer.Stop()
		for {
			// Handlers for detecting a closed connection (from either the server or client)
			select {
			case <-w.(http.CloseNotifier).CloseNotify(): //nolint
				return
			case <-r.Context().Done():
				return
			case line := <-logChan:
				fmt.Fprint(w, line)
			case <-flushTimer.C:
				// The buffer will flush on its own most of the time, but when we run out of logs flush so the client is up to date.
				flusher.Flush()
			}
		}
	}
}

// ExportStreamLogs export output of stream-logs to a file. Currently used for remote config stream logs
func ExportStreamLogs(la logsAgent.Component, streamLogParams *LogParams) error {
	var f *os.File
	var bufWriter *bufio.Writer

	err := apiutils.CheckDirExists(streamLogParams.FilePath)
	if err != nil {
		return fmt.Errorf("error creating directory for file %s: %v", streamLogParams.FilePath, err)
	}

	f, bufWriter, err = apiutils.OpenFileForWriting(streamLogParams.FilePath)
	if err != nil {
		return fmt.Errorf("error opening file %s for writing: %v", streamLogParams.FilePath, err)
	}
	defer func() {
		err := bufWriter.Flush()
		if err != nil {
			fmt.Printf("Error flushing buffer for log stream: %v", err)
		}
		f.Close()
	}()

	messageReceiver := la.GetMessageReceiver()

	if !messageReceiver.SetEnabled(true) {
		return fmt.Errorf("unable to enable message receiver, another client is already streaming logs")
	}
	defer messageReceiver.SetEnabled(false)

	var filters diagnostic.Filters
	done := make(chan struct{})
	defer close(done)

	logChan := messageReceiver.Filter(&filters, done)

	timer := time.NewTimer(streamLogParams.Duration)
	defer timer.Stop()

	for {
		select {
		case log := <-logChan:
			if _, err := bufWriter.WriteString(log + "\n"); err != nil {
				return fmt.Errorf("failed to write to file: %v", err)
			}
		case <-timer.C:
			return nil
		}
	}
}
