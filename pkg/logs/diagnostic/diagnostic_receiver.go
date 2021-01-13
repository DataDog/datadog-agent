// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package diagnostic

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

type DiagnosticReceiver struct {
	inputChan chan message.Message
	done      chan struct{}
}

func New() *DiagnosticReceiver {
	return &DiagnosticReceiver{
		inputChan: make(chan message.Message, 100),
	}
}

func (d *DiagnosticReceiver) Stop() {
	close(d.inputChan)
}

func (d *DiagnosticReceiver) Clear() {
	fmt.Println("brian: Clear " + string(len(d.inputChan)))
	l := len(d.inputChan)
	for i := 0; i < l; i++ {
		<-d.inputChan
	}
}

func (d *DiagnosticReceiver) Next() (line string, ok bool) {
	select {
	case msg := <-d.inputChan:
		return formatMessage(&msg), true
	default:
		return "", false
	}
}

func (d *DiagnosticReceiver) Channel() chan message.Message {
	return d.inputChan
}

func formatMessage(m *message.Message) string {
	return fmt.Sprintf("%s | %s | %s", m.Origin.Source(), m.Origin.LogSource.Config.Type, string(m.Content))
}
