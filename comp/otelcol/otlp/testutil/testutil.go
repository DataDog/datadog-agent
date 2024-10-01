// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

package testutil

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	"github.com/DataDog/opentelemetry-mapping-go/pkg/inframetadata/payload"
	"github.com/DataDog/opentelemetry-mapping-go/pkg/otlp/attributes/source"
	"github.com/DataDog/sketches-go/ddsketch"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"google.golang.org/protobuf/proto"

	pkgConfigModel "github.com/DataDog/datadog-agent/pkg/config/model"
	pkgConfigSetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
)

// OTLPConfigFromPorts creates a test OTLP config map.
func OTLPConfigFromPorts(bindHost string, gRPCPort uint, httpPort uint) map[string]interface{} {
	otlpConfig := map[string]interface{}{"protocols": map[string]interface{}{}}

	if gRPCPort > 0 {
		otlpConfig["protocols"].(map[string]interface{})["grpc"] = map[string]interface{}{
			"endpoint": fmt.Sprintf("%s:%d", bindHost, gRPCPort),
		}
	}
	if httpPort > 0 {
		otlpConfig["protocols"].(map[string]interface{})["http"] = map[string]interface{}{
			"endpoint": fmt.Sprintf("%s:%d", bindHost, httpPort),
		}
	}
	return otlpConfig
}

// LoadConfig from a given path.
func LoadConfig(path string) (pkgConfigModel.Reader, error) {
	cfg := pkgConfigModel.NewConfig("datadog", "DD", strings.NewReplacer(".", "_"))
	pkgConfigSetup.OTLP(cfg)
	cfg.SetConfigFile(path)
	err := cfg.ReadInConfig()
	if err != nil {
		return nil, err
	}

	return cfg, nil
}

// JSONLogs is the type for the array of processed JSON log data from each request
type JSONLogs []map[string]any

var (
	// TestLogTime is the default time used for tests.
	TestLogTime = time.Date(2020, 2, 11, 20, 26, 13, 789, time.UTC)
	// TestLogTimestamp is the default timestamp used for tests.
	TestLogTimestamp = pcommon.NewTimestampFromTime(TestLogTime)
)

// GenerateLogsOneEmptyResourceLogs generates one empty logs structure.
func GenerateLogsOneEmptyResourceLogs() plog.Logs {
	ld := plog.NewLogs()
	ld.ResourceLogs().AppendEmpty()
	return ld
}

// GenerateLogsNoLogRecords generates a logs structure with one entry.
func GenerateLogsNoLogRecords() plog.Logs {
	ld := GenerateLogsOneEmptyResourceLogs()
	ld.ResourceLogs().At(0).Resource().Attributes().PutStr("resource-attr", "resource-attr-val-1")
	return ld
}

// GenerateLogsOneEmptyLogRecord generates a log structure with one empty record.
func GenerateLogsOneEmptyLogRecord() plog.Logs {
	ld := GenerateLogsNoLogRecords()
	rs0 := ld.ResourceLogs().At(0)
	rs0.ScopeLogs().AppendEmpty().LogRecords().AppendEmpty()
	return ld
}

// GenerateLogsOneLogRecordNoResource generates a logs structure with one record and no resource.
func GenerateLogsOneLogRecordNoResource() plog.Logs {
	ld := GenerateLogsOneEmptyResourceLogs()
	rs0 := ld.ResourceLogs().At(0)
	fillLogOne(rs0.ScopeLogs().AppendEmpty().LogRecords().AppendEmpty())
	return ld
}

// GenerateLogsOneLogRecord generates a logs structure with one record.
func GenerateLogsOneLogRecord() plog.Logs {
	ld := GenerateLogsOneEmptyLogRecord()
	fillLogOne(ld.ResourceLogs().At(0).ScopeLogs().At(0).LogRecords().At(0))
	return ld
}

// GenerateLogsTwoLogRecordsSameResource generates a logs structure with two log records sharding
// the same resource.
func GenerateLogsTwoLogRecordsSameResource() plog.Logs {
	ld := GenerateLogsOneEmptyLogRecord()
	logs := ld.ResourceLogs().At(0).ScopeLogs().At(0).LogRecords()
	fillLogOne(logs.At(0))
	fillLogTwo(logs.AppendEmpty())
	return ld
}

func fillLogOne(log plog.LogRecord) {
	log.SetTimestamp(TestLogTimestamp)
	log.SetDroppedAttributesCount(1)
	log.SetSeverityNumber(plog.SeverityNumberInfo)
	log.SetSeverityText("Info")
	log.SetSpanID([8]byte{0x01, 0x02, 0x04, 0x08})
	log.SetTraceID([16]byte{0x08, 0x04, 0x02, 0x01})

	attrs := log.Attributes()
	attrs.PutStr("app", "server")
	attrs.PutInt("instance_num", 1)

	log.Body().SetStr("This is a log message")
}

func fillLogTwo(log plog.LogRecord) {
	log.SetTimestamp(TestLogTimestamp)
	log.SetDroppedAttributesCount(1)
	log.SetSeverityNumber(plog.SeverityNumberInfo)
	log.SetSeverityText("Info")

	attrs := log.Attributes()
	attrs.PutStr("customer", "acme")
	attrs.PutStr("env", "dev")

	log.Body().SetStr("something happened")
}

// DatadogServer is the mock Datadog backend server
type DatadogServer struct {
	*httptest.Server
	MetadataChan chan payload.HostMetadata
}

/* #nosec G101 -- This is a false positive, these are API endpoints rather than credentials */
const (
	ValidateAPIKeyEndpoint = "/api/v1/validate" // nolint G101
	MetricV1Endpoint       = "/api/v1/series"
	MetricV2Endpoint       = "/api/v2/series"
	SketchesMetricEndpoint = "/api/beta/sketches"
	MetadataEndpoint       = "/intake"
	TraceEndpoint          = "/api/v0.2/traces"
	APMStatsEndpoint       = "/api/v0.2/stats"
)

// DatadogServerMock mocks a Datadog backend server
func DatadogServerMock(overwriteHandlerFuncs ...OverwriteHandleFunc) *DatadogServer {
	metadataChan := make(chan payload.HostMetadata)
	mux := http.NewServeMux()

	handlers := map[string]http.HandlerFunc{
		ValidateAPIKeyEndpoint: validateAPIKeyEndpoint,
		MetricV1Endpoint:       metricsEndpoint,
		MetricV2Endpoint:       metricsV2Endpoint,
		MetadataEndpoint:       newMetadataEndpoint(metadataChan),
		"/":                    func(_ http.ResponseWriter, _ *http.Request) {},
	}
	for _, f := range overwriteHandlerFuncs {
		p, hf := f()
		handlers[p] = hf
	}
	for pattern, handler := range handlers {
		mux.HandleFunc(pattern, handler)
	}

	srv := httptest.NewServer(mux)

	return &DatadogServer{
		srv,
		metadataChan,
	}
}

// OverwriteHandleFunc allows to overwrite the default handler functions
type OverwriteHandleFunc func() (string, http.HandlerFunc)

// HTTPRequestRecorder records a HTTP request.
type HTTPRequestRecorder struct {
	Pattern  string
	Header   http.Header
	ByteBody []byte
}

// HandlerFunc implements an HTTP handler
func (rec *HTTPRequestRecorder) HandlerFunc() (string, http.HandlerFunc) {
	return rec.Pattern, func(_ http.ResponseWriter, r *http.Request) {
		rec.Header = r.Header
		rec.ByteBody, _ = io.ReadAll(r.Body)
	}
}

// HTTPRequestRecorderWithChan puts all incoming HTTP request bytes to the given channel.
type HTTPRequestRecorderWithChan struct {
	Pattern string
	ReqChan chan []byte
}

// HandlerFunc implements an HTTP handler
func (rec *HTTPRequestRecorderWithChan) HandlerFunc() (string, http.HandlerFunc) {
	return rec.Pattern, func(_ http.ResponseWriter, r *http.Request) {
		bytesBody, _ := io.ReadAll(r.Body)
		rec.ReqChan <- bytesBody
	}
}

// ValidateAPIKeyEndpointInvalid returns a handler function that returns an invalid API key response
func ValidateAPIKeyEndpointInvalid() (string, http.HandlerFunc) {
	return "/api/v1/validate", validateAPIKeyEndpointInvalid
}

type validateAPIKeyResponse struct {
	Valid bool `json:"valid"`
}

func validateAPIKeyEndpoint(w http.ResponseWriter, _ *http.Request) {
	res := validateAPIKeyResponse{Valid: true}
	resJSON, _ := json.Marshal(res)

	w.Header().Set("Content-Type", "application/json")
	_, err := w.Write(resJSON)
	if err != nil {
		log.Fatalln(err)
	}
}

func validateAPIKeyEndpointInvalid(w http.ResponseWriter, _ *http.Request) {
	res := validateAPIKeyResponse{Valid: false}
	resJSON, _ := json.Marshal(res)

	w.Header().Set("Content-Type", "application/json")
	_, err := w.Write(resJSON)
	if err != nil {
		log.Fatalln(err)
	}
}

type metricsResponse struct {
	Status string `json:"status"`
}

func metricsEndpoint(w http.ResponseWriter, _ *http.Request) {
	res := metricsResponse{Status: "ok"}
	resJSON, _ := json.Marshal(res)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_, err := w.Write(resJSON)
	if err != nil {
		log.Fatalln(err)
	}
}

func metricsV2Endpoint(w http.ResponseWriter, _ *http.Request) {
	res := metricsResponse{Status: "ok"}
	resJSON, _ := json.Marshal(res)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_, err := w.Write(resJSON)
	if err != nil {
		log.Fatalln(err)
	}
}

func newMetadataEndpoint(c chan payload.HostMetadata) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		reader, err := gzip.NewReader(r.Body)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}

		body, err := io.ReadAll(reader)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}

		var recvMetadata payload.HostMetadata
		if err = json.Unmarshal(body, &recvMetadata); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}

		c <- recvMetadata
	}
}

var (
	testAttributes = map[string]string{"datadog.host.name": "custom-hostname"}
	// TestMetrics metrics for tests.
	TestMetrics = newMetricsWithAttributeMap(testAttributes)
	// TestTraces traces for tests.
	TestTraces = newTracesWithAttributeMap(testAttributes)
)

func fillAttributeMap(attrs pcommon.Map, mp map[string]string) {
	attrs.EnsureCapacity(len(mp))
	for k, v := range mp {
		attrs.PutStr(k, v)
	}
}

// NewAttributeMap creates a new attribute map (string only)
// from a Go map
func NewAttributeMap(mp map[string]string) pcommon.Map {
	attrs := pcommon.NewMap()
	fillAttributeMap(attrs, mp)
	return attrs
}

// TestGauge holds the definition of a basic gauge.
type TestGauge struct {
	Name       string
	DataPoints []DataPoint
}

// DataPoint specifies a DoubleVal data point and its attributes.
type DataPoint struct {
	Value      float64
	Attributes map[string]string
}

// NewGaugeMetrics creates a set of pmetric.Metrics containing all the specified
// test gauges.
func NewGaugeMetrics(tgs []TestGauge) pmetric.Metrics {
	metrics := pmetric.NewMetrics()
	all := metrics.ResourceMetrics().AppendEmpty().ScopeMetrics().AppendEmpty().Metrics()
	for _, tg := range tgs {
		m := all.AppendEmpty()
		m.SetName(tg.Name)
		g := m.SetEmptyGauge()
		for _, dp := range tg.DataPoints {
			d := g.DataPoints().AppendEmpty()
			d.SetDoubleValue(dp.Value)
			fillAttributeMap(d.Attributes(), dp.Attributes)
		}
	}
	return metrics
}

func newMetricsWithAttributeMap(mp map[string]string) pmetric.Metrics {
	md := pmetric.NewMetrics()
	fillAttributeMap(md.ResourceMetrics().AppendEmpty().Resource().Attributes(), mp)
	return md
}

func newTracesWithAttributeMap(mp map[string]string) ptrace.Traces {
	traces := ptrace.NewTraces()
	resourceSpans := traces.ResourceSpans()
	rs := resourceSpans.AppendEmpty()
	fillAttributeMap(rs.Resource().Attributes(), mp)
	rs.ScopeSpans().AppendEmpty().Spans().AppendEmpty()
	return traces
}

// MockSourceProvider mocks a source provider
type MockSourceProvider struct {
	Src source.Source
}

// Source implements the source provider interface
func (s *MockSourceProvider) Source(_ context.Context) (source.Source, error) {
	return s.Src, nil
}

// MockStatsProcessor mocks a stats processor
type MockStatsProcessor struct {
	In []*pb.ClientStatsPayload
}

// ProcessStats implements the stats processor interface
func (s *MockStatsProcessor) ProcessStats(in *pb.ClientStatsPayload, _, _ string) {
	s.In = append(s.In, in)
}

// StatsPayloads contains a couple of *pb.ClientStatsPayloads used for testing.
var StatsPayloads = []*pb.ClientStatsPayload{
	{
		Hostname:         "host",
		Env:              "prod",
		Version:          "v1.2",
		Lang:             "go",
		TracerVersion:    "v44",
		RuntimeID:        "123jkl",
		Sequence:         2,
		AgentAggregation: "blah",
		Service:          "mysql",
		ContainerID:      "abcdef123456",
		Tags:             []string{"a:b", "c:d"},
		Stats: []*pb.ClientStatsBucket{
			{
				Start:    10,
				Duration: 1,
				Stats: []*pb.ClientGroupedStats{
					{
						Service:        "kafka",
						Name:           "queue.add",
						Resource:       "append",
						HTTPStatusCode: 220,
						Type:           "queue",
						Hits:           15,
						Errors:         3,
						Duration:       143,
						OkSummary:      testSketchBytes(1, 2, 3),
						ErrorSummary:   testSketchBytes(4, 5, 6),
						TopLevelHits:   5,
					},
				},
			},
		},
	},
	{
		Hostname:         "host2",
		Env:              "prod2",
		Version:          "v1.22",
		Lang:             "go2",
		TracerVersion:    "v442",
		RuntimeID:        "123jkl2",
		Sequence:         22,
		AgentAggregation: "blah2",
		Service:          "mysql2",
		ContainerID:      "abcdef1234562",
		Tags:             []string{"a:b2", "c:d2"},
		Stats: []*pb.ClientStatsBucket{
			{
				Start:    102,
				Duration: 12,
				Stats: []*pb.ClientGroupedStats{
					{
						Service:        "kafka2",
						Name:           "queue.add2",
						Resource:       "append2",
						HTTPStatusCode: 2202,
						Type:           "queue2",
						Hits:           152,
						Errors:         32,
						Duration:       1432,
						OkSummary:      testSketchBytes(7, 8),
						ErrorSummary:   testSketchBytes(9, 10, 11),
						TopLevelHits:   52,
					},
				},
			},
		},
	},
}

// The sketch's relative accuracy and maximum number of bins is identical
// to the one used in the trace-agent for consistency:
// https://github.com/DataDog/datadog-agent/blob/cbac965/pkg/trace/stats/statsraw.go#L18-L26
const (
	sketchRelativeAccuracy = 0.01
	sketchMaxBins          = 2048
)

// testSketchBytes returns the proto-encoded version of a DDSketch containing the
// points in nums.
func testSketchBytes(nums ...float64) []byte {
	sketch, err := ddsketch.LogCollapsingLowestDenseDDSketch(sketchRelativeAccuracy, sketchMaxBins)
	if err != nil {
		// the only possible error is if the relative accuracy is < 0 or > 1;
		// we know that's not the case because it's a constant defined as 0.01
		panic(err)
	}
	for _, num := range nums {
		if err2 := sketch.Add(num); err2 != nil {
			panic(err2)
		}
	}
	buf, err := proto.Marshal(sketch.ToProto())
	if err != nil {
		// there should be no error under any circumstances here
		panic(err)
	}
	return buf
}

// JSONLog is the type for the processed JSON log data from a single log
type JSONLog map[string]any

// HasDDTag returns true if every log has the given ddtags
func (jsonLogs *JSONLogs) HasDDTag(ddtags string) bool {
	for _, logData := range *jsonLogs {
		if ddtags != logData["ddtags"] {
			return false
		}
	}
	return true
}

// DatadogLogsServer implements a HTTP server that accepts Datadog logs
type DatadogLogsServer struct {
	*httptest.Server
	// LogsData is the array of json requests sent to datadog backend
	LogsData JSONLogs
}

// DatadogLogServerMock mocks a Datadog Logs Intake backend server
func DatadogLogServerMock(overwriteHandlerFuncs ...OverwriteHandleFunc) *DatadogLogsServer {
	mux := http.NewServeMux()

	server := &DatadogLogsServer{}
	handlers := map[string]http.HandlerFunc{
		// logs backend doesn't have validate endpoint
		// but adding one here for ease of testing
		"/api/v1/validate": validateAPIKeyEndpoint,
		"/":                server.logsEndpoint,
	}
	for _, f := range overwriteHandlerFuncs {
		p, hf := f()
		handlers[p] = hf
	}
	for pattern, handler := range handlers {
		mux.HandleFunc(pattern, handler)
	}
	server.Server = httptest.NewServer(mux)
	return server
}

func (s *DatadogLogsServer) logsEndpoint(w http.ResponseWriter, r *http.Request) {
	jsonLogs := processLogsRequest(w, r)
	s.LogsData = append(s.LogsData, jsonLogs...)
}

func processLogsRequest(w http.ResponseWriter, r *http.Request) JSONLogs {
	// we can reuse same response object for logs as well
	req, err := gUnzipData(r.Body)
	handleError(w, err, http.StatusBadRequest)
	var jsonLogs JSONLogs
	err = json.Unmarshal(req, &jsonLogs)
	handleError(w, err, http.StatusBadRequest)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_, err = w.Write([]byte(`{"status":"ok"}`))
	handleError(w, err, 0)
	return jsonLogs
}

func gUnzipData(rg io.Reader) ([]byte, error) {
	r, err := gzip.NewReader(rg)
	if err != nil {
		return nil, err
	}
	return io.ReadAll(r)
}

// handleError logs the given error and writes the given status code if one is provided
// A statusCode of 0 represents no status code to write
func handleError(w http.ResponseWriter, err error, statusCode int) {
	if err != nil {
		if statusCode != 0 {
			w.WriteHeader(statusCode)
		}
		log.Fatalln(err)
	}
}

// MockLogsEndpoint returns the processed JSON log data for each endpoint call
func MockLogsEndpoint(w http.ResponseWriter, r *http.Request) JSONLogs {
	return processLogsRequest(w, r)
}

// ProcessLogsAgentRequest handles HTTP requests from logs agent
func ProcessLogsAgentRequest(w http.ResponseWriter, r *http.Request) JSONLogs {
	// we can reuse same response object for logs as well
	req, err := gUnzipData(r.Body)
	handleError(w, err, http.StatusBadRequest)
	var jsonLogs JSONLogs
	err = json.Unmarshal(req, &jsonLogs)
	handleError(w, err, http.StatusBadRequest)

	// unmarshal nested message JSON
	for i := range jsonLogs {
		messageJSON := jsonLogs[i]["message"].(string)
		var message JSONLog
		err = json.Unmarshal([]byte(messageJSON), &message)
		handleError(w, err, http.StatusBadRequest)
		jsonLogs[i]["message"] = message
		// delete dynamic keys that can't be tested
		delete(jsonLogs[i], "hostname")  // hostname of host running tests
		delete(jsonLogs[i], "timestamp") // ingestion timestamp
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_, err = w.Write([]byte(`{"status":"ok"}`))
	handleError(w, err, 0)
	return jsonLogs
}
