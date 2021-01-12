// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package diagnostic

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

type DiagnosticReceiver struct {
	inputChan chan message.Message
}

func New() *DiagnosticReceiver {
	return &DiagnosticReceiver{
		inputChan: make(chan message.Message, config.ChanSize),
	}
}

func (d *DiagnosticReceiver) Start() {
	go d.run()
}

func (d *DiagnosticReceiver) Stop() {
	close(d.inputChan)
}

// run starts the processing of the inputChan
func (d *DiagnosticReceiver) run() {
	for msg := range d.inputChan {
		fmt.Println(string(msg.Content))
	}
}

func (d *DiagnosticReceiver) Channel() chan message.Message {
	return d.inputChan
}
