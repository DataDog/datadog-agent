// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/tagger/origindetection"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/trace/api/internal/header"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/info"
	"github.com/DataDog/datadog-agent/pkg/trace/sampler"
	"github.com/DataDog/datadog-agent/pkg/trace/telemetry"
	"github.com/DataDog/datadog-agent/pkg/trace/testutil"
	"github.com/DataDog/datadog-agent/pkg/trace/timing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tinylib/msgp/msgp"
	vmsgp "github.com/vmihailenco/msgpack/v4"

	"github.com/DataDog/datadog-go/v5/statsd"
)

// Traces shouldn't come from more than 5 different sources
var langs = []string{"python", "ruby", "go", "java", "C#"}

// headerFields is a map used to decode the header metas
var headerFields = map[string]string{
	"lang":           "Datadog-Meta-Lang",
	"lang_version":   "Datadog-Meta-Lang-Version",
	"interpreter":    "Datadog-Meta-Lang-Interpreter",
	"tracer_version": "Datadog-Meta-Tracer-Version",
}

type noopStatsProcessor struct{}

func (noopStatsProcessor) ProcessStats(_ *pb.ClientStatsPayload, _, _, _, _ string) {}

func newTestReceiverFromConfig(conf *config.AgentConfig) *HTTPReceiver {
	dynConf := sampler.NewDynamicConfig()

	rawTraceChan := make(chan *Payload, 5000)
	receiver := NewHTTPReceiver(conf, dynConf, rawTraceChan, noopStatsProcessor{}, telemetry.NewNoopCollector(), &statsd.NoOpClient{}, &timing.NoopReporter{})

	return receiver
}

func newTestReceiverConfig() *config.AgentConfig {
	conf := config.New()
	conf.Endpoints[0].APIKey = "test"
	conf.DecoderTimeout = 10000
	conf.ReceiverTimeout = 1
	conf.ReceiverPort = 8326 // use non-default port to avoid conflict with a running agent

	return conf
}

func TestMain(m *testing.M) {
	// We're about to os.Exit, no need to revert this value to original
	killProcess = func(format string, args ...interface{}) {
		fmt.Printf(format, args...)
		fmt.Println()
	}
	os.Exit(m.Run())
}

func TestServerShutdown(t *testing.T) {
	// prepare the msgpack payload
	bts, err := testutil.GetTestTraces(10, 10, true).MarshalMsg(nil)
	assert.Nil(t, err)

	// prepare the receiver
	conf := newTestReceiverConfig()
	conf.ReceiverSocket = t.TempDir() + "/somesock.sock"
	dynConf := sampler.NewDynamicConfig()

	rawTraceChan := make(chan *Payload)
	receiver := NewHTTPReceiver(conf, dynConf, rawTraceChan, noopStatsProcessor{}, telemetry.NewNoopCollector(), &statsd.NoOpClient{}, &timing.NoopReporter{})

	receiver.Start()

	go func() {
		for {
			// simulate the channel being busy
			time.Sleep(100 * time.Millisecond)
			_, ok := <-rawTraceChan
			if !ok {
				return
			}
		}
	}()

	// Create two clients - one for TCP and one for UDS
	tcpClient := http.Client{Timeout: 10 * time.Second}
	udsClient := http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return net.Dial("unix", conf.ReceiverSocket)
			},
		},
	}

	wg := &sync.WaitGroup{}

	// Send requests to both TCP and UDS endpoints
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for n := 0; n < 200; n++ {
				// Send to TCP endpoint
				req, _ := http.NewRequest("POST", "http://localhost:8326/v0.4/traces", bytes.NewReader(bts))
				req.Header.Set("Content-Type", "application/msgpack")
				resp, _ := tcpClient.Do(req)
				if resp != nil {
					resp.Body.Close()
				}

				// Send to UDS endpoint
				req, _ = http.NewRequest("POST", "http://unix/v0.4/traces", bytes.NewReader(bts))
				req.Header.Set("Content-Type", "application/msgpack")
				resp, _ = udsClient.Do(req)
				if resp != nil {
					resp.Body.Close()
				}
			}
		}()
	}

	time.Sleep(100 * time.Millisecond)

	receiver.Stop()

	wg.Wait()
}

func TestReceiverRequestBodyLength(t *testing.T) {
	assert := assert.New(t)

	conf := newTestReceiverConfig()
	conf.MaxRequestBytes = 2
	receiver := newTestReceiverFromConfig(conf)
	go receiver.Start()

	defer receiver.Stop()

	url := fmt.Sprintf("http://%s:%d/v0.4/traces",
		conf.ReceiverHost, conf.ReceiverPort)

	// Before going further, make sure receiver is started
	// since it's running in another goroutine
	serverReady := false
	for i := 0; i < 100; i++ {
		var client http.Client

		body := bytes.NewBufferString("[]")
		req, err := http.NewRequest("POST", url, body)
		assert.NoError(err)

		resp, err := client.Do(req)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				serverReady = true
				break
			}
		}
		time.Sleep(10 * time.Millisecond)
	}

	assert.True(serverReady)
	testBody := func(expectedStatus int, bodyData string) {
		var client http.Client

		body := bytes.NewBufferString(bodyData)
		req, err := http.NewRequest("POST", url, body)
		assert.NoError(err)

		resp, err := client.Do(req)
		assert.NoError(err)
		assert.Equal(expectedStatus, resp.StatusCode)
		resp.Body.Close()
	}

	testBody(http.StatusOK, "[]")
	testBody(http.StatusRequestEntityTooLarge, " []")
}

func TestListenTCP(t *testing.T) {
	t.Run("measured", func(t *testing.T) {
		r := &HTTPReceiver{conf: &config.AgentConfig{ConnectionLimit: 0}}
		ln, err := r.listenTCP("127.0.0.1:0")
		require.NoError(t, err)
		defer ln.Close()
		_, ok := ln.(*measuredListener)
		assert.True(t, ok)
	})

	t.Run("limited", func(t *testing.T) {
		r := &HTTPReceiver{conf: &config.AgentConfig{ConnectionLimit: 10}}
		ln, err := r.listenTCP("127.0.0.1:0")
		require.NoError(t, err)
		defer ln.Close()
		_, ok := ln.(*rateLimitedListener)
		assert.True(t, ok)
	})
}

func TestNoDuplicatePatterns(t *testing.T) {
	handlerPatternsMap := make(map[string]int)
	for _, endpoint := range endpoints {
		handlerPatternsMap[endpoint.Pattern]++
		if handlerPatternsMap[endpoint.Pattern] > 1 {
			assert.Fail(t, fmt.Sprintf("duplicate handler pattern %v", endpoint.Pattern))
		}
	}
}

func TestTracesDecodeMakingHugeAllocation(t *testing.T) {
	r := newTestReceiverFromConfig(newTestReceiverConfig())
	r.Start()
	defer r.Stop()
	data := []byte{0x96, 0x97, 0xa4, 0x30, 0x30, 0x30, 0x30, 0xa6, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0xa6, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0xa6, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0xa6, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0xa6, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0xa6, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x96, 0x94, 0x9c, 0x00, 0x00, 0x00, 0x30, 0x30, 0xd1, 0x30, 0x30, 0x30, 0x30, 0x30, 0xdf, 0x30, 0x30, 0x30, 0x30}

	path := fmt.Sprintf("http://%s:%d/v0.5/traces", r.conf.ReceiverHost, r.conf.ReceiverPort)
	resp, err := http.Post(path, "application/msgpack", bytes.NewReader(data))
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestStateHeaders(t *testing.T) {
	assert := assert.New(t)
	cfg := newTestReceiverConfig()
	cfg.AgentVersion = "testVersion"
	url := fmt.Sprintf("http://%s:%d",
		cfg.ReceiverHost, cfg.ReceiverPort)
	r := newTestReceiverFromConfig(cfg)
	r.Start()
	defer r.Stop()
	data := msgpTraces(t, pb.Traces{
		testutil.RandomTrace(10, 20),
		testutil.RandomTrace(10, 20),
		testutil.RandomTrace(10, 20),
	})

	for _, e := range []string{
		"/v0.3/traces",
		"/v0.4/traces",
		// this one will return 500, but that's fine, we want to test that all
		// reponses have the header regardless of status code
		"/v0.5/traces",
		"/v0.7/traces",
	} {
		resp, err := http.Post(url+e, "application/msgpack", bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		_, ok := resp.Header["Datadog-Agent-Version"]
		assert.True(ok)
		v := resp.Header.Get("Datadog-Agent-Version")
		assert.Equal("testVersion", v)

		_, ok = resp.Header["Datadog-Agent-State"]
		assert.True(ok)
		v = resp.Header.Get("Datadog-Agent-State")
		assert.NotEmpty(v)
	}
}

func TestLegacyReceiver(t *testing.T) {
	// testing traces without content-type in agent endpoints, it should use JSON decoding
	assert := assert.New(t)
	conf := newTestReceiverConfig()
	testCases := []struct {
		name        string
		r           *HTTPReceiver
		apiVersion  Version
		contentType string
		traces      pb.Trace
	}{
		{"v01 with empty content-type", newTestReceiverFromConfig(conf), v01, "", pb.Trace{testutil.GetTestSpan()}},
		{"v01 with application/json", newTestReceiverFromConfig(conf), v01, "application/json", pb.Trace{testutil.GetTestSpan()}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// start testing server
			server := httptest.NewServer(
				tc.r.handleWithVersion(tc.apiVersion, tc.r.handleTraces),
			)

			// send traces to that endpoint without a content-type
			data, err := json.Marshal(tc.traces)
			assert.Nil(err)
			req, err := http.NewRequest("POST", server.URL, bytes.NewBuffer(data))
			assert.Nil(err)
			req.Header.Set("Content-Type", tc.contentType)

			var client http.Client
			resp, err := client.Do(req)
			assert.Nil(err)
			assert.Equal(200, resp.StatusCode)

			// now we should be able to read the trace data
			select {
			case p := <-tc.r.out:
				assert.Len(p.Chunks(), 1)
				rt := p.Chunk(0).Spans
				assert.Len(rt, 1)
				span := rt[0]
				assert.Equal(uint64(42), span.TraceID)
				assert.Equal(uint64(52), span.SpanID)
				assert.Equal("fennel_IS amazing!", span.Service)
				assert.Equal("something &&<@# that should be a metric!", span.Name)
				assert.Equal("NOT touched because it is going to be hashed", span.Resource)
				assert.Equal("192.168.0.1", span.Meta["http.host"])
				assert.Equal(41.99, span.Metrics["http.monitor"])
			case <-time.After(time.Second):
				t.Fatalf("no data received")
			}

			resp.Body.Close()
			server.Close()
		})
	}
}

func TestReceiverJSONDecoder(t *testing.T) {
	// testing traces without content-type in agent endpoints, it should use JSON decoding
	assert := assert.New(t)
	conf := newTestReceiverConfig()
	testCases := []struct {
		name        string
		r           *HTTPReceiver
		apiVersion  Version
		contentType string
		traces      []pb.Trace
	}{
		{"v02 with empty content-type", newTestReceiverFromConfig(conf), v02, "", testutil.GetTestTraces(1, 1, false)},
		{"v03 with empty content-type", newTestReceiverFromConfig(conf), v03, "", testutil.GetTestTraces(1, 1, false)},
		{"v04 with empty content-type", newTestReceiverFromConfig(conf), v04, "", testutil.GetTestTraces(1, 1, false)},
		{"v02 with application/json", newTestReceiverFromConfig(conf), v02, "application/json", testutil.GetTestTraces(1, 1, false)},
		{"v03 with application/json", newTestReceiverFromConfig(conf), v03, "application/json", testutil.GetTestTraces(1, 1, false)},
		{"v04 with application/json", newTestReceiverFromConfig(conf), v04, "application/json", testutil.GetTestTraces(1, 1, false)},
		{"v02 with text/json", newTestReceiverFromConfig(conf), v02, "text/json", testutil.GetTestTraces(1, 1, false)},
		{"v03 with text/json", newTestReceiverFromConfig(conf), v03, "text/json", testutil.GetTestTraces(1, 1, false)},
		{"v04 with text/json", newTestReceiverFromConfig(conf), v04, "text/json", testutil.GetTestTraces(1, 1, false)},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// start testing server
			server := httptest.NewServer(
				tc.r.handleWithVersion(tc.apiVersion, tc.r.handleTraces),
			)

			// send traces to that endpoint without a content-type
			data, err := json.Marshal(tc.traces)
			assert.Nil(err)
			req, err := http.NewRequest("POST", server.URL, bytes.NewBuffer(data))
			assert.Nil(err)
			req.Header.Set("Content-Type", tc.contentType)

			var client http.Client
			resp, err := client.Do(req)
			assert.Nil(err)
			assert.Equal(200, resp.StatusCode)

			// now we should be able to read the trace data
			select {
			case p := <-tc.r.out:
				rt := p.Chunk(0).Spans
				assert.Len(rt, 1)
				span := rt[0]
				assert.Equal(uint64(42), span.TraceID)
				assert.Equal(uint64(52), span.SpanID)
				assert.Equal("fennel_IS amazing!", span.Service)
				assert.Equal("something &&<@# that should be a metric!", span.Name)
				assert.Equal("NOT touched because it is going to be hashed", span.Resource)
				assert.Equal("192.168.0.1", span.Meta["http.host"])
				assert.Equal(41.99, span.Metrics["http.monitor"])
			case <-time.After(time.Second):
				t.Fatalf("no data received")
			}

			resp.Body.Close()
			server.Close()
		})
	}
}

func TestReceiverMsgpackDecoder(t *testing.T) {
	// testing traces without content-type in agent endpoints, it should use Msgpack decoding
	// or it should raise a 415 Unsupported media type
	assert := assert.New(t)
	conf := newTestReceiverConfig()
	testCases := []struct {
		name        string
		r           *HTTPReceiver
		apiVersion  Version
		contentType string
		traces      pb.Traces
	}{
		{"v01 with application/msgpack", newTestReceiverFromConfig(conf), v01, "application/msgpack", testutil.GetTestTraces(1, 1, false)},
		{"v02 with application/msgpack", newTestReceiverFromConfig(conf), v02, "application/msgpack", testutil.GetTestTraces(1, 1, false)},
		{"v03 with application/msgpack", newTestReceiverFromConfig(conf), v03, "application/msgpack", testutil.GetTestTraces(1, 1, false)},
		{"v04 with application/msgpack", newTestReceiverFromConfig(conf), v04, "application/msgpack", testutil.GetTestTraces(1, 1, false)},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// start testing server
			server := httptest.NewServer(
				tc.r.handleWithVersion(tc.apiVersion, tc.r.handleTraces),
			)

			// send traces to that endpoint using the msgpack content-type
			bts, err := tc.traces.MarshalMsg(nil)
			assert.Nil(err)
			req, err := http.NewRequest("POST", server.URL, bytes.NewReader(bts))
			assert.Nil(err)
			req.Header.Set("Content-Type", tc.contentType)

			var client http.Client
			resp, err := client.Do(req)
			assert.Nil(err)

			switch tc.apiVersion {
			case v01:
				assert.Equal(415, resp.StatusCode)
			case v02:
				assert.Equal(415, resp.StatusCode)
			case v03:
				assert.Equal(200, resp.StatusCode)

				// now we should be able to read the trace data
				select {
				case p := <-tc.r.out:
					rt := p.Chunk(0).Spans
					assert.Len(rt, 1)
					span := rt[0]
					assert.Equal(uint64(42), span.TraceID)
					assert.Equal(uint64(52), span.SpanID)
					assert.Equal("fennel_IS amazing!", span.Service)
					assert.Equal("something &&<@# that should be a metric!", span.Name)
					assert.Equal("NOT touched because it is going to be hashed", span.Resource)
					assert.Equal("192.168.0.1", span.Meta["http.host"])
					assert.Equal(41.99, span.Metrics["http.monitor"])
				case <-time.After(time.Second):
					t.Fatalf("no data received")
				}

				body, err := io.ReadAll(resp.Body)
				assert.Nil(err)
				assert.Equal("OK\n", string(body))
			case v04:
				assert.Equal(200, resp.StatusCode)

				// now we should be able to read the trace data
				select {
				case p := <-tc.r.out:
					rt := p.Chunk(0).Spans
					assert.Len(rt, 1)
					span := rt[0]
					assert.Equal(uint64(42), span.TraceID)
					assert.Equal(uint64(52), span.SpanID)
					assert.Equal("fennel_IS amazing!", span.Service)
					assert.Equal("something &&<@# that should be a metric!", span.Name)
					assert.Equal("NOT touched because it is going to be hashed", span.Resource)
					assert.Equal("192.168.0.1", span.Meta["http.host"])
					assert.Equal(41.99, span.Metrics["http.monitor"])
					assert.Equal(1, len(span.SpanLinks))
					assert.Equal(uint64(42), span.SpanLinks[0].TraceID)
					assert.Equal(uint64(32), span.SpanLinks[0].TraceIDHigh)
					assert.Equal(uint64(52), span.SpanLinks[0].SpanID)
					assert.Equal("v1", span.SpanLinks[0].Attributes["a1"])
					assert.Equal("v2", span.SpanLinks[0].Attributes["a2"])
					assert.Equal("dd=s:2;o:rum,congo=baz123", span.SpanLinks[0].Tracestate)
					assert.Equal(uint32(2147483649), span.SpanLinks[0].Flags)
				case <-time.After(time.Second):
					t.Fatalf("no data received")
				}

				body, err := io.ReadAll(resp.Body)
				assert.Nil(err)
				var tr traceResponse
				err = json.Unmarshal(body, &tr)
				assert.Nil(err, "the answer should be a valid JSON")
			}

			resp.Body.Close()
			server.Close()
		})
	}
}

func TestReceiverDecodingError(t *testing.T) {
	assert := assert.New(t)
	conf := newTestReceiverConfig()
	r := newTestReceiverFromConfig(conf)
	server := httptest.NewServer(r.handleWithVersion(v04, r.handleTraces))
	data := []byte("} invalid json")
	var client http.Client

	t.Run("no-header", func(_ *testing.T) {
		req, err := http.NewRequest("POST", server.URL, bytes.NewBuffer(data))
		assert.NoError(err)
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		assert.NoError(err)
		resp.Body.Close()
		assert.Equal(400, resp.StatusCode)
		assert.EqualValues(0, r.Stats.GetTagStats(info.Tags{EndpointVersion: "v0.4"}).TracesDropped.DecodingError.Load())
	})

	t.Run("with-header", func(_ *testing.T) {
		req, err := http.NewRequest("POST", server.URL, bytes.NewBuffer(data))
		assert.NoError(err)
		traceCount := 10
		req.Header.Set(header.TraceCount, strconv.Itoa(traceCount))
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		assert.NoError(err)
		resp.Body.Close()
		assert.Equal(400, resp.StatusCode)
		assert.EqualValues(traceCount, r.Stats.GetTagStats(info.Tags{EndpointVersion: "v0.4"}).TracesDropped.DecodingError.Load())
	})
}

func TestHandleWithVersionRejectCrossSite(t *testing.T) {
	assert := assert.New(t)
	conf := newTestReceiverConfig()
	r := newTestReceiverFromConfig(conf)
	server := httptest.NewServer(r.handleWithVersion(v04, r.handleTraces))

	var client http.Client
	req, err := http.NewRequest("POST", server.URL, nil)
	assert.NoError(err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Sec-Fetch-Site", "cross-site")

	resp, err := client.Do(req)
	assert.NoError(err)
	resp.Body.Close()
	assert.Equal(http.StatusForbidden, resp.StatusCode)
}

func TestReceiverUnexpectedEOF(t *testing.T) {
	assert := assert.New(t)
	conf := newTestReceiverConfig()
	r := newTestReceiverFromConfig(conf)
	server := httptest.NewServer(r.handleWithVersion(v05, r.handleTraces))
	var client http.Client
	traceCount := 2

	// we get to read the header and the entire dictionary, but the Content-Length claims
	// to be much larger
	data := []byte{
		0x92,                    // Short array with 2 elements
		0x91,                    // Short array with 1 element
		0xA5,                    // Short string with 5 elements
		'a', 'b', 'c', 'd', 'e', // bytes
	}
	req, err := http.NewRequest("POST", server.URL, bytes.NewBuffer(data))
	assert.NoError(err)
	req.Header.Set("Content-Type", "application/msgpack")
	req.Header.Set("Content-Length", "270")
	req.Header.Set(header.TraceCount, strconv.Itoa(traceCount))

	resp, err := client.Do(req)
	assert.NoError(err)

	resp.Body.Close()
	assert.Equal(400, resp.StatusCode)
	assert.EqualValues(traceCount, r.Stats.GetTagStats(info.Tags{EndpointVersion: "v0.5"}).TracesDropped.MSGPShortBytes.Load())
}

func TestTraceCount(t *testing.T) {
	req, err := http.NewRequest("GET", "/", nil)
	assert.NoError(t, err)

	t.Run("missing", func(t *testing.T) {
		for k := range req.Header {
			delete(req.Header, k)
		}
		_, err := traceCount(req)
		assert.EqualError(t, err, fmt.Sprintf("HTTP header %q not found", header.TraceCount))
	})

	t.Run("value-empty", func(t *testing.T) {
		req.Header.Set(header.TraceCount, "")
		_, err := traceCount(req)
		assert.EqualError(t, err, fmt.Sprintf("HTTP header %q not found", header.TraceCount))
	})

	t.Run("value-bad", func(t *testing.T) {
		req.Header.Set(header.TraceCount, "qwe")
		_, err := traceCount(req)
		assert.Equal(t, err, errInvalidHeaderTraceCountValue)
	})

	t.Run("ok", func(t *testing.T) {
		req.Header.Set(header.TraceCount, "123")
		count, err := traceCount(req)
		assert.NoError(t, err)
		assert.Equal(t, count, int64(123))
	})
}

func TestDecodeV05(t *testing.T) {
	assert := assert.New(t)
	data := [2]interface{}{
		0: []string{
			0:  "Service2",
			1:  "Name2",
			2:  "Resource",
			3:  "Service",
			4:  "Name",
			5:  "A",
			6:  "B",
			7:  "X",
			8:  "y",
			9:  "sql",
			10: "Resource2",
			11: "c",
			12: "d",
		},
		1: [][][12]interface{}{
			{
				{uint32(3), uint32(4), uint32(2), uint64(1), uint64(2), uint64(3), int64(123), int64(456), 1, map[uint32]uint32{5: 6}, map[uint32]float64{7: 1.2}, uint32(9)},
				{uint32(0), uint32(1), uint32(10), uint64(2), uint64(3), uint64(3), int64(789), int64(456), 0, map[uint32]uint32{11: 12}, map[uint32]float64{8: 1.4}, uint32(9)},
				{uint32(0), uint32(1), uint32(10), uint64(2), uint64(3), uint64(3), int64(789), int64(456), 0, map[uint32]uint32{11: 12}, map[uint32]float64{}, uint32(9)},
			},
		},
	}
	b, err := vmsgp.Marshal(&data)
	assert.NoError(err)
	req, err := http.NewRequest("POST", "/v0.5/traces", bytes.NewReader(b))
	assert.NoError(err)
	req.Header.Set(header.ContainerID, "abcdef123789456")
	tp, err := decodeTracerPayload(v05, req, NewIDProvider("", func(_ origindetection.OriginInfo) (string, error) {
		return "abcdef123789456", nil
	}), "python", "3.8.1", "1.2.3")
	assert.NoError(err)
	assert.EqualValues(tp, &pb.TracerPayload{
		ContainerID:     "abcdef123789456",
		LanguageName:    "python",
		LanguageVersion: "3.8.1",
		TracerVersion:   "1.2.3",
		Chunks: []*pb.TraceChunk{
			{
				Tags:     make(map[string]string),
				Priority: int32(sampler.PriorityNone),
				Spans: []*pb.Span{
					{
						Service:  "Service",
						Name:     "Name",
						Resource: "Resource",
						TraceID:  1,
						SpanID:   2,
						ParentID: 3,
						Start:    123,
						Duration: 456,
						Error:    1,
						Meta:     map[string]string{"A": "B"},
						Metrics:  map[string]float64{"X": 1.2},
						Type:     "sql",
					},
					{
						Service:  "Service2",
						Name:     "Name2",
						Resource: "Resource2",
						TraceID:  2,
						SpanID:   3,
						ParentID: 3,
						Start:    789,
						Duration: 456,
						Error:    0,
						Meta:     map[string]string{"c": "d"},
						Metrics:  map[string]float64{"y": 1.4},
						Type:     "sql",
					},
					{
						Service:  "Service2",
						Name:     "Name2",
						Resource: "Resource2",
						TraceID:  2,
						SpanID:   3,
						ParentID: 3,
						Start:    789,
						Duration: 456,
						Error:    0,
						Meta:     map[string]string{"c": "d"},
						Metrics:  nil,
						Type:     "sql",
					},
				},
			},
		},
	})
}

type mockStatsProcessor struct {
	mu                sync.RWMutex
	lastP             *pb.ClientStatsPayload
	lastLang          string
	lastTracerVersion string
	containerID       string
	obfVersion        string
}

func (m *mockStatsProcessor) ProcessStats(p *pb.ClientStatsPayload, lang, tracerVersion, containerID, obfVersion string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lastP = p
	m.lastLang = lang
	m.lastTracerVersion = tracerVersion
	m.containerID = containerID
	m.obfVersion = obfVersion
}

func (m *mockStatsProcessor) Got() (p *pb.ClientStatsPayload, lang, tracerVersion, containerID string) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.lastP, m.lastLang, m.lastTracerVersion, m.containerID
}

func TestHandleStats(t *testing.T) {
	p := testutil.StatsPayloadSample()
	t.Run("on", func(t *testing.T) {
		cfg := newTestReceiverConfig()
		rcv := newTestReceiverFromConfig(cfg)
		mockProcessor := new(mockStatsProcessor)
		rcv.statsProcessor = mockProcessor
		mux := rcv.buildMux()
		server := httptest.NewServer(mux)

		var buf bytes.Buffer
		if err := msgp.Encode(&buf, p); err != nil {
			t.Fatal(err)
		}
		req, _ := http.NewRequest("POST", server.URL+"/v0.6/stats", &buf)
		req.Header.Set("Content-Type", "application/msgpack")
		req.Header.Set(header.Lang, "lang1")
		req.Header.Set(header.TracerVersion, "0.1.0")
		req.Header.Set(header.ContainerID, "abcdef123789456")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != 200 {
			slurp, _ := io.ReadAll(resp.Body)
			t.Fatal(string(slurp), resp.StatusCode)
		}

		resp.Body.Close()
		gotp, gotlang, gotTracerVersion, containerID := mockProcessor.Got()
		assert.True(t, reflect.DeepEqual(gotp, p), "payload did not match")
		assert.Equal(t, "lang1", gotlang, "lang did not match")
		assert.Equal(t, "0.1.0", gotTracerVersion, "tracerVersion did not match")
		assert.Equal(t, "abcdef123789456", containerID, "containerID did not match")

		_, ok := rcv.Stats.Stats[info.Tags{Lang: "lang1", EndpointVersion: "v0.6", Service: "service", TracerVersion: "0.1.0"}]
		assert.True(t, ok)
	})
}

func TestClientComputedStatsHeader(t *testing.T) {
	conf := newTestReceiverConfig()
	rcv := newTestReceiverFromConfig(conf)
	mux := rcv.buildMux()
	server := httptest.NewServer(mux)

	// run runs the test with ClientComputedStats turned on.
	run := func(on bool) func(t *testing.T) {
		return func(t *testing.T) {
			bts, err := testutil.GetTestTraces(10, 10, true).MarshalMsg(nil)
			assert.Nil(t, err)

			req, _ := http.NewRequest("POST", server.URL+"/v0.4/traces", bytes.NewReader(bts))
			req.Header.Set("Content-Type", "application/msgpack")
			req.Header.Set(header.Lang, "lang1")
			if on {
				req.Header.Set(header.ComputedStats, "yes")
			} else {
				req.Header.Set(header.ComputedStats, "false")
			}
			var wg sync.WaitGroup
			wg.Add(1)
			go func() {
				defer wg.Done()
				resp, err := http.DefaultClient.Do(req)
				if err != nil {
					t.Error(err)
					return
				}
				resp.Body.Close()
				if resp.StatusCode != 200 {
					t.Error(resp.StatusCode)
					return
				}
			}()
			timeout := time.After(time.Second)
			for {
				select {
				case p := <-rcv.out:
					assert.Equal(t, p.ClientComputedStats, on)
					wg.Wait()
					return
				case <-timeout:
					t.Fatal("no output")
				}
			}
		}
	}

	t.Run("on", run(true))
	t.Run("off", run(false))
}

func TestHandleTraces(t *testing.T) {
	// prepare the msgpack payload
	bts, err := testutil.GetTestTraces(10, 10, true).MarshalMsg(nil)
	assert.Nil(t, err)

	// prepare the receiver
	conf := newTestReceiverConfig()
	receiver := newTestReceiverFromConfig(conf)

	// response recorder
	handler := receiver.handleWithVersion(v04, receiver.handleTraces)

	for n := 0; n < 10; n++ {
		// consume the traces channel without doing anything
		select {
		case <-receiver.out:
		default:
		}

		// forge the request
		rr := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v0.4/traces", bytes.NewReader(bts))
		req.Header.Set("Content-Type", "application/msgpack")

		// Add meta data to simulate data coming from multiple applications
		req.Header.Set("Datadog-Meta-Lang", langs[n%len(langs)])

		handler.ServeHTTP(rr, req)
	}

	rs := receiver.Stats
	assert.Equal(t, 5, len(rs.Stats)) // We have a tagStats struct for each application

	// We test stats for each app
	for _, lang := range langs {
		ts, ok := rs.Stats[info.Tags{Lang: lang, EndpointVersion: "v0.4", Service: "fennel_IS amazing!"}]
		assert.True(t, ok)
		assert.Equal(t, int64(20), ts.TracesReceived.Load())
		assert.Equal(t, int64(83022), ts.TracesBytes.Load())
	}
	// make sure we have all our languages registered
	assert.Equal(t, "C#|go|java|python|ruby", receiver.Languages())

	t.Run("overwhelmed", func(t *testing.T) {
		// prepare the msgpack payload
		bts, err := testutil.GetTestTraces(10, 10, true).MarshalMsg(nil)
		assert.Nil(t, err)

		// prepare the receiver
		conf := newTestReceiverConfig()
		conf.DecoderTimeout = 1
		conf.Decoders = 1
		dynConf := sampler.NewDynamicConfig()

		rawTraceChan := make(chan *Payload)
		receiver := NewHTTPReceiver(conf, dynConf, rawTraceChan, noopStatsProcessor{}, telemetry.NewNoopCollector(), &statsd.NoOpClient{}, &timing.NoopReporter{})
		receiver.recvsem = make(chan struct{}) //overwrite recvsem to ALWAYS block and ensure we look overwhelmed
		// response recorder
		handler := receiver.handleWithVersion(v04, receiver.handleTraces)
		rr := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v0.4/traces", bytes.NewReader(bts))
		req.Header.Set("Content-Type", "application/msgpack")
		req.Header.Set("Datadog-Send-Real-Http-Status", "true")
		handler.ServeHTTP(rr, req)
		result := rr.Result()
		defer result.Body.Close()
		assert.Equal(t, http.StatusTooManyRequests, result.StatusCode)
		assert.Equal(t, "application/json", result.Header.Get("Content-Type"))
	})
}

func TestClientComputedTopLevel(t *testing.T) {
	conf := newTestReceiverConfig()
	rcv := newTestReceiverFromConfig(conf)
	mux := rcv.buildMux()
	server := httptest.NewServer(mux)

	// run runs the test with ClientComputedStats turned on.
	run := func(on bool) func(t *testing.T) {
		return func(t *testing.T) {
			bts, err := testutil.GetTestTraces(10, 10, true).MarshalMsg(nil)
			assert.Nil(t, err)

			req, _ := http.NewRequest("POST", server.URL+"/v0.4/traces", bytes.NewReader(bts))
			req.Header.Set("Content-Type", "application/msgpack")
			req.Header.Set(header.Lang, "lang1")
			if on {
				req.Header.Set(header.ComputedTopLevel, "yes")
			}
			var wg sync.WaitGroup
			wg.Add(1)
			go func() {
				defer wg.Done()
				resp, err := http.DefaultClient.Do(req)
				if err != nil {
					t.Error(err)
					return
				}
				resp.Body.Close()
				if resp.StatusCode != 200 {
					t.Error(resp.StatusCode)
					return
				}
			}()
			timeout := time.After(time.Second)
			for {
				select {
				case p := <-rcv.out:
					assert.Equal(t, p.ClientComputedTopLevel, on)
					wg.Wait()
					return
				case <-timeout:
					t.Fatal("no output")
				}
			}
		}
	}

	t.Run("on", run(true))
	t.Run("off", run(false))
}

func TestClientDropP0s(t *testing.T) {
	conf := newTestReceiverConfig()
	rcv := newTestReceiverFromConfig(conf)
	mux := rcv.buildMux()
	server := httptest.NewServer(mux)

	bts, err := testutil.GetTestTraces(10, 10, true).MarshalMsg(nil)
	assert.Nil(t, err)

	req, _ := http.NewRequest("POST", server.URL+"/v0.4/traces", bytes.NewReader(bts))
	req.Header.Set("Content-Type", "application/msgpack")
	req.Header.Set(header.Lang, "lang1")
	req.Header.Set(header.DroppedP0Traces, "153")
	req.Header.Set(header.DroppedP0Spans, "2331")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatal(resp.StatusCode)
	}
	p := <-rcv.out
	assert.Equal(t, p.ClientDroppedP0s, int64(153))
}

func BenchmarkHandleTracesFromOneApp(b *testing.B) {
	assert := assert.New(b)
	// prepare the payload
	// msgpack payload
	bts, err := testutil.GetTestTraces(1, 1, true).MarshalMsg(nil)
	assert.Nil(err)

	// prepare the receiver
	conf := newTestReceiverConfig()
	receiver := newTestReceiverFromConfig(conf)

	// response recorder
	handler := receiver.handleWithVersion(v04, receiver.handleTraces)

	// benchmark
	b.ResetTimer()
	b.ReportAllocs()
	for n := 0; n < b.N; n++ {
		b.StopTimer()
		// consume the traces channel without doing anything
		select {
		case <-receiver.out:
		default:
		}

		// forge the request
		rr := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v0.4/traces", bytes.NewReader(bts))
		req.Header.Set("Content-Type", "application/msgpack")

		// Add meta data to simulate data coming from multiple applications
		for _, v := range headerFields {
			req.Header.Set(v, langs[n%len(langs)])
		}

		// trace only this execution
		b.StartTimer()
		handler.ServeHTTP(rr, req)
	}
}

func BenchmarkHandleTracesFromMultipleApps(b *testing.B) {
	assert := assert.New(b)
	// prepare the payload
	// msgpack payload
	bts, err := testutil.GetTestTraces(1, 1, true).MarshalMsg(nil)
	assert.Nil(err)

	// prepare the receiver
	conf := newTestReceiverConfig()
	receiver := newTestReceiverFromConfig(conf)

	// response recorder
	handler := receiver.handleWithVersion(v04, receiver.handleTraces)

	// benchmark
	b.ResetTimer()
	b.ReportAllocs()
	for n := 0; n < b.N; n++ {
		b.StopTimer()
		// consume the traces channel without doing anything
		select {
		case <-receiver.out:
		default:
		}

		// forge the request
		rr := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v0.4/traces", bytes.NewReader(bts))
		req.Header.Set("Content-Type", "application/msgpack")

		// Add meta data to simulate data coming from multiple applications
		for _, v := range headerFields {
			req.Header.Set(v, langs[n%len(langs)])
		}

		// trace only this execution
		b.StartTimer()
		handler.ServeHTTP(rr, req)
	}
}

func BenchmarkDecoderJSON(b *testing.B) {
	assert := assert.New(b)
	traces := testutil.GetTestTraces(150, 66, true)

	// json payload
	payload, err := json.Marshal(traces)
	assert.Nil(err)

	// benchmark
	b.ResetTimer()
	b.ReportAllocs()
	for n := 0; n < b.N; n++ {
		b.StopTimer()
		reader := bytes.NewReader(payload)

		b.StartTimer()
		var spans pb.Traces
		decoder := json.NewDecoder(reader)
		_ = decoder.Decode(&spans)
	}
}

func BenchmarkDecoderMsgpack(b *testing.B) {
	assert := assert.New(b)

	// msgpack payload
	bts, err := testutil.GetTestTraces(150, 66, true).MarshalMsg(nil)
	assert.Nil(err)
	bufferPool := &sync.Pool{
		New: func() interface{} {
			return new(bytes.Buffer)
		},
	}

	// benchmark
	b.ResetTimer()
	b.ReportAllocs()
	for n := 0; n < b.N; n++ {
		var traces pb.Traces
		buffer := bufferPool.Get().(*bytes.Buffer)
		buffer.Reset()
		_, _ = io.Copy(buffer, bytes.NewReader(bts))
		_, _ = traces.UnmarshalMsg(buffer.Bytes())
		bufferPool.Put(buffer)
	}
}

func BenchmarkWatchdog(b *testing.B) {
	now := time.Now()
	conf := newTestReceiverConfig()
	conf.Endpoints[0].APIKey = "apikey_2"
	r := NewHTTPReceiver(conf, nil, nil, nil, telemetry.NewNoopCollector(), &statsd.NoOpClient{}, &timing.NoopReporter{})

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		r.watchdog(now)
	}
}

func TestReplyOKV5(t *testing.T) {
	r := newTestReceiverFromConfig(newTestReceiverConfig())
	r.Start()
	defer r.Stop()

	data, err := vmsgp.Marshal([2][]interface{}{{}, {}})
	assert.NoError(t, err)
	path := fmt.Sprintf("http://%s:%d/v0.5/traces", r.conf.ReceiverHost, r.conf.ReceiverPort)
	resp, err := http.Post(path, "application/msgpack", bytes.NewReader(data))
	assert.NoError(t, err)
	slurp, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	assert.Contains(t, string(slurp), `"rate_by_service"`)
}

func TestExpvar(t *testing.T) {
	if testing.Short() {
		return
	}

	c := newTestReceiverConfig()
	c.DebugServerPort = 6789
	assert.NoError(t, info.InitInfo(c))

	// Starting a TLS httptest server to retrieve tlsCert
	ts := httptest.NewTLSServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {}))
	tlsConfig := ts.TLS.Clone()
	// Setting a client with the proper TLS configuration
	client := ts.Client()
	ts.Close()

	// Starting Debug Server
	s := NewDebugServer(c)
	s.SetTLSConfig(tlsConfig)

	// Starting the Debug server
	s.Start()
	defer s.Stop()

	resp, err := client.Get(fmt.Sprintf("https://127.0.0.1:%d/debug/vars", c.DebugServerPort))
	require.NoError(t, err)
	defer resp.Body.Close()

	t.Run("read-expvars", func(t *testing.T) {
		assert.EqualValues(t, resp.StatusCode, http.StatusOK, "failed to read expvars from local server")
	})
	t.Run("valid-response", func(t *testing.T) {
		if resp.StatusCode == http.StatusOK {
			var out map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&out)
			assert.NoError(t, err, "/debug/vars must return valid json")
			assert.NotNil(t, out["receiver"], "expvar receiver must not be nil")
		}
	})
}

func TestWithoutIPCCert(t *testing.T) {
	c := newTestReceiverConfig()

	// Getting an available port
	a, err := net.ResolveTCPAddr("tcp", "localhost:0")
	require.NoError(t, err)

	var l *net.TCPListener
	l, err = net.ListenTCP("tcp", a)
	require.NoError(t, err)

	availablePort := l.Addr().(*net.TCPAddr).Port
	require.NoError(t, l.Close())
	require.NotZero(t, availablePort)

	c.DebugServerPort = availablePort
	info.InitInfo(c)

	// Starting Debug Server
	s := NewDebugServer(c)

	// Starting the Debug server
	s.Start()
	defer s.Stop()

	// Server should not be able to connect because it didn't start
	conn, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(c.DebugServerPort)), time.Second)
	require.Error(t, err)
	if conn != nil {
		conn.Close()
	}
}

func TestNormalizeHTTPHeader(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{
			input:    "Header: value\nAnother header: value",
			expected: "Header: value_Another header: value",
		},
		{
			input:    "Header: value\rAnother header: value",
			expected: "Header: value_Another header: value",
		},
		{
			input:    "Header: value\r\nAnother header: value",
			expected: "Header: value_Another header: value",
		},
		{
			input:    "SingleLineHeader: value",
			expected: "SingleLineHeader: value",
		},
		{
			input:    "\rLeading carriage return",
			expected: "_Leading carriage return",
		},
		{
			input:    "\nLeading line break",
			expected: "_Leading line break",
		},
		{
			input:    "Trailing carriage return\r",
			expected: "Trailing carriage return_",
		},
		{
			input:    "Trailing line break\n",
			expected: "Trailing line break_",
		},
		{
			input:    "Multiple\r\nline\r\nbreaks",
			expected: "Multiple_line_breaks",
		},
		{
			input:    "",
			expected: "",
		},
	}

	for _, test := range tests {
		result := normalizeHTTPHeader(test.input)
		if result != test.expected {
			t.Errorf("normalizeHTTPHeader(%q) = %q; expected %q", test.input, result, test.expected)
		}
	}
}

func TestGetProcessTags(t *testing.T) {
	tests := []struct {
		name     string
		header   http.Header
		payload  *pb.TracerPayload
		expected string
	}{
		{
			name: "process tags in payload tags",
			header: http.Header{
				header.ProcessTags: []string{"header-value"},
			},
			payload: &pb.TracerPayload{
				Tags: map[string]string{
					tagProcessTags: "payload-tag-value",
				},
			},
			expected: "payload-tag-value",
		},
		{
			name:   "process tags in first span meta",
			header: http.Header{header.ProcessTags: []string{"header-value"}},
			payload: &pb.TracerPayload{
				Chunks: []*pb.TraceChunk{
					{
						Spans: []*pb.Span{
							{
								Meta: map[string]string{
									tagProcessTags: "span-meta-value",
								},
							},
						},
					},
				},
			},
			expected: "span-meta-value",
		},
		{
			name: "process tags in header only",
			header: http.Header{
				header.ProcessTags: []string{"header-value"},
			},
			payload:  &pb.TracerPayload{},
			expected: "header-value",
		},
		{
			name:     "no tags anywhere",
			header:   http.Header{},
			payload:  &pb.TracerPayload{},
			expected: "",
		},
		{
			name:   "chunks but no spans",
			header: http.Header{header.ProcessTags: []string{"header-value"}},
			payload: &pb.TracerPayload{
				Chunks: []*pb.TraceChunk{
					nil,
					{},
					{Spans: []*pb.Span{nil}},
				},
			},
			expected: "header-value",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := getProcessTags(tc.header, tc.payload)
			if result != tc.expected {
				t.Errorf("expected %q, got %q", tc.expected, result)
			}
		})
	}
}

func TestUpdateAPIKey(t *testing.T) {
	assert := assert.New(t)

	var counter int // keeps track of every time the buildHandler function has been called
	buildHandler := func(*HTTPReceiver) http.Handler {
		counter++
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			fmt.Fprintf(w, "%d", counter)
		})
	}

	testEndpoint := Endpoint{
		Pattern: "/test",
		Handler: buildHandler,
	}
	AttachEndpoint(testEndpoint)

	conf := newTestReceiverConfig()
	receiver := newTestReceiverFromConfig(conf)
	receiver.Start()
	defer receiver.Stop()

	assert.Equal(1, counter)

	url := fmt.Sprintf("http://%s:%d/test",
		conf.ReceiverHost, conf.ReceiverPort)

	for i := 1; i <= 10; i++ {
		receiver.UpdateAPIKey() // force handler rebuild

		req, err := http.NewRequest("GET", url, nil)
		assert.NoError(err)

		resp, err := http.DefaultClient.Do(req)
		assert.NoError(err)
		defer resp.Body.Close()

		assert.Equal(200, resp.StatusCode)

		body, err := io.ReadAll(resp.Body)
		assert.NoError(err)

		number, err := strconv.Atoi(string(body))
		assert.NoError(err)

		assert.Equal(counter, number)
	}
}

func msgpTraces(t *testing.T, traces pb.Traces) []byte {
	bts, err := traces.MarshalMsg(nil)
	if err != nil {
		t.Fatal(err)
	}
	return bts
}
