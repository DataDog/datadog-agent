// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

//go:build test

package testutil

import (
	"fmt"
	"strings"
	"time"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"

	"github.com/DataDog/datadog-agent/pkg/config"
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
func LoadConfig(path string) (config.Config, error) {
	cfg := config.NewConfig("datadog", "DD", strings.NewReplacer(".", "_"))
	config.SetupOTLP(cfg)
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
