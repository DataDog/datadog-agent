// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package diagnostic

import (
	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

type DiagnosticReceiver struct {
	inputChan chan message.Message
	done      chan struct{}
}

func New() *DiagnosticReceiver {
	return &DiagnosticReceiver{
		inputChan: make(chan message.Message, 1),
		done:      make(chan struct{}),
	}
}

// func (d *DiagnosticReceiver) Start() chan string {
// 	outputChan := make(chan string)
// 	go d.run(outputChan)
// 	return outputChan
// }

func (d *DiagnosticReceiver) Stop() {
	d.done <- struct{}{}
}

func (d *DiagnosticReceiver) Close() {
	close(d.inputChan)
}

// // run starts the processing of the inputChan
// func (d *DiagnosticReceiver) run(outputChan chan string) {
// 	for {
// 		select {
// 		case <-d.done:
// 			break
// 		case msg := <-d.inputChan:
// 			outputChan <- string(msg.Content)
// 		}
// 	}
// 	// for msg := range d.inputChan {
// 	// 	fmt.Println(string(msg.Content))
// 	// }
// }

func (d *DiagnosticReceiver) Next() (line string, ok bool) {
	// msg := <-d.inputChan
	// return string(msg.Content), true
	select {
	case msg := <-d.inputChan:
		return string(msg.Content), true
	default:
		return "", false
	}

}

func (d *DiagnosticReceiver) Channel() chan message.Message {
	return d.inputChan
}
