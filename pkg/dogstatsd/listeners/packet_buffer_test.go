package listeners

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func buildPacketBuffer(buffersSize int) (*packetBuffer, chan Packets) {
	pool := NewPacketPool(buffersSize)
	out := make(chan Packets, 16)
	psb := newPacketsBuffer(1, 1*time.Hour, out)
	pb := newPacketBuffer(pool, 100*time.Millisecond, psb)
	return pb, out
}

func TestPacketBufferTimeout(t *testing.T) {
	pb, out := buildPacketBuffer(16)
	message := []byte("test")

	pb.addMessage(message)

	packets := <-out
	assert.Len(t, packets, 1)
	assert.Equal(t, message, packets[0].Contents)
}

func TestPacketBufferMerge(t *testing.T) {
	pb, out := buildPacketBuffer(16)
	message1 := []byte("test1")
	message2 := []byte("test2")

	pb.addMessage(message1)
	pb.addMessage(message2)

	packets := <-out
	assert.Len(t, packets, 1)
	assert.Equal(t, []byte("test1\ntest2"), packets[0].Contents)
}

func TestPacketBufferMergeMaxSize(t *testing.T) {
	pb, out := buildPacketBuffer(16)
	message1 := []byte("12345678")
	message2 := []byte("1234567")

	pb.addMessage(message1)
	pb.addMessage(message2)

	packets := <-out
	assert.Len(t, packets, 1)
	assert.Equal(t, []byte("12345678\n1234567"), packets[0].Contents)
}

func TestPacketBufferOverflow(t *testing.T) {
	pb, out := buildPacketBuffer(16)
	message1 := []byte("12345678")
	message2 := []byte("12345678")

	pb.addMessage(message1)
	pb.addMessage(message2)

	packets1 := <-out
	packets2 := <-out
	assert.Len(t, packets1, 1)
	assert.Len(t, packets2, 1)
	assert.Equal(t, []byte("12345678"), packets1[0].Contents)
	assert.Equal(t, []byte("12345678"), packets2[0].Contents)
}

func TestPacketBufferMergePlusOverflow(t *testing.T) {
	pb, out := buildPacketBuffer(16)
	message1 := []byte("12345678")
	message2 := []byte("1234567")
	message3 := []byte("1")

	pb.addMessage(message1)
	pb.addMessage(message2)
	pb.addMessage(message3)

	packets1 := <-out
	packets2 := <-out
	assert.Len(t, packets1, 1)
	assert.Len(t, packets2, 1)
	assert.Equal(t, []byte("12345678\n1234567"), packets1[0].Contents)
	assert.Equal(t, []byte("1"), packets2[0].Contents)
}

func TestPacketBufferEmpty(t *testing.T) {
	pb, out := buildPacketBuffer(16)
	message1 := []byte("")
	message2 := []byte("test2")

	pb.addMessage(message1)
	pb.addMessage(message2)

	packets := <-out
	assert.Len(t, packets, 1)
	assert.Equal(t, []byte("test2"), packets[0].Contents)
}

func TestPacketBufferEmptySecond(t *testing.T) {
	pb, out := buildPacketBuffer(16)
	message1 := []byte("test1")
	message2 := []byte("")

	pb.addMessage(message1)
	pb.addMessage(message2)

	packets := <-out
	assert.Len(t, packets, 1)
	assert.Equal(t, []byte("test1\n"), packets[0].Contents)
}
