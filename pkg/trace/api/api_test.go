// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/info"
	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/DataDog/datadog-agent/pkg/trace/sampler"
	"github.com/DataDog/datadog-agent/pkg/trace/test/testutil"

	"github.com/cihub/seelog"
	"github.com/stretchr/testify/assert"
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

func newTestReceiverFromConfig(conf *config.AgentConfig) *HTTPReceiver {
	dynConf := sampler.NewDynamicConfig("none")

	rawTraceChan := make(chan *Payload, 5000)
	receiver := NewHTTPReceiver(conf, dynConf, rawTraceChan)

	return receiver
}

func newTestReceiverConfig() *config.AgentConfig {
	conf := config.New()
	conf.Endpoints[0].APIKey = "test"

	return conf
}

func TestMain(m *testing.M) {
	seelog.UseLogger(seelog.Disabled)

	defer func(old func(string, ...interface{})) { killProcess = old }(killProcess)
	killProcess = func(_ string, _ ...interface{}) {}

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
	for i := 0; i < 10; i++ {
		var client http.Client

		body := bytes.NewBufferString("[]")
		req, err := http.NewRequest("POST", url, body)
		assert.Nil(err)

		resp, err := client.Do(req)
		if err == nil && resp.StatusCode == http.StatusOK {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	testBody := func(expectedStatus int, bodyData string) {
		var client http.Client

		body := bytes.NewBufferString(bodyData)
		req, err := http.NewRequest("POST", url, body)
		assert.Nil(err)

		resp, err := client.Do(req)
		assert.Nil(err)
		assert.Equal(expectedStatus, resp.StatusCode)
	}

	testBody(http.StatusOK, "[]")
	testBody(http.StatusRequestEntityTooLarge, " []")
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
				http.HandlerFunc(tc.r.handleWithVersion(tc.apiVersion, tc.r.handleTraces)),
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
				assert.Len(p.Traces, 1)
				rt := p.Traces[0]
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
				http.HandlerFunc(tc.r.handleWithVersion(tc.apiVersion, tc.r.handleTraces)),
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
				rt := p.Traces[0]
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
				http.HandlerFunc(tc.r.handleWithVersion(tc.apiVersion, tc.r.handleTraces)),
			)

			// send traces to that endpoint using the msgpack content-type
			var buf bytes.Buffer
			err := msgp.Encode(&buf, tc.traces)
			assert.Nil(err)
			req, err := http.NewRequest("POST", server.URL, &buf)
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
					rt := p.Traces[0]
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

				body, err := ioutil.ReadAll(resp.Body)
				assert.Nil(err)
				assert.Equal("OK\n", string(body))
			case v04:
				assert.Equal(200, resp.StatusCode)

				// now we should be able to read the trace data
				select {
				case p := <-tc.r.out:
					rt := p.Traces[0]
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

				body, err := ioutil.ReadAll(resp.Body)
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
	server := httptest.NewServer(http.HandlerFunc(r.handleWithVersion(v04, r.handleTraces)))
	data := []byte("} invalid json")
	var client http.Client

	t.Run("no-header", func(t *testing.T) {
		req, err := http.NewRequest("POST", server.URL, bytes.NewBuffer(data))
		assert.NoError(err)
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		assert.NoError(err)
		assert.Equal(400, resp.StatusCode)
		assert.EqualValues(0, r.Stats.GetTagStats(info.Tags{EndpointVersion: "v0.4"}).TracesDropped.DecodingError)
	})

	t.Run("with-header", func(t *testing.T) {
		req, err := http.NewRequest("POST", server.URL, bytes.NewBuffer(data))
		assert.NoError(err)
		traceCount := 10
		req.Header.Set(headerTraceCount, strconv.Itoa(traceCount))
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		assert.NoError(err)
		assert.Equal(400, resp.StatusCode)
		assert.EqualValues(traceCount, r.Stats.GetTagStats(info.Tags{EndpointVersion: "v0.4"}).TracesDropped.DecodingError)
	})
}

func TestReceiverUnexpectedEOF(t *testing.T) {
	assert := assert.New(t)
	conf := newTestReceiverConfig()
	r := newTestReceiverFromConfig(conf)
	server := httptest.NewServer(http.HandlerFunc(r.handleWithVersion(v05, r.handleTraces)))
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
	req.Header.Set(headerTraceCount, strconv.Itoa(traceCount))

	resp, err := client.Do(req)
	assert.NoError(err)

	assert.Equal(400, resp.StatusCode)
	assert.EqualValues(traceCount, r.Stats.GetTagStats(info.Tags{EndpointVersion: "v0.5"}).TracesDropped.EOF)
}

func TestTraceCount(t *testing.T) {
	req, err := http.NewRequest("GET", "/", nil)
	assert.NoError(t, err)

	t.Run("missing", func(t *testing.T) {
		for k := range req.Header {
			delete(req.Header, k)
		}
		_, err := traceCount(req)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("value-empty", func(t *testing.T) {
		req.Header.Set(headerTraceCount, "")
		_, err := traceCount(req)
		assert.Contains(t, err.Error(), "value not set")
	})

	t.Run("value-bad", func(t *testing.T) {
		req.Header.Set(headerTraceCount, "qwe")
		_, err := traceCount(req)
		assert.Contains(t, err.Error(), "can not be parsed")
	})

	t.Run("ok", func(t *testing.T) {
		req.Header.Set(headerTraceCount, "123")
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
	traces, err := decodeTraces(v05, req)
	assert.NoError(err)
	assert.EqualValues(traces, pb.Traces{
		{
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
	})
}

func TestHandleTraces(t *testing.T) {
	assert := assert.New(t)

	// prepare the msgpack payload
	var buf bytes.Buffer
	msgp.Encode(&buf, testutil.GetTestTraces(10, 10, true))

	// prepare the receiver
	conf := newTestReceiverConfig()
	receiver := newTestReceiverFromConfig(conf)

	// response recorder
	handler := http.HandlerFunc(receiver.handleWithVersion(v04, receiver.handleTraces))

	for n := 0; n < 10; n++ {
		// consume the traces channel without doing anything
		select {
		case <-receiver.out:
		default:
		}

		// forge the request
		rr := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v0.4/traces", bytes.NewReader(buf.Bytes()))
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
		assert.Equal(int64(20), ts.TracesReceived)
		assert.Equal(int64(59222), ts.TracesBytes)
	}
	// make sure we have all our languages registered
	assert.Equal("C#|go|java|python|ruby", receiver.Languages())
}

// chunkedReader is a reader which forces partial reads, this is required
// to trigger some network related bugs, such as body not being read fully by server.
// Without this, all the data could be read/written at once, not triggering the issue.
type chunkedReader struct {
	reader io.Reader
}

func (sr *chunkedReader) Read(p []byte) (n int, err error) {
	size := 1024
	if size > len(p) {
		size = len(p)
	}
	buf := p[0:size]
	return sr.reader.Read(buf)
}

func TestReceiverRateLimiterCancel(t *testing.T) {
	assert := assert.New(t)

	var wg sync.WaitGroup
	var buf bytes.Buffer

	n := 100 // Payloads need to be big enough, else bug is not triggered
	msgp.Encode(&buf, testutil.GetTestTraces(n, n, true))

	conf := newTestReceiverConfig()
	receiver := newTestReceiverFromConfig(conf)
	receiver.RateLimiter.SetTargetRate(0.000001) // Make sure we sample aggressively

	server := httptest.NewServer(http.HandlerFunc(receiver.handleWithVersion(v04, receiver.handleTraces)))

	defer server.Close()
	url := server.URL + "/v0.4/traces"

	// Make sure we use share clients, and they are reused.
	client := &http.Client{Transport: &http.Transport{
		MaxIdleConnsPerHost: 100,
	}}
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			for j := 0; j < 3; j++ {
				reader := &chunkedReader{reader: bytes.NewReader(buf.Bytes())}
				req, err := http.NewRequest("POST", url, reader)
				req.Header.Set("Content-Type", "application/msgpack")
				req.Header.Set(headerTraceCount, strconv.Itoa(n))
				assert.Nil(err)

				resp, err := client.Do(req)
				assert.Nil(err)
				assert.NotNil(resp)
				if resp != nil {
					assert.Equal(http.StatusOK, resp.StatusCode)
				}
			}
			wg.Done()
		}()
	}
	wg.Wait()
}

func BenchmarkHandleTracesFromOneApp(b *testing.B) {
	// prepare the payload
	// msgpack payload
	var buf bytes.Buffer
	msgp.Encode(&buf, testutil.GetTestTraces(1, 1, true))

	// prepare the receiver
	conf := newTestReceiverConfig()
	receiver := newTestReceiverFromConfig(conf)

	// response recorder
	handler := http.HandlerFunc(receiver.handleWithVersion(v04, receiver.handleTraces))

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
		req, _ := http.NewRequest("POST", "/v0.4/traces", bytes.NewReader(buf.Bytes()))
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
	// prepare the payload
	// msgpack payload
	var buf bytes.Buffer
	msgp.Encode(&buf, testutil.GetTestTraces(1, 1, true))

	// prepare the receiver
	conf := newTestReceiverConfig()
	receiver := newTestReceiverFromConfig(conf)

	// response recorder
	handler := http.HandlerFunc(receiver.handleWithVersion(v04, receiver.handleTraces))

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
		req, _ := http.NewRequest("POST", "/v0.4/traces", bytes.NewReader(buf.Bytes()))
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
	var buf bytes.Buffer
	err := msgp.Encode(&buf, testutil.GetTestTraces(150, 66, true))
	assert.Nil(err)

	// benchmark
	b.ResetTimer()
	b.ReportAllocs()
	for n := 0; n < b.N; n++ {
		b.StopTimer()
		reader := bytes.NewReader(buf.Bytes())

		b.StartTimer()
		var traces pb.Traces
		_ = msgp.Decode(reader, &traces)
	}
}

func BenchmarkWatchdog(b *testing.B) {
	now := time.Now()
	conf := config.New()
	conf.Endpoints[0].APIKey = "apikey_2"
	r := NewHTTPReceiver(conf, nil, nil)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		r.watchdog(now)
	}
}

func TestReplyOKV5(t *testing.T) {
	r := newTestReceiverFromConfig(config.New())
	r.Start()
	defer r.Stop()

	data, err := vmsgp.Marshal([2][]interface{}{{}, {}})
	assert.NoError(t, err)
	path := fmt.Sprintf("http://%s:%d/v0.5/traces", r.conf.ReceiverHost, r.conf.ReceiverPort)
	resp, err := http.Post(path, "application/msgpack", bytes.NewReader(data))
	assert.NoError(t, err)
	slurp, err := ioutil.ReadAll(resp.Body)
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

	r := newTestReceiverFromConfig(config.New())
	r.Start()
	defer r.Stop()

	resp, err := http.Get("http://localhost:8126/debug/vars")
	assert.NoError(t, err)
	defer resp.Body.Close()
	assert.EqualValues(t, resp.StatusCode, http.StatusOK, "failed to read expvars from local server")

	if resp.StatusCode == http.StatusOK {
		var out map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&out)
		assert.NoError(t, err, "/debug/vars must return valid json")
	}
}

func TestWatchdog(t *testing.T) {
	t.Run("rate-limit", func(t *testing.T) {
		if testing.Short() {
			return
		}
		os.Setenv("DD_APM_FEATURES", "429")
		defer os.Unsetenv("DD_APM_FEATURES")

		conf := config.New()
		conf.Endpoints[0].APIKey = "apikey_2"
		conf.MaxMemory = 1e10
		conf.WatchdogInterval = time.Minute // we trigger manually

		r := newTestReceiverFromConfig(conf)
		r.Start()
		defer r.Stop()
		go func() {
			for range r.out {
			}
		}()

		data := msgpTraces(t, pb.Traces{
			testutil.RandomTrace(10, 20),
			testutil.RandomTrace(10, 20),
			testutil.RandomTrace(10, 20),
		})

		// first request is accepted
		r.watchdog(time.Now())
		resp, err := http.Post("http://localhost:8126/v0.4/traces", "application/msgpack", bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("got %d", resp.StatusCode)
		}

		// follow-up requests should trigger a reject
		r.conf.MaxMemory = 1
		for tries := 0; tries < 100; tries++ {
			req, err := http.NewRequest("POST", "http://localhost:8126/v0.4/traces", bytes.NewReader(data))
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Content-Type", "application/msgpack")
			req.Header.Set(headerTraceCount, "3")
			resp, err = http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			if resp.StatusCode == http.StatusTooManyRequests {
				break // 👍
			}
			r.watchdog(time.Now())
		}
		if resp.StatusCode != http.StatusTooManyRequests {
			t.Fatalf("didn't close, got %d", resp.StatusCode)
		}
	})

	t.Run("disabling", func(t *testing.T) {
		cfg := config.New()
		r := &HTTPReceiver{
			conf:        cfg,
			RateLimiter: newRateLimiter(),
		}

		cfg.MaxMemory = 0
		cfg.MaxCPU = 0
		r.watchdog(time.Now())
		assert.Equal(t, 1.0, r.RateLimiter.TargetRate())

		cfg.MaxMemory = 1
		r.watchdog(time.Now())
		assert.NotEqual(t, 1.0, r.RateLimiter.TargetRate())
	})
}

func msgpTraces(t *testing.T, traces pb.Traces) []byte {
	var body bytes.Buffer
	if err := msgp.Encode(&body, traces); err != nil {
		t.Fatal(err)
	}
	return body.Bytes()
}
