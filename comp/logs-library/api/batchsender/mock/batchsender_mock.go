// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

// Package mock provides a mock implementation of the batch sender component
package mock

import (
	"github.com/DataDog/datadog-agent/comp/logs-library/api/batchsender/def"
	"github.com/DataDog/datadog-agent/comp/logs-library/client"
	"github.com/DataDog/datadog-agent/comp/logs-library/config"
	"github.com/DataDog/datadog-agent/comp/logs-library/message"
)

// ProvidesMock is the mock component output
type ProvidesMock struct {
	Comp def.FactoryComponent
}

func NewMock() ProvidesMock {
	return ProvidesMock{
		Comp: &MockBatchSenderFactory{},
	}
}

type MockBatchSender struct {
	inputChan chan *message.Message
}

func (s *MockBatchSender) Start() {
	go func() {
		for range s.inputChan {
		}
	}()
}

func (s *MockBatchSender) Stop() {
	close(s.inputChan)
}

func (s *MockBatchSender) GetInputChan() chan *message.Message {
	return s.inputChan
}

type MockBatchSenderFactory struct{}

func (f *MockBatchSenderFactory) NewBatchSender(
	endpoints *config.Endpoints,
	destinationsContext *client.DestinationsContext,
	eventType string,
	contentType string,
	category string,
	disableBatching bool,
	pipelineID int,
) def.BatchSender {
	return &MockBatchSender{
		inputChan: make(chan *message.Message, 10),
	}
}
