// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package log

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/util/log/slog/formatters"
)

func TestExtractContextString(t *testing.T) {
	assert.Equal(t, `,"foo":"bar"`, formatters.ExtraJSONContext(toAttrHolder([]interface{}{"foo", "bar"})))
	assert.Equal(t, `foo:bar | `, formatters.ExtraTextContext(toAttrHolder([]interface{}{"foo", "bar"})))
	assert.Equal(t, `,"foo":"bar","bar":"buzz"`, formatters.ExtraJSONContext(toAttrHolder([]interface{}{"foo", "bar", "bar", "buzz"})))
	assert.Equal(t, `foo:bar,bar:buzz | `, formatters.ExtraTextContext(toAttrHolder([]interface{}{"foo", "bar", "bar", "buzz"})))
	assert.Equal(t, `,"foo":"b\"a\"r"`, formatters.ExtraJSONContext(toAttrHolder([]interface{}{"foo", "b\"a\"r"})))
	assert.Equal(t, `,"foo":"3"`, formatters.ExtraJSONContext(toAttrHolder([]interface{}{"foo", 3})))
	assert.Equal(t, `,"foo":"4.131313131"`, formatters.ExtraJSONContext(toAttrHolder([]interface{}{"foo", float64(4.131313131)})))
	assert.Equal(t, "", formatters.ExtraJSONContext(toAttrHolder(nil)))
	assert.Equal(t, "", formatters.ExtraJSONContext(toAttrHolder([]interface{}{2, 3})))
	assert.Equal(t, `,"foo":"bar","bar":"buzz"`, formatters.ExtraJSONContext(toAttrHolder([]interface{}{"foo", "bar", 2, 3, "bar", "buzz"})))
}
