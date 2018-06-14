// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package docker

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseMessage(t *testing.T) {
	preMessage := "100000002018-06-14T18:27:03.246999277Z "
	longLog := []byte(preMessage + strings.Repeat("a", 16*1024) + preMessage + strings.Repeat("b", 50))
	msgLength := len(strings.Repeat("a", 16*1024) + strings.Repeat("b", 50))
	ts, status, msg, err := ParseMessage(longLog)
	assert.NotNil(t, ts)
	assert.NotNil(t, status)
	assert.NotNil(t, msg)
	assert.Nil(t, err)
	assert.Equal(t, msgLength, len(msg))

	msg = []byte{}
	msg = append(msg, []byte{1, 0, 0, 0, 0}...)
	_, _, _, err = ParseMessage(msg)
	assert.Equal(t, errors.New("Can't parse docker message: expected a 8 bytes header"), err)

	msg = []byte{}
	msg = append(msg, []byte{1, 0, 0, 0, 0, 62, 49, 103}...)
	msg = append(msg, []byte("INFO_10:26:31_Loading_settings_from_file:/etc/cassandra/cassandra.yaml")...)

	_, _, _, err = ParseMessage(msg)
	assert.Equal(t, errors.New("Can't parse docker message: expected a whitespace after header"), err)

}