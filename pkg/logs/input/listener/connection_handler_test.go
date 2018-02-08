// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package listener

import (
	"net"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
	pipeline "github.com/DataDog/datadog-agent/pkg/logs/pipeline/mock"
)

type ConnectionHandlerTestSuite struct {
	suite.Suite

	h *ConnectionHandler
}

func (suite *ConnectionHandlerTestSuite) SetupTest() {
	pp := pipeline.NewMockProvider()
	source := config.NewLogSource("", &config.LogsConfig{})
	suite.h = NewConnectionHandler(pp, source)
}

func (suite *ConnectionHandlerTestSuite) TestCreateAndCheckWorkers() {
	h := suite.h
	suite.Equal(0, len(h.workers))

	var r net.Conn

	// a new worker should be created
	r, _ = net.Pipe()
	h.createWorker(r)
	suite.Equal(1, len(h.workers))

	// a new worker should be created
	r, _ = net.Pipe()
	h.createWorker(r)
	suite.Equal(2, len(h.workers))

	// the number of workers should not change
	w := h.workers[0]
	w.shouldStop = true
	suite.Equal(2, len(h.workers))

	// one worker should be deleted
	h.checkWorkers()
	suite.Equal(1, len(h.workers))
}

func (suite *ConnectionHandlerTestSuite) TestLifeCyle() {
	h := suite.h

	// no work should be up and running
	h.Start()
	suite.Equal(0, len(h.workers))

	r, _ := net.Pipe()
	h.HandleConnection(r)

	// all workers should stopped and released
	h.Stop()
	suite.Equal(0, len(h.workers))
}

func TestConnectionHandlerTestSuite(t *testing.T) {
	suite.Run(t, new(ConnectionHandlerTestSuite))
}
