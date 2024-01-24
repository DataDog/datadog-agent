// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package info

import (
	"bytes"
	"encoding/json"
	"expvar"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/watchdog"
)

var clearAddEp = map[string][]string{
	"ep": {"aaaaaaaaaaaaaaaaaaaaaaaaaaaabbbb", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaabbbb"},
}

var scrubbedAddEp = map[string][]string{
	"ep": {"***************************abbbb", "***********************************abbbb"},
}

type testServerHandler struct {
	t *testing.T
}

func (h *testServerHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	json, err := os.ReadFile("./testdata/okay.json")
	if err != nil {
		h.t.Errorf("error loading json file: %v", err)
	}

	switch r.URL.Path {
	case "/debug/vars":
		h.t.Logf("serving fake (static) info data for %s", r.URL.Path)
		_, err := w.Write(json)
		if err != nil {
			h.t.Errorf("error serving %s: %v", r.URL.Path, err)
		}
	default:
		h.t.Logf("answering 404 for %s", r.URL.Path)
		w.WriteHeader(http.StatusNotFound)
	}
}

func testServer(t *testing.T) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(&testServerHandler{t: t})
	t.Logf("test server (serving fake yet valid data) listening on %s", server.URL)
	return server
}

type testServerWarningHandler struct {
	t *testing.T
}

func (h *testServerWarningHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	json, err := os.ReadFile("./testdata/warning.json")
	if err != nil {
		h.t.Errorf("error loading json file: %v", err)
	}

	switch r.URL.Path {
	case "/debug/vars":
		h.t.Logf("serving fake (static) info data for %s", r.URL.Path)
		_, err := w.Write(json)
		if err != nil {
			h.t.Errorf("error serving %s: %v", r.URL.Path, err)
		}
	default:
		h.t.Logf("answering 404 for %s", r.URL.Path)
		w.WriteHeader(http.StatusNotFound)
	}
}

func testServerWarning(t *testing.T) *httptest.Server {
	server := httptest.NewServer(&testServerWarningHandler{t: t})
	t.Logf("test server (serving data containing worrying values) listening on %s", server.URL)
	return server
}

type testServerErrorHandler struct {
	t *testing.T
}

func (h *testServerErrorHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	switch r.URL.Path {
	case "/debug/vars":
		h.t.Logf("serving fake (static) info data for %s", r.URL.Path)
		_, err := w.Write([]byte(`this is *NOT* a valid JSON, no way...`))
		if err != nil {
			h.t.Errorf("error serving %s: %v", r.URL.Path, err)
		}
	default:
		h.t.Logf("answering 404 for %s", r.URL.Path)
		w.WriteHeader(http.StatusNotFound)
	}
}

func testServerError(t *testing.T) *httptest.Server {
	server := httptest.NewServer(&testServerErrorHandler{t: t})
	t.Logf("test server (serving bad data to trigger errors) listening on %s", server.URL)
	return server
}

// run this at the beginning of each test, this is because we *really*
// need to have InitInfo be called before doing anything
func testInit(t *testing.T) *config.AgentConfig {
	assert := assert.New(t)
	conf := config.New()
	conf.Endpoints[0].APIKey = "key1"
	conf.Endpoints = append(conf.Endpoints, &config.Endpoint{Host: "ABC", APIKey: "key2"})
	conf.TelemetryConfig.Endpoints[0].APIKey = "key1"
	conf.Proxy = nil
	conf.EVPProxy.APIKey = "evp_api_key"
	conf.EVPProxy.ApplicationKey = "evp_app_key"
	conf.EVPProxy.AdditionalEndpoints = clearAddEp
	conf.ProfilingProxy.AdditionalEndpoints = clearAddEp
	conf.DebuggerProxy.APIKey = "debugger_proxy_key"
	assert.NotNil(conf)

	err := InitInfo(conf)
	assert.NoError(err)

	return conf
}

func TestInfo(t *testing.T) {
	assert := assert.New(t)
	conf := testInit(t)
	assert.NotNil(conf)

	server := testServer(t)
	assert.NotNil(server)
	defer server.Close()

	url, err := url.Parse(server.URL)
	assert.NotNil(url)
	assert.NoError(err)

	hostPort := strings.Split(url.Host, ":")
	assert.Equal(2, len(hostPort))
	port, err := strconv.Atoi(hostPort[1])
	assert.NoError(err)
	conf.DebugServerPort = port

	var buf bytes.Buffer
	err = Info(&buf, conf)
	assert.NoError(err)
	info := buf.String()
	assert.NotEmpty(info)
	t.Logf("Info:\n%s\n", info)
	expectedInfo, err := os.ReadFile("./testdata/okay.info")
	re := regexp.MustCompile(`\r\n`)
	expectedInfoString := re.ReplaceAllString(string(expectedInfo), "\n")
	assert.NoError(err)
	assert.Equal(expectedInfoString, info)
}

func TestHideAPIKeys(t *testing.T) {
	assert := assert.New(t)
	conf := testInit(t)

	js := expvar.Get("config").String()
	assert.NotEqual("", js)
	var got config.AgentConfig
	err := json.Unmarshal([]byte(js), &got)
	assert.NoError(err)
	assert.NotEmpty(conf.Endpoints[0].APIKey)
	assert.Empty(got.Endpoints[0].APIKey)
}

func TestWarning(t *testing.T) {
	assert := assert.New(t)
	conf := testInit(t)
	assert.NotNil(conf)

	server := testServerWarning(t)
	assert.NotNil(server)
	defer server.Close()

	url, err := url.Parse(server.URL)
	assert.NotNil(url)
	assert.NoError(err)

	hostPort := strings.Split(url.Host, ":")
	assert.Equal(2, len(hostPort))
	port, err := strconv.Atoi(hostPort[1])
	assert.NoError(err)
	conf.DebugServerPort = port

	var buf bytes.Buffer
	err = Info(&buf, conf)
	assert.NoError(err)
	info := buf.String()

	expectedWarning, err := os.ReadFile("./testdata/warning.info")
	re := regexp.MustCompile(`\r\n`)
	expectedWarningString := re.ReplaceAllString(string(expectedWarning), "\n")
	assert.NoError(err)
	assert.Equal(expectedWarningString, info)

	t.Logf("Info:\n%s\n", info)
}

func TestNotRunning(t *testing.T) {
	assert := assert.New(t)
	conf := testInit(t)
	assert.NotNil(conf)

	server := testServer(t)
	assert.NotNil(server)

	url, err := url.Parse(server.URL)
	assert.NotNil(url)
	assert.NoError(err)

	server.Close()

	hostPort := strings.Split(url.Host, ":")
	assert.Equal(2, len(hostPort))
	port, err := strconv.Atoi(hostPort[1])
	assert.NoError(err)
	conf.DebugServerPort = port

	var buf bytes.Buffer
	err = Info(&buf, conf)
	assert.NotNil(err)
	info := buf.String()

	t.Logf("Info:\n%s\n", info)

	lines := strings.Split(info, "\n")
	assert.Equal(7, len(lines))
	assert.Regexp(regexp.MustCompile(`^={10,100}$`), lines[0])
	assert.Regexp(regexp.MustCompile(`^Trace Agent \(v.*\)$`), lines[1])
	assert.Regexp(regexp.MustCompile(`^={10,100}$`), lines[2])
	assert.Equal(len(lines[1]), len(lines[0]))
	assert.Equal(len(lines[1]), len(lines[2]))
	assert.Equal("", lines[3])
	assert.Equal(fmt.Sprintf("  Not running or unreachable on 127.0.0.1:%d", port), lines[4])
	assert.Equal("", lines[5])
	assert.Equal("", lines[6])
}

func TestError(t *testing.T) {
	assert := assert.New(t)
	conf := testInit(t)
	assert.NotNil(conf)

	server := testServerError(t)
	assert.NotNil(server)
	defer server.Close()

	url, err := url.Parse(server.URL)
	assert.NotNil(url)
	assert.NoError(err)

	hostPort := strings.Split(url.Host, ":")
	assert.Equal(2, len(hostPort))
	port, err := strconv.Atoi(hostPort[1])
	assert.NoError(err)
	conf.DebugServerPort = port

	var buf bytes.Buffer
	err = Info(&buf, conf)
	assert.NotNil(err)
	info := buf.String()

	t.Logf("Info:\n%s\n", info)

	lines := strings.Split(info, "\n")
	assert.Equal(8, len(lines))
	assert.Regexp(regexp.MustCompile(`^={10,100}$`), lines[0])
	assert.Regexp(regexp.MustCompile(`^Trace Agent \(v.*\)$`), lines[1])
	assert.Regexp(regexp.MustCompile(`^={10,100}$`), lines[2])
	assert.Equal(len(lines[1]), len(lines[0]))
	assert.Equal(len(lines[1]), len(lines[2]))
	assert.Equal("", lines[3])
	assert.Regexp(regexp.MustCompile(`^  Error: .*$`), lines[4])
	assert.Equal(fmt.Sprintf("  URL: http://127.0.0.1:%d/debug/vars", port), lines[5])
	assert.Equal("", lines[6])
	assert.Equal("", lines[7])
}

func TestInfoReceiverStats(t *testing.T) {
	assert := assert.New(t)
	conf := testInit(t)
	assert.NotNil(conf)

	assert.NotNil(publishReceiverStats())

	stats := NewReceiverStats()
	t1 := &TagStats{
		Tags{Lang: "python"},
		Stats{},
	}
	t1.Stats.TracesReceived.Store(23)
	t1.Stats.TracesBytes.Store(3244)
	t1.Stats.SpansReceived.Store(213)
	t1.Stats.SpansDropped.Store(14)
	t2 := &TagStats{
		Tags{Lang: "go"},
		Stats{},
	}
	stats.Stats = map[Tags]*TagStats{
		t1.Tags: t1,
		t2.Tags: t2,
	}

	// run this with -race flag
	done := make(chan struct{}, 4)
	for i := 0; i < 2; i++ {
		go func() {
			for j := 0; j < 1000; j++ {
				UpdateReceiverStats(stats)
			}
			done <- struct{}{}
		}()
	}
	for i := 0; i < 2; i++ {
		go func() {
			for j := 0; j < 1000; j++ {
				_ = publishReceiverStats()
			}
			done <- struct{}{}
		}()
	}
	for i := 0; i < 4; i++ {
		<-done
	}
	s := publishReceiverStats()
	switch s := s.(type) {
	case []TagStats:
		for _, tagStats := range s {
			assert.Equal(*stats.Stats[tagStats.Tags], tagStats)
		}
	default:
		t.Errorf("bad stats type: %v", s)
	}
	stats.Stats[t1.Tags].TracesReceived.Inc()
	UpdateReceiverStats(stats)
	s = publishReceiverStats()
	switch s := s.(type) {
	case []TagStats:
		for _, tagStats := range s {
			if tagStats.Tags == t1.Tags {
				assert.Equal(t1.Stats.TracesReceived, tagStats.Stats.TracesReceived)
			}
		}
	default:
		t.Errorf("bad stats type: %v", s)
	}
}

func TestInfoConfig(t *testing.T) {
	assert := assert.New(t)
	conf := testInit(t)
	assert.NotNil(conf)

	js := expvar.Get("config").String() // this is what expvar will call
	assert.NotEqual("", js)
	var confCopy config.AgentConfig
	err := json.Unmarshal([]byte(js), &confCopy)
	assert.NoError(err)
	for i, e := range confCopy.Endpoints {
		assert.Equal("", e.APIKey, "API Keys should *NEVER* be exported")
		conf.Endpoints[i].APIKey = "" // make conf equal to confCopy to assert equality of other fields
	}
	for i, e := range confCopy.TelemetryConfig.Endpoints {
		assert.Equal("", e.APIKey, "API Keys should *NEVER* be exported")
		conf.TelemetryConfig.Endpoints[i].APIKey = "" // make conf equal to confCopy to assert equality of other fields
	}
	assert.Equal("", confCopy.EVPProxy.APIKey, "EVP API Key should *NEVER* be exported")
	conf.EVPProxy.APIKey = ""
	assert.Equal("", confCopy.EVPProxy.ApplicationKey, "EVP APP Key should *NEVER* be exported")
	conf.EVPProxy.ApplicationKey = ""
	assert.Equal("", confCopy.DebuggerProxy.APIKey, "Debugger Proxy API Key should *NEVER* be exported")
	conf.DebuggerProxy.APIKey = ""
	assert.Equal("", confCopy.DebuggerDiagnosticsProxy.APIKey, "Debugger Diagnostics Proxy API Key should *NEVER* be exported")
	conf.DebuggerDiagnosticsProxy.APIKey = ""


	// Any key-like data should scrubbed
	conf.EVPProxy.AdditionalEndpoints = scrubbedAddEp
	conf.ProfilingProxy.AdditionalEndpoints = scrubbedAddEp

	conf.ContainerTags = nil

	assert.Equal(*conf, confCopy) // ensure all fields have been exported then parsed correctly
}

func TestPublishUptime(t *testing.T) {
	up := publishUptime()
	// just test the type, as the time itself is nondeterministic
	_, ok := up.(int)
	require.True(t, ok)
}

func TestPublishReceiverStats(t *testing.T) {
	receiverStats = []TagStats{{
		Tags: Tags{
			Lang: "go",
		},
		Stats: Stats{
			TracesReceived: atom(1),
			TracesDropped: &TracesDropped{
				atom(1),
				atom(2),
				atom(3),
				atom(4),
				atom(5),
				atom(6),
				atom(7),
				atom(8),
			},
			SpansMalformed: &SpansMalformed{
				atom(1),
				atom(2),
				atom(3),
				atom(4),
				atom(5),
				atom(6),
				atom(7),
				atom(8),
				atom(9),
				atom(10),
				atom(11),
				atom(12),
				atom(13),
				atom(14),
			},
			TracesFiltered:     atom(4),
			TracesPriorityNone: atom(5),
			TracesPerSamplingPriority: samplingPriorityStats{
				[maxAbsPriority*2 + 1]atomic.Int64{
					maxAbsPriority + 0: atom(1),
					maxAbsPriority + 1: atom(2),
					maxAbsPriority + 2: atom(3),
					maxAbsPriority + 3: atom(4),
					maxAbsPriority + 4: atom(5),
				},
			},
			ClientDroppedP0Traces: atom(7),
			ClientDroppedP0Spans:  atom(8),
			TracesBytes:           atom(9),
			SpansReceived:         atom(10),
			SpansDropped:          atom(11),
			SpansFiltered:         atom(12),
			EventsExtracted:       atom(13),
			EventsSampled:         atom(14),
			PayloadAccepted:       atom(15),
			PayloadRefused:        atom(16),
		},
	}}

	testExpvarPublish(t, publishReceiverStats,
		[]interface{}{map[string]interface{}{
			"ClientDroppedP0Spans":  8.0,
			"ClientDroppedP0Traces": 7.0,
			"EndpointVersion":       "",
			"EventsExtracted":       13.0,
			"EventsSampled":         14.0,
			"Interpreter":           "",
			"Lang":                  "go",
			"LangVendor":            "",
			"LangVersion":           "",
			"PayloadAccepted":       15.0,
			"PayloadRefused":        16.0,
			"SpansDropped":          11.0,
			"SpansFiltered":         12.0,
			"SpansMalformed": map[string]interface{}{
				"DuplicateSpanID":       1.0,
				"ServiceEmpty":          2.0,
				"ServiceTruncate":       3.0,
				"ServiceInvalid":        4.0,
				"PeerServiceTruncate":   5.0,
				"PeerServiceInvalid":    6.0,
				"SpanNameEmpty":         7.0,
				"SpanNameTruncate":      8.0,
				"SpanNameInvalid":       9.0,
				"ResourceEmpty":         10.0,
				"TypeTruncate":          11.0,
				"InvalidStartDate":      12.0,
				"InvalidDuration":       13.0,
				"InvalidHTTPStatusCode": 14.0,
			},
			"SpansReceived": 10.0,
			"TracerVersion": "",
			"TracesBytes":   9.0,
			"TracesDropped": map[string]interface{}{
				"DecodingError":   1.0,
				"PayloadTooLarge": 2.0,
				"EmptyTrace":      3.0,
				"TraceIDZero":     4.0,
				"SpanIDZero":      5.0,
				"ForeignSpan":     6.0,
				"Timeout":         7.0,
				"EOF":             8.0,
			},
			"TracesFiltered":            4.0,
			"TracesPerSamplingPriority": map[string]interface{}{},
			"TracesPriorityNone":        5.0,
			"TracesReceived":            1.0,
		}})
}

func TestPublishWatchdogInfo(t *testing.T) {
	watchdogInfo = watchdog.Info{
		CPU: watchdog.CPUInfo{UserAvg: 1.2},
		Mem: watchdog.MemInfo{Alloc: 1000},
	}

	testExpvarPublish(t, publishWatchdogInfo,
		map[string]interface{}{
			"CPU": map[string]interface{}{"UserAvg": 1.2},
			"Mem": map[string]interface{}{"Alloc": 1000.0},
		})
}

func TestScrubCreds(t *testing.T) {
	assert := assert.New(t)
	conf := testInit(t)
	assert.NotNil(conf)

	confExpvar := expvar.Get("config").String()
	var got config.AgentConfig
	err := json.Unmarshal([]byte(confExpvar), &got)
	assert.NoError(err)

	assert.EqualValues(got.EVPProxy.AdditionalEndpoints, scrubbedAddEp)
	assert.EqualValues(got.ProfilingProxy.AdditionalEndpoints, scrubbedAddEp)
}
