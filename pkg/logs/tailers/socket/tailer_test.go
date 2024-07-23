// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package socket

import (
	"errors"
	"fmt"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
)

func TestReadAndForwardShouldSucceedWithSuccessfulRead(t *testing.T) {
	fmt.Println("wack")
	msgChan := make(chan *message.Message)
	r, w := net.Pipe()
	fmt.Println("wack1")
	tailer := NewTailer(sources.NewLogSource("", &config.LogsConfig{}), r, msgChan, read)
	fmt.Println("wack2")
	tailer.Start()
	fmt.Println("wack3")

	var msg *message.Message

	// should receive and decode one message

	w.Write([]byte("foo\n"))
	fmt.Println("wack4")
	msg = <-msgChan
	fmt.Println("wack4.5")
	assert.Equal(t, "foo", string(msg.GetContent()))
	fmt.Println("wack5")

	// should receive and decode two messages
	w.Write([]byte("bar\nboo\n"))
	msg = <-msgChan
	assert.Equal(t, "bar", string(msg.GetContent()))
	msg = <-msgChan
	assert.Equal(t, "boo", string(msg.GetContent()))

	tailer.Stop()
}

func TestReadShouldFailWithError(t *testing.T) {
	msgChan := make(chan *message.Message)
	r, w := net.Pipe()
	read := func(*Tailer) ([]byte, string, error) { return nil, "", errors.New("") }
	tailer := NewTailer(sources.NewLogSource("", &config.LogsConfig{}), r, msgChan, read)
	tailer.Start()

	w.Write([]byte("foo\n"))
	select {
	case <-msgChan:
		assert.Fail(t, "no data should return")
	default:
		break
	}

	tailer.Stop()
}

func read(tailer *Tailer) ([]byte, string, error) {
	inBuf := make([]byte, 4096)
	n, err := tailer.Conn.Read(inBuf)
	if err != nil {
		return nil, "", err
	}
	return inBuf[:n], "", nil
}
