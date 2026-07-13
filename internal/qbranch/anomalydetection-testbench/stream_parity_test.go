// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/memory"
	"github.com/apache/arrow-go/v18/parquet/pqarrow"
	"github.com/stretchr/testify/require"

	observerimpl "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/impl"
	"github.com/DataDog/datadog-agent/internal/qbranch/anomalydetection-testbench/bench"
)

func TestHeadlessStreamParquetMatchesBatchOutput(t *testing.T) {
	scenariosDir := t.TempDir()
	parquetDir := filepath.Join(scenariosDir, "scenario", "parquet")
	require.NoError(t, os.MkdirAll(parquetDir, 0o755))
	writeParityLogParquet(t, filepath.Join(parquetDir, "observer-logs-000000.parquet"))
	writeParityMetricParquet(t, filepath.Join(parquetDir, "observer-metrics-000000.parquet"))

	settings := observerimpl.ComponentSettings{Baseline: observerimpl.BaselineConfig{Enabled: false}}
	batchOutput := filepath.Join(t.TempDir(), "batch.json")
	streamOutput := filepath.Join(t.TempDir(), "stream.json")

	runFxApp(t, CLIParams{
		ScenariosDir:      scenariosDir,
		Headless:          "scenario",
		Output:            batchOutput,
		BatchParquet:      true,
		ComponentSettings: settings,
	})
	runFxApp(t, CLIParams{
		ScenariosDir:      scenariosDir,
		Headless:          "scenario",
		Output:            streamOutput,
		ComponentSettings: settings,
	})

	batch := readObserverOutput(t, batchOutput)
	stream := readObserverOutput(t, streamOutput)
	// Processing durations vary between runs; counts and observer results must match.
	batch.Metadata.Stats.DetectorStats = nil
	stream.Metadata.Stats.DetectorStats = nil
	require.Equal(t, batch, stream)
}

func readObserverOutput(t *testing.T, path string) bench.ObserverOutput {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var output bench.ObserverOutput
	require.NoError(t, json.Unmarshal(data, &output))
	require.NotNil(t, output.Metadata.Stats)
	return output
}

func writeParityLogParquet(t *testing.T, path string) {
	t.Helper()
	schema := arrow.NewSchema([]arrow.Field{
		{Name: "RunID", Type: arrow.BinaryTypes.String},
		{Name: "Time", Type: arrow.PrimitiveTypes.Int64},
		{Name: "Content", Type: arrow.BinaryTypes.Binary},
		{Name: "Status", Type: arrow.BinaryTypes.String},
		{Name: "Hostname", Type: arrow.BinaryTypes.String},
		{Name: "Tags", Type: arrow.ListOf(arrow.BinaryTypes.String)},
	}, nil)
	builder := array.NewRecordBuilder(memory.DefaultAllocator, schema)
	defer builder.Release()

	runIDBuilder := builder.Field(0).(*array.StringBuilder)
	timeBuilder := builder.Field(1).(*array.Int64Builder)
	contentBuilder := builder.Field(2).(*array.BinaryBuilder)
	statusBuilder := builder.Field(3).(*array.StringBuilder)
	hostnameBuilder := builder.Field(4).(*array.StringBuilder)
	tagsBuilder := builder.Field(5).(*array.ListBuilder)
	tagValueBuilder := tagsBuilder.ValueBuilder().(*array.StringBuilder)

	for i := 0; i < 120; i++ {
		runIDBuilder.Append("run")
		timeBuilder.Append(int64(1_000_000 + i*1_000))
		contentBuilder.Append([]byte("request failed while connecting to database"))
		statusBuilder.Append("error")
		hostnameBuilder.Append("host-a")
		tagsBuilder.Append(true)
		tagValueBuilder.AppendValues([]string{"service:api", "env:test"}, nil)
	}

	record := builder.NewRecord()
	defer record.Release()
	table := array.NewTableFromRecords(schema, []arrow.Record{record})
	defer table.Release()

	f, err := os.Create(path)
	require.NoError(t, err)
	require.NoError(t, pqarrow.WriteTable(table, f, 8, nil, pqarrow.DefaultWriterProps()))
}

func writeParityMetricParquet(t *testing.T, path string) {
	t.Helper()
	schema := arrow.NewSchema([]arrow.Field{
		{Name: "RunID", Type: arrow.BinaryTypes.String},
		{Name: "Time", Type: arrow.PrimitiveTypes.Int64},
		{Name: "MetricName", Type: arrow.BinaryTypes.String},
		{Name: "ValueFloat", Type: arrow.PrimitiveTypes.Float64},
		{Name: "Tags", Type: arrow.ListOf(arrow.BinaryTypes.String)},
		{Name: "Dropped", Type: arrow.FixedWidthTypes.Boolean},
	}, nil)
	builder := array.NewRecordBuilder(memory.DefaultAllocator, schema)
	defer builder.Release()

	runIDBuilder := builder.Field(0).(*array.StringBuilder)
	timeBuilder := builder.Field(1).(*array.Int64Builder)
	metricNameBuilder := builder.Field(2).(*array.StringBuilder)
	valueBuilder := builder.Field(3).(*array.Float64Builder)
	tagsBuilder := builder.Field(4).(*array.ListBuilder)
	tagValueBuilder := tagsBuilder.ValueBuilder().(*array.StringBuilder)
	droppedBuilder := builder.Field(5).(*array.BooleanBuilder)

	for i := 0; i < 120; i++ {
		runIDBuilder.Append("run")
		timeBuilder.Append(int64(1_000_000 + i*1_000))
		metricNameBuilder.Append("system.cpu.user")
		valueBuilder.Append(float64(i))
		tagsBuilder.Append(true)
		tagValueBuilder.Append("host:a")
		droppedBuilder.Append(false)
	}

	record := builder.NewRecord()
	defer record.Release()
	table := array.NewTableFromRecords(schema, []arrow.Record{record})
	defer table.Release()

	f, err := os.Create(path)
	require.NoError(t, err)
	require.NoError(t, pqarrow.WriteTable(table, f, 8, nil, pqarrow.DefaultWriterProps()))
}
