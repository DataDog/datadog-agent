// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package http

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

const (
	// STREAM_INACTIVITY_TIMEOUT_SECONDS specifies the amount of time to wait between receiving data
	// before a stall condition is detected and the connection is backed off.
	STREAM_INACTIVITY_TIMEOUT_SECONDS int = 90
)

type HttpStream struct {
	// HttpClient can be set to provide a custom HTTP client, useful if URL serves a self-signed SSL cert and validation errors need to be ignored, for example.
	HttpClient *http.Client
	// HttpRequest can be set to provide a custom HTTP request, useful in cases where the default HTTP GET verb is not appropriate, for example.
	HttpRequest *http.Request
	// Data provides the data channel that is handed each data chunk that is read from the stream.
	Data chan []byte
	// Error can be read to be notified of any connection errors that occur during the lifetime of the stream.
	// Fatal errors will be delivered on this channel before the stream is closed permanently via Close().
	// Reading from this channel is optional, it will not block if there is no reader.
	Error chan error
	// Exit can be read to be notified when the stream has exited permanently e.g. due to Close() being called, or a fatal error occurring.
	// Reading from this channel is optional, it will not block if there is no reader.
	Exit      chan bool
	exiting   bool
	waitGroup *sync.WaitGroup
}

// Connect to the configured URL and begin reading data.
func (s *HttpStream) Connect() {
	go s.enterReadStreamLoop()
}

// Close permanently disconnects the stream reader and cleans up all resources.
func (s *HttpStream) Close() {
	if s.exiting {
		return
	}
	s.exiting = true
	close(s.Exit)
	go func() {
		s.waitGroup.Wait()
		close(s.Data)
		close(s.Error)
	}()
}

func (s *HttpStream) sendErr(err error) {
	// write to error chan without blocking if there are no readers
	select {
	case s.Error <- err:
	default:
	}
}

func (s *HttpStream) connect() (*http.Response, error) {
	resp, err := s.HttpClient.Do(s.HttpRequest)
	if err != nil {
		return nil, err
	}

	return resp, nil
}

func (s *HttpStream) connectAndReadStream() {
	resp, err := s.connect()
	if err != nil {
		// TODO Differentiate between transient tcp/ip errors and fatal errors (such as malformed url etc.)
		// and close the stream if appropriate.
		s.sendErr(err)
		return
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case 200:
		fmt.Println("READING STERAM DATA")
		s.enterReadDataLoop(resp.Body)
	default:
		err = fmt.Errorf("Encountered unhandled status code: %v", resp.StatusCode)
		s.sendErr(err)
		s.Close()
	}
}

func (s *HttpStream) enterReadStreamLoop() {
	s.waitGroup.Add(1)
	defer s.waitGroup.Done()
	for {
		select {
		case <-s.Exit:
			return
		default:
			s.connectAndReadStream()
		}
	}
}

func (s *HttpStream) enterReadDataLoop(reader io.Reader) {
	scanner := bufio.NewScanner(reader)
	for {
		dataCh, errCh := s.readData(scanner)
		select {
		case data := <-dataCh:
			if len(data) > 0 { // drop empty heartbeats
				s.Data <- data
			}
		case <-s.Exit:
			return
		case err := <-errCh:
			s.sendErr(err)
			return
		case <-time.After(time.Duration(STREAM_INACTIVITY_TIMEOUT_SECONDS) * time.Second):
			fmt.Printf("Stream inactive for %d seconds; leaving read data loop.", STREAM_INACTIVITY_TIMEOUT_SECONDS)
			return
		}
	}
}

func (s *HttpStream) readData(scanner *bufio.Scanner) (<-chan []byte, <-chan error) {
	dataCh := make(chan []byte)
	errCh := make(chan error)
	go func() {
		if ok := scanner.Scan(); !ok {
			errCh <- scanner.Err()
			return
		}
		dataCh <- scanner.Bytes()[:]
	}()
	return dataCh, errCh
}

// NewStream creates a new stream instance.
func NewStream(client *http.Client, req *http.Request) *HttpStream {
	s := HttpStream{
		HttpClient:  client,
		HttpRequest: req,
		Data:        make(chan []byte),
		Error:       make(chan error),
		Exit:        make(chan bool),
		waitGroup:   &sync.WaitGroup{},
	}
	return &s
}
