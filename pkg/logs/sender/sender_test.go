// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package sender

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/logs/pb"
	"github.com/stretchr/testify/assert"
)

func NewTestSender() Sender {
	return Sender{nil, nil, nil, nil, nil, &LengthPrefix}
}

func TestToFrame(t *testing.T) {

	s := NewTestSender()

	raw1, err := (&pb.Log{}).Marshal()
	assert.Nil(t, err)

	frame1, err := s.toFrame(raw1)
	assert.Nil(t, err)
	assert.Equal(t, 4, len(frame1))
	assert.Equal(t, []byte{0x0, 0x0, 0x0, 0x0}, frame1[0:4])

	raw2, err := (&pb.Log{Message: "a"}).Marshal()
	assert.Nil(t, err)

	frame2, err := s.toFrame(raw2)
	assert.Nil(t, err)
	assert.Equal(t, 7, len(frame2))
	assert.Equal(t, []byte{0x0, 0x0, 0x0, 0x3}, frame2[0:4])

}
