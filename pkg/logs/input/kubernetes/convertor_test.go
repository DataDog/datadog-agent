// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package kubernetes

import (
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	iParser "github.com/DataDog/datadog-agent/pkg/logs/parser"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestConvertorConvert(t *testing.T) {
	convertor := &Convertor{}
	prefix := iParser.Prefix{
		Timestamp: "2018-09-20T11:54:11.753589172Z",
		Status:    message.StatusAlert,
	}
	// normal case
	line := convertor.Convert([]byte("2018-09-20T11:54:11.753589143Z stdout F msg"), prefix)
	assert.Equal(t, "msg", string(line.Content))
	assert.Equal(t, 3, line.Size)
	assert.Equal(t, "info", line.Status)
	assert.Equal(t, "2018-09-20T11:54:11.753589143Z", line.Timestamp)

	// length mismatch
	line = convertor.Convert([]byte("partial msg"), prefix)
	assert.Equal(t, "partial msg", string(line.Content))
	assert.Equal(t, 11, line.Size)
	assert.Equal(t, message.StatusAlert, line.Status)
	assert.Equal(t, "2018-09-20T11:54:11.753589172Z", line.Timestamp)

	// invalid timestamp, treat as partial log
	line = convertor.Convert([]byte("2018-09-20T11-54-11.753589143Z stdout F msg"), prefix)
	assert.Equal(t, "2018-09-20T11-54-11.753589143Z stdout F msg", string(line.Content))
	assert.Equal(t, 43, line.Size)
	assert.Equal(t, message.StatusAlert, line.Status)
	assert.Equal(t, "2018-09-20T11:54:11.753589172Z", line.Timestamp)

	// missing status, treat as partial log
	line = convertor.Convert([]byte("2018-09-20T11:54:11.753589143Z std F msg"), prefix)
	assert.Equal(t, "2018-09-20T11:54:11.753589143Z std F msg", string(line.Content))
	assert.Equal(t, 40, line.Size)
	assert.Equal(t, message.StatusAlert, line.Status)
	assert.Equal(t, "2018-09-20T11:54:11.753589172Z", line.Timestamp)

	// wrong flag, treat as partial log
	line = convertor.Convert([]byte("2018-09-20T11:54:11.753589143Z stdout H msg"), prefix)
	assert.Equal(t, "2018-09-20T11:54:11.753589143Z stdout H msg", string(line.Content))
	assert.Equal(t, 43, line.Size)
	assert.Equal(t, message.StatusAlert, line.Status)
	assert.Equal(t, "2018-09-20T11:54:11.753589172Z", line.Timestamp)

	// no content
	line = convertor.Convert([]byte("2018-09-20T11:54:11.753589143Z stdout F"), prefix)
	assert.Nil(t, line)
}
