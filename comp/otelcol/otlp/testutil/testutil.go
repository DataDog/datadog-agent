// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

//go:build test

package testutil

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	pkgConfigModel "github.com/DataDog/datadog-agent/pkg/config/model"
	pkgConfigSetup "github.com/DataDog/datadog-agent/pkg/config/setup"

	"github.com/DataDog/opentelemetry-mapping-go/pkg/inframetadata/payload"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
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
