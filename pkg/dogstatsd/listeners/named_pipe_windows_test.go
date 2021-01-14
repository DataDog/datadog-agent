// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.
// +build windows

package listeners

import (
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/Microsoft/go-winio"
	"github.com/stretchr/testify/assert"
)

const pipeName = "TestPipeName"
const maxPipeMessageCount = 1000
const namedPipeBufferSize = 13

func TestNamedPipeListen(t *testing.T) {
	listener := newNamedPipeListenerTest(t)
	defer listener.Stop()
	client := createNamedPipeClient(t)
	defer client.Close()

	res := sendAndGetNamedPipeMessage(t, listener, client, "data\n")
	assert.Equal(t, "data", res)

}

func TestNamedPipeSeveralClients(t *testing.T) {
	listener := newNamedPipeListenerTest(t)
	defer listener.Stop()
	for i := 0; i < 3; i++ {
		client := createNamedPipeClient(t)
		_, err := client.Write([]byte(fmt.Sprintf("client %d\n", i)))
		assert.NoError(t, err)

		// `Close` does not flush the previous write.
		// Wait to make sure the write is flushed before closing.
		time.Sleep(100 * time.Millisecond)
		client.Close()
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

	res := sendAndGetNamedPipeMessage(t, listener, client, "data\n")
	assert.Equal(t, "data", res)
}

func TestNamedPipeStop(t *testing.T) {
	listener := newNamedPipeListenerTest(t)
	client := createNamedPipeClient(t)

	res := sendAndGetNamedPipeMessage(t, listener, client, "data\n")
	assert.Equal(t, "data", res)
	assert.Equal(t, int32(1), listener.getActiveConnectionsCount())

	client.Close()
	listener.Stop()

	assert.Equal(t, int32(0), listener.getActiveConnectionsCount())
}

func TestNamedPipeMultipleMessages(t *testing.T) {
	listener := newNamedPipeListenerTest(t)
	defer listener.Stop()
	client := createNamedPipeClient(t)
	defer client.Close()

	nbMessage := maxPipeMessageCount
	for i := 0; i < nbMessage; i++ {
		v := fmt.Sprintf("data%d\n", i)
		client.Write([]byte(v))
	}
	messages := getNamedPipeMessages(t, listener, nbMessage)
	for i := 0; i < nbMessage; i++ {
		expectedMessage := fmt.Sprintf("data%d", i)
		assert.True(t, messages[expectedMessage])
	}
}

func TestNamedPipeTooBigMessage(t *testing.T) {
	listener := newNamedPipeListenerTest(t)
	defer listener.Stop()
	client := createNamedPipeClient(t)
	defer client.Close()

	message := strings.Repeat("1", namedPipeBufferSize+3) + "\n"
	client.Write([]byte(message))
	client.Write([]byte("data\n"))

	messages := getNamedPipeMessages(t, listener, 2)
	assert.True(t, messages["data"])
}

type namedPipeListenerTest struct {
	*NamedPipeListener
	packetOut chan Packets
	client    net.Conn
}

func newNamedPipeListenerTest(t *testing.T) namedPipeListenerTest {
	pool := NewPacketPool(maxPipeMessageCount)
	packetOut := make(chan Packets, maxPipeMessageCount)
	packetManager := newPacketManager(10, maxPipeMessageCount, 10*time.Millisecond, packetOut, pool)

	listener, err := newNamedPipeListener(
		pipeName,
		namedPipeBufferSize,
		packetManager)
	assert.NoError(t, err)

	listenerTest := namedPipeListenerTest{
		NamedPipeListener: listener,
		packetOut:         packetOut,
	}

	go listenerTest.Listen()
	return listenerTest
}

func sendAndGetNamedPipeMessage(t *testing.T, listener namedPipeListenerTest, client net.Conn, str string) string {
	_, err := client.Write([]byte(str))
	assert.NoError(t, err)
	return getNamedPipeMessage(t, listener)
}

func getNamedPipeMessage(t *testing.T, listener namedPipeListenerTest) string {
	messages := readNamedPipeMessagesFromChan(listener.packetOut)
	assert.Equal(t, 1, len(messages))
	return messages[0]
}

func getNamedPipeMessages(t *testing.T, listener namedPipeListenerTest, nbMessage int) map[string]bool {
	messageSet := make(map[string]bool)

	for len(messageSet) < nbMessage {

		messages := readNamedPipeMessagesFromChan(listener.packetOut)
		for _, m := range messages {
			messageSet[m] = true
		}
	}
	return messageSet
}

func readNamedPipeMessagesFromChan(packetOut chan Packets) []string {
	var messages []string
	packets := <-packetOut
	for _, packet := range packets {
		newMessages := strings.FieldsFunc(string(packet.Contents), func(c rune) bool { return c == '\n' })
		messages = append(messages, newMessages...)
	}
	return messages
}

func createNamedPipeClient(t *testing.T) net.Conn {
	client, err := winio.DialPipe(pipeNamePrefix+pipeName, nil)
	assert.NoError(t, err)
	return client
}
