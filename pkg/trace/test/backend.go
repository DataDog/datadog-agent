// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package test

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/DataDog/datadog-agent/pkg/trace/stats"
	"github.com/gogo/protobuf/proto"
)

// defaultBackendAddress is the default listening address for the fake
// backend.
const defaultBackendAddress = "localhost:8888"

// defaultChannelSize is the default size of the buffered channel
// receiving any payloads sent by the trace-agent to the backend.
const defaultChannelSize = 100

type fakeBackend struct {
	srv     http.Server
	out     chan interface{} // payload output
	started uint64           // 0 if server is stopped
}

func newFakeBackend(channelSize int) *fakeBackend {
	size := defaultChannelSize
	if channelSize != 0 {
		size = channelSize
	}
	fb := fakeBackend{
		out: make(chan interface{}, size),
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v0.2/traces", fb.handleTraces)
	mux.HandleFunc("/api/v0.2/stats", fb.handleStats)
	mux.HandleFunc("/_health", fb.handleHealth)

	fb.srv = http.Server{
		Addr:    defaultBackendAddress,
		Handler: mux,
	}
	return &fb
}

func (s *fakeBackend) Start() error {
	if atomic.LoadUint64(&s.started) > 0 {
		// already running
		return nil
	}
	go func() {
		atomic.StoreUint64(&s.started, 1)
		defer atomic.StoreUint64(&s.started, 0)
		if err := s.srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server: %v", err)
		}
	}()

	timeout := time.After(5 * time.Second)
	for {
		select {
		case <-timeout:
			return errors.New("server: timed out out waiting for start")
		default:
			resp, err := http.Get(fmt.Sprintf("http://%s/_health", s.srv.Addr))
			if err == nil && resp.StatusCode == http.StatusOK {
				return nil
			}
			time.Sleep(5 * time.Millisecond)
		}
	}
}

func (s *fakeBackend) Out() <-chan interface{} { return s.out }

// Shutdown shuts down the backend and stops any running agent.
func (s *fakeBackend) Shutdown(wait time.Duration) error {
	defer close(s.out)

	ctx, cancel := context.WithTimeout(context.Background(), wait)
	defer cancel()
	return s.srv.Shutdown(ctx)
}

func (s *fakeBackend) handleHealth(w http.ResponseWriter, req *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func (s *fakeBackend) handleStats(w http.ResponseWriter, req *http.Request) {
	var payload stats.Payload
	if err := readJSONRequest(req, &payload); err != nil {
		log.Println("server: error reading stats: ", err)
	}
	s.out <- payload
}

func (s *fakeBackend) handleTraces(w http.ResponseWriter, req *http.Request) {
	var payload pb.TracePayload
	if err := readProtoRequest(req, &payload); err != nil {
		log.Println("server: error reading traces: ", err)
	}
	s.out <- payload
}

func readJSONRequest(req *http.Request, v interface{}) error {
	rc, err := readCloserFromRequest(req)
	if err != nil {
		return err
	}
	defer rc.Close()
	return json.NewDecoder(rc).Decode(v)
}

func readProtoRequest(req *http.Request, msg proto.Message) error {
	rc, err := readCloserFromRequest(req)
	if err != nil {
		return err
	}
	slurp, err := ioutil.ReadAll(rc)
	defer rc.Close()
	if err != nil {
		return err
	}
	return proto.Unmarshal(slurp, msg)
}

func readCloserFromRequest(req *http.Request) (io.ReadCloser, error) {
	rc := struct {
		io.Reader
		io.Closer
	}{
		Reader: req.Body,
		Closer: req.Body,
	}
	if req.Header.Get("Accept-Encoding") == "gzip" {
		gz, err := gzip.NewReader(req.Body)
		if err != nil {
			return nil, err
		}
		defer gz.Close()
		rc.Reader = gz
	}
	return rc, nil
}
