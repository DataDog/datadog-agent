// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build docker

package docker

import (
	"testing"

	parser "github.com/DataDog/datadog-agent/pkg/logs/docker"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/stretchr/testify/suite"
)

type TailerTestSuite struct {
	suite.Suite
	tailer *Tailer
}

func (suite *TailerTestSuite) SetupTest() {
	suite.tailer = &Tailer{}
}

func (suite *TailerTestSuite) TestTailerRemovesDate() {
	msgMeta := [8]byte{}
	msgMeta[0] = 1
	// https://godoc.org/github.com/moby/moby/client#Client.ContainerLogs
	// next bytes represent the size of the content
	msgMeta[5] = '>'
	msgMeta[6] = '1'
	msgMeta[7] = 'g'

	msg := []byte{}
	for i := 0; i < len(msgMeta); i++ {
		msg = append(msg, msgMeta[i])
	}
	msg = append(msg, []byte("2007-01-12T01:01:01.000000000Z my message")...)
	dockerMsg, err := parser.ParseMessage(msg)
	suite.Nil(err)
	suite.Equal("my message", string(dockerMsg.Content))
	suite.Equal(message.StatusInfo, dockerMsg.Status)
	suite.Equal("2007-01-12T01:01:01.000000000Z", dockerMsg.Timestamp)

	msgMeta[0] = 2
	msg = []byte{}
	for i := 0; i < len(msgMeta); i++ {
		msg = append(msg, msgMeta[i])
	}
	msg = append(msg, []byte("2008-01-12T01:01:01.000000000Z my error")...)
	dockerMsg, err = parser.ParseMessage(msg)
	suite.Nil(err)
	suite.Equal("my error", string(dockerMsg.Content))
	suite.Equal(message.StatusError, dockerMsg.Status)
	suite.Equal("2008-01-12T01:01:01.000000000Z", dockerMsg.Timestamp)
}

func (suite *TailerTestSuite) TestTailerNextLogSinceDate() {
	suite.Equal("2008-01-12T01:01:01.000000001Z", suite.tailer.nextLogSinceDate("2008-01-12T01:01:01.000000000Z"))
	suite.Equal("2008-01-12T01:01:01.anything", suite.tailer.nextLogSinceDate("2008-01-12T01:01:01.anything"))
	suite.Equal("", suite.tailer.nextLogSinceDate(""))
}

func (suite *TailerTestSuite) TestTailerIdentifier() {
	suite.tailer.ContainerID = "test"
	suite.Equal("docker:test", suite.tailer.Identifier())
}

func TestTailerTestSuite(t *testing.T) {
	suite.Run(t, new(TailerTestSuite))
}
