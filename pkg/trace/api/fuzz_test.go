// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build go1.18

package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/trace/api/apiutil"
	"github.com/DataDog/datadog-agent/pkg/trace/api/internal/header"
	"github.com/DataDog/datadog-agent/pkg/trace/testutil"

	"github.com/tinylib/msgp/msgp"
)

type encoder func(pb.Traces) ([]byte, error)

func commonMsgpackEncoder(traces pb.Traces) ([]byte, error) {
	return traces.MarshalMsg(nil)
}

func commonJSONEncoder(traces pb.Traces) ([]byte, error) {
	return json.Marshal(traces)
}

type decoder func([]byte) error

func commonMsgpackDecoder(traces []byte) error {
	var pbTraces pb.Traces
	_, err := pbTraces.UnmarshalMsg(traces)
	return err
}

func commonJSONDecoder(traces []byte) error {
	var pbTraces pb.Traces
	return json.NewDecoder(bytes.NewReader(traces)).Decode(&pbTraces)
}

func FuzzHandleTracesV07(f *testing.F) {
	decode := func(traces []byte) error {
		var payload pb.TracerPayload
		_, err := payload.UnmarshalMsg(traces)
		return err
	}
	fuzzTracesAPI(f, V07, "application/msgpack", commonMsgpackEncoder, decode)
}

func FuzzHandleTracesV05(f *testing.F) {
	decode := func(traces []byte) error {
		var pbTraces pb.Traces
		return pbTraces.UnmarshalMsgDictionary(traces)
	}
	fuzzTracesAPI(f, v05, "application/msgpack", commonMsgpackEncoder, decode)
}

func FuzzHandleTracesV04Msgpack(f *testing.F) {
	fuzzTracesAPI(f, v04, "application/msgpack", commonMsgpackEncoder, commonMsgpackDecoder)
}

func FuzzHandleTracesV04JSON(f *testing.F) {
	fuzzTracesAPI(f, v04, "application/json", commonJSONEncoder, commonJSONDecoder)
}

func FuzzHandleTracesV03Msgpack(f *testing.F) {
	fuzzTracesAPI(f, v03, "application/msgpack", commonMsgpackEncoder, commonMsgpackDecoder)
}

func FuzzHandleTracesV03JSON(f *testing.F) {
	fuzzTracesAPI(f, v03, "application/json", commonJSONEncoder, commonJSONDecoder)
}

func FuzzHandleTracesV02JSON(f *testing.F) {
	fuzzTracesAPI(f, v02, "application/json", commonJSONEncoder, commonJSONDecoder)
}

// fuzzTracesAPI allows fuzzing multiple trace APIs.
// The caller has to provide the targeted API version, the content type and the functions to encode and decode the payload.
func fuzzTracesAPI(f *testing.F, v Version, contentType string, encode encoder, decode decoder) {
	conf := newTestReceiverConfig()
	receiver := newTestReceiverFromConfig(conf)
	handlerFunc := receiver.handleWithVersion(v, receiver.handleTraces)
	server := httptest.NewServer(handlerFunc)
	defer server.Close()
	for _, n := range []int{1, 10, 100} {
		pbTraces := testutil.GetTestTraces(n, n, true)
		traces, err := encode(pbTraces)
		if err != nil {
			f.Fatalf("Couldn't generate seed corpus: %v", err)
		}
		f.Add(traces)
	}
	f.Fuzz(func(t *testing.T, traces []byte) {
		req, err := http.NewRequest("POST", server.URL, bytes.NewReader(traces))
		if err != nil {
			t.Fatalf("Couldn't create http request: %v", err)
		}
		req.Header.Set("Content-Type", contentType)
		var client http.Client
		resp, err := client.Do(req)
		if err != nil {
			// Most likely caused by a network issue (out of scope)
			t.Skipf("Couldn't perform http request: %v", err)
		}
		defer func() {
			if err = resp.Body.Close(); err != nil {
				t.Errorf("Couldn't close response body: %v", err)
			}
		}()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("Couldn't read response body: %v", err)
		}
		decodeErr := decode(traces)
		respContentType := resp.Header.Get("Content-Type")
		switch resp.StatusCode {
		case http.StatusOK:
			if decodeErr != nil {
				t.Fatalf("Got status code 200 for invalid request payload: %v", decodeErr)
			}
			switch respContentType {
			case "text/plain; charset=utf-8":
				expected := "OK\n"
				got := string(body)
				if expected != got {
					t.Fatalf("Unexpected message in the response body: expected (%s) got (%s)", expected, got)
				}
			case "application/json":
				var traceResp traceResponse
				if err := json.Unmarshal(body, &traceResp); err != nil {
					t.Fatalf("Got invalid response for status code 200: %v", err)
				}
			default:
				t.Fatalf("Unexpected content type (%s)", respContentType)
			}
		default:
			if decodeErr == nil {
				t.Fatalf("Unexpected status code (%d) for a valid request payload", resp.StatusCode)
			}
			if len(body) == 0 {
				// The server doesn't use a predefined format for the error message
				// so we just ensure the body isn't empty
				t.Fatal("Empty response body for an invalid request payload")
			}
			if respContentType != "text/plain; charset=utf-8" {
				t.Fatalf("Unexpected content type: expected (text/plain; charset=utf-8) got (%s)", respContentType)
			}
		}
	})
}

func FuzzHandleStats(f *testing.F) {
	cfg := newTestReceiverConfig()
	decode := func(stats []byte) (*pb.ClientStatsPayload, error) {
		reader := bytes.NewReader(stats)
		payload := &pb.ClientStatsPayload{}
		return payload, msgp.Decode(apiutil.NewLimitedReader(io.NopCloser(reader), cfg.MaxRequestBytes), payload)
	}
	receiver := newTestReceiverFromConfig(cfg)
	mockProcessor := new(mockStatsProcessor)
	receiver.statsProcessor = mockProcessor
	handlerFunc := http.HandlerFunc(receiver.handleStats)
	server := httptest.NewServer(handlerFunc)
	defer server.Close()
	pbStats := testutil.StatsPayloadSample()
	stats, err := pbStats.MarshalMsg(nil)
	if err != nil {
		f.Fatalf("Couldn't generate seed corpus: %v", err)
	}
	f.Add(stats)
	f.Fuzz(func(t *testing.T, stats []byte) {
		req, err := http.NewRequest("POST", server.URL, bytes.NewReader(stats))
		if err != nil {
			t.Fatalf("Couldn't create http request: %v", err)
		}
		req.Header.Set("Content-Type", "application/msgpack")
		req.Header.Set(header.Lang, "lang")
		req.Header.Set(header.TracerVersion, "0.0.1")
		var client http.Client
		resp, err := client.Do(req)
		if err != nil {
			// Most likely caused by a network issue (out of scope)
			t.Skipf("Couldn't perform http request: %v", err)
		}
		defer func() {
			if err = resp.Body.Close(); err != nil {
				t.Errorf("Couldn't close response body: %v", err)
			}
		}()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("Couldn't read response body: %v", err)
		}
		payload, decodeErr := decode(stats)
		switch resp.StatusCode {
		case http.StatusOK:
			if decodeErr != nil {
				t.Fatalf("Got status code 200 for invalid request payload: %v", decodeErr)
			}
			gotPayload, gotLang, gotVersion := mockProcessor.Got()
			if !reflect.DeepEqual(payload, gotPayload) {
				t.Fatalf("Expected payload (%v) got (%v)", payload, gotPayload)
			}
			if gotLang != "lang" {
				t.Fatalf("Expected lang (lang) got (%s)", gotLang)
			}
			if gotVersion != "0.0.1" {
				t.Fatalf("Expected version (0.0.1) got (%s)", gotVersion)
			}
			if len(body) != 0 {
				t.Fatalf("Expected empty response body, got (%s):", string(body))
			}
		default:
			if decodeErr == nil {
				t.Fatalf("Unexpected status code (%d) for a valid request payload", resp.StatusCode)
			}
			if len(body) == 0 {
				// The server doesn't use a predefined format for the error message
				// so we just ensure the body isn't empty
				t.Fatal("Empty response body for an invalid request payload")
			}
			respContentType := resp.Header.Get("Content-Type")
			if respContentType != "text/plain; charset=utf-8" {
				t.Fatalf("Unexpected content type: expected (text/plain; charset=utf-8) got (%s)", respContentType)
			}
		}
	})
}
