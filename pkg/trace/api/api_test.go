// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strconv"
	"sync"
	"testing"
	"time"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/trace/api/internal/header"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/info"
	"github.com/DataDog/datadog-agent/pkg/trace/sampler"
	"github.com/DataDog/datadog-agent/pkg/trace/telemetry"
	"github.com/DataDog/datadog-agent/pkg/trace/testutil"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tinylib/msgp/msgp"
	vmsgp "github.com/vmihailenco/msgpack/v4"
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

func (noopStatsProcessor) ProcessStats(_ *pb.ClientStatsPayload, _, _ string) {}

func newTestReceiverFromConfig(conf *config.AgentConfig) *HTTPReceiver {
	dynConf := sampler.NewDynamicConfig()

	rawTraceChan := make(chan *Payload, 5000)
	receiver := NewHTTPReceiver(conf, dynConf, rawTraceChan, noopStatsProcessor{}, telemetry.NewNoopCollector())

	return receiver
}

func newTestReceiverConfig() *config.AgentConfig {
	conf := config.New()
	conf.Endpoints[0].APIKey = "test"
	conf.DecoderTimeout = 10000

	return conf
}

func TestMain(m *testing.M) {
	defer func(old func(string, ...interface{})) { killProcess = old }(killProcess)
	killProcess = func(format string, args ...interface{}) {
		fmt.Printf(format, args...)
		fmt.Println()
	}
	os.Exit(m.Run())
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
		ln, err := r.listenTCP(":0")
		require.NoError(t, err)
		defer ln.Close()
		_, ok := ln.(*measuredListener)
		assert.True(t, ok)
	})

	t.Run("limited", func(t *testing.T) {
		r := &HTTPReceiver{conf: &config.AgentConfig{ConnectionLimit: 10}}
		ln, err := r.listenTCP(":0")
		require.NoError(t, err)
		defer ln.Close()
		_, ok := ln.(*rateLimitedListener)
		assert.True(t, ok)
	})
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
		resp, err := http.Post("http://localhost:8126"+e, "application/msgpack", bytes.NewReader(data))
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
		t.Run(fmt.Sprintf(tc.name), func(t *testing.T) {
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
		t.Run(fmt.Sprintf(tc.name), func(t *testing.T) {
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
		t.Run(fmt.Sprintf(tc.name), func(t *testing.T) {
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

	t.Run("no-header", func(t *testing.T) {
		req, err := http.NewRequest("POST", server.URL, bytes.NewBuffer(data))
		assert.NoError(err)
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		assert.NoError(err)
		resp.Body.Close()
		assert.Equal(400, resp.StatusCode)
		assert.EqualValues(0, r.Stats.GetTagStats(info.Tags{EndpointVersion: "v0.4"}).TracesDropped.DecodingError.Load())
	})

	t.Run("with-header", func(t *testing.T) {
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
	assert.EqualValues(traceCount, r.Stats.GetTagStats(info.Tags{EndpointVersion: "v0.5"}).TracesDropped.EOF.Load())
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
	tp, _, err := decodeTracerPayload(v05, req, &info.TagStats{
		Tags: info.Tags{
			Lang:          "python",
			LangVersion:   "3.8.1",
			TracerVersion: "1.2.3",
		},
	}, NewIDProvider(""))
	assert.NoError(err)
	assert.EqualValues(tp, &pb.TracerPayload{
		ContainerID:     "abcdef123789456",
		LanguageName:    "python",
		LanguageVersion: "3.8.1",
		TracerVersion:   "1.2.3",
		Chunks: []*pb.TraceChunk{
			{
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
}

func (m *mockStatsProcessor) ProcessStats(p *pb.ClientStatsPayload, lang, tracerVersion string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lastP = p
	m.lastLang = lang
	m.lastTracerVersion = tracerVersion
}

func (m *mockStatsProcessor) Got() (p *pb.ClientStatsPayload, lang, tracerVersion string) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.lastP, m.lastLang, m.lastTracerVersion
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
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != 200 {
			slurp, _ := io.ReadAll(resp.Body)
			t.Fatal(string(slurp), resp.StatusCode)
		}

		resp.Body.Close()
		gotp, gotlang, gotTracerVersion := mockProcessor.Got()
		if !reflect.DeepEqual(gotp, p) || gotlang != "lang1" || gotTracerVersion != "0.1.0" {
			t.Fatalf("Did not match payload: %v: %v", gotlang, gotp)
		}
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
	assert := assert.New(t)

	// prepare the msgpack payload
	bts, err := testutil.GetTestTraces(10, 10, true).MarshalMsg(nil)
	assert.Nil(err)

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
	assert.Equal(5, len(rs.Stats)) // We have a tagStats struct for each application

	// We test stats for each app
	for _, lang := range langs {
		ts, ok := rs.Stats[info.Tags{Lang: lang, EndpointVersion: "v0.4"}]
		assert.True(ok)
		assert.Equal(int64(20), ts.TracesReceived.Load())
		assert.Equal(int64(59222), ts.TracesBytes.Load())
	}
	// make sure we have all our languages registered
	assert.Equal("C#|go|java|python|ruby", receiver.Languages())
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
	r := NewHTTPReceiver(conf, nil, nil, nil, telemetry.NewNoopCollector())

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
	c.DebugServerPort = 5012
	info.InitInfo(c)
	s := NewDebugServer(c)
	s.Start()
	defer s.Stop()

	resp, err := http.Get("http://127.0.0.1:5012/debug/vars")
	assert.NoError(t, err)
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

func msgpTraces(t *testing.T, traces pb.Traces) []byte {
	bts, err := traces.MarshalMsg(nil)
	if err != nil {
		t.Fatal(err)
	}
	return bts
}
