// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.
// +build windows

package listeners

import (
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/Microsoft/go-winio"
	"github.com/stretchr/testify/assert"
)

const pipeName = "TestPipeName"

func TestNamedPipeListen(t *testing.T) {
	listener := newNamedPipeListenerTest(t)
	defer listener.Stop()
	client := createNamedPipeClient(t)
	defer client.Close()

	res := sendAndGetNamedPipeMessage(t, listener, client, "data")
	assert.Equal(t, "data", res)

}

func TestNamedPipeSeveralClients(t *testing.T) {
	listener := newNamedPipeListenerTest(t)
	defer listener.Stop()
	for i := 0; i < 3; i++ {
		client := createNamedPipeClient(t)
		defer client.Close()
		_, err := client.Write([]byte(fmt.Sprintf("client %d", i)))
		assert.NoError(t, err)
	}

	messages := getNamedPipeMessages(t, listener, 3)
	assert.Equal(t, 3, len(messages))
	assert.True(t, messages["client 0"])
	assert.True(t, messages["client 1"])
	assert.True(t, messages["client 2"])
}

func TestNamedPipeReconnection(t *testing.T) {
	listener := newNamedPipeListenerTest(t)
	defer listener.Stop()
	for i := 0; i < 2; i++ {
		client := createNamedPipeClient(t)
		client.Close()
	}
	client := createNamedPipeClient(t)
	defer client.Close()

	res := sendAndGetNamedPipeMessage(t, listener, client, "data")
	assert.Equal(t, "data", res)
}

func TestNamedPipeStop(t *testing.T) {
	listener := newNamedPipeListenerTest(t)
	client := createNamedPipeClient(t)

	res := sendAndGetNamedPipeMessage(t, listener, client, "data")
	assert.Equal(t, "data", res)
	assert.Equal(t, 1, listener.GetActiveConnectionsCount())
	client.Close()
	listener.Stop()

	// Wait listener is really stopped
	<-listener.listenStop
	for listener.GetActiveConnectionsCount() > 1 {
		time.Sleep(100 * time.Millisecond)
	}
}

type namedPipeListenerTest struct {
	*NamedPipeListener
	packetOut  chan Packets
	client     net.Conn
	listenStop chan bool
}

func newNamedPipeListenerTest(t *testing.T) namedPipeListenerTest {
	pool := NewPacketPool(10)
	packetOut := make(chan Packets)
	packetManager := newPacketManager(100, 1, 10*time.Millisecond, packetOut, pool)

	listener, err := newNamedPipeListener(
		pipeName,
		100,
		packetManager)
	assert.NoError(t, err)

	listenStop := make(chan bool)
	listenerTest := namedPipeListenerTest{
		NamedPipeListener: listener,
		packetOut:         packetOut,
		listenStop:        listenStop,
	}

	go func() {
		listenerTest.Listen()
		listenStop <- true
	}()
	return listenerTest
}

func sendAndGetNamedPipeMessage(t *testing.T, listener namedPipeListenerTest, client net.Conn, str string) string {
	_, err := client.Write([]byte(str))
	assert.NoError(t, err)
	return getNamedPipeMessage(t, listener)
}

func getNamedPipeMessage(t *testing.T, listener namedPipeListenerTest) string {
	res := <-listener.packetOut
	assert.Equal(t, 1, len(res))
	return string(res[0].Contents)
}

func getNamedPipeMessages(t *testing.T, listener namedPipeListenerTest, nbMessage int) map[string]bool {
	messages := make(map[string]bool)

	for nbMessage > 0 {
		message := getNamedPipeMessage(t, listener)
		messages[message] = true
		nbMessage--
	}
	return messages
}

func createNamedPipeClient(t *testing.T) net.Conn {
	client, err := winio.DialPipe(pipeNamePrefix+pipeName, nil)
	assert.NoError(t, err)
	return client
}
