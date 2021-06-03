// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package replay

import (
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestReader(t *testing.T) {
	// well-formed input file
	tc, err := NewTrafficCaptureReader("resources/test/datadog-capture.dog", 1)
	assert.Nil(t, err)
	assert.NotNil(t, tc)

	// read state from file
	pidMap, entityMap, err := tc.ReadState()
	assert.Nil(t, err)
	assert.NotNil(t, pidMap)
	assert.NotNil(t, entityMap)

	// advance the offset to where the packets start
	tc.Lock()
	tc.offset += uint32(len(datadogHeader))
	tc.Unlock()

	cnt := 1
	for msg, err := tc.ReadNext(); err != io.EOF; msg, err = tc.ReadNext() {
		if err == io.EOF {
			assert.Nil(t, msg)
		} else {
			assert.Nil(t, err)
			assert.NotNil(t, msg)
		}
		cnt++
	}

	// 22 metrics in the capture
	assert.Equal(t, 22, cnt)

}
