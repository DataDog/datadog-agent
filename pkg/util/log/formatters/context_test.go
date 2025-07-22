// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package formatters

import (
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExtractContextString(t *testing.T) {
	assert.Equal(t, `,"foo":"bar"`, extractContextString(jsonFormat, makeRecord("foo", "bar")))
	assert.Equal(t, `foo:bar | `, extractContextString(textFormat, makeRecord("foo", "bar")))
	assert.Equal(t, `,"foo":"bar","bar":"buzz"`, extractContextString(jsonFormat, makeRecord("foo", "bar", "bar", "buzz")))
	assert.Equal(t, `foo:bar,bar:buzz | `, extractContextString(textFormat, makeRecord("foo", "bar", "bar", "buzz")))
	assert.Equal(t, `,"foo":"b\"a\"r"`, extractContextString(jsonFormat, makeRecord("foo", "b\"a\"r")))
	assert.Equal(t, `,"foo":"3"`, extractContextString(jsonFormat, makeRecord("foo", 3)))
	assert.Equal(t, `,"foo":"4.131313131"`, extractContextString(jsonFormat, makeRecord("foo", float64(4.131313131))))
	assert.Equal(t, "", extractContextString(jsonFormat, makeRecord()))
	assert.Equal(t, ",\"!BADKEY\":\"2\",\"!BADKEY\":\"3\"", extractContextString(jsonFormat, makeRecord(2, 3)))
}

func makeRecord(attrs ...interface{}) slog.Record {
	record := slog.Record{}
	record.Add(attrs...)
	return record
}
