package listeners

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// copy of aggregator.MetricSamplePoolBatchSize to avoid cycling import
const sampleBatchSize = 32

func buildPacketAssembler() (*packetAssembler, chan Packets) {
	out := make(chan Packets, 16)
	psb := newPacketsBuffer(1, 1*time.Hour, out)
	pb := newPacketAssembler(100*time.Millisecond, psb, NewPacketPool(sampleBatchSize))
	return pb, out
}

func generateRandomPacket(size uint) []byte {
	garbage := make([]byte, size)
	j := 0
	for i := range garbage {
		garbage[i] = byte(65 + j)
		j++
		if j > 25 {
			j = 0
		}
	}
	return garbage
}

func TestPacketBufferTimeout(t *testing.T) {
	pb, out := buildPacketAssembler()
	message := []byte("test")

	pb.addMessage(message)

	packets := <-out
	assert.Len(t, packets, 1)
	assert.Equal(t, message, packets[0].Contents)
}

func TestPacketBufferMerge(t *testing.T) {
	pb, out := buildPacketAssembler()
	message1 := []byte("test1")
	message2 := []byte("test2")

	pb.addMessage(message1)
	pb.addMessage(message2)

	packets := <-out
	assert.Len(t, packets, 1)
	assert.Equal(t, []byte("test1\ntest2"), packets[0].Contents)
}

func TestPacketBufferMergeMaxSize(t *testing.T) {
	pb, out := buildPacketAssembler()
	message1 := []byte("12345678")
	message2 := []byte("1234567")

	pb.addMessage(message1)
	pb.addMessage(message2)

	packets := <-out
	assert.Len(t, packets, 1)
	assert.Equal(t, []byte("12345678\n1234567"), packets[0].Contents)
}

func TestPacketBufferOverflow(t *testing.T) {
	pb, out := buildPacketAssembler()
	// generate a message exactly of the size of the buffer of the packet assembler
	// to fill it completely
	message1 := generateRandomPacket(sampleBatchSize)
	message2 := []byte("12345678")

	pb.addMessage(message1)
	pb.addMessage(message2)

	packets1 := <-out
	packets2 := <-out
	assert.Len(t, packets1, 1)
	assert.Len(t, packets2, 1)
	assert.Equal(t, message1, packets1[0].Contents)
	assert.Equal(t, message2, packets2[0].Contents)
}

func TestPacketBufferMergePlusOverflow(t *testing.T) {
	pb, out := buildPacketAssembler()
	message1 := generateRandomPacket(sampleBatchSize / 2)
	message2 := generateRandomPacket((sampleBatchSize / 2) - 1)
	message3 := []byte("Z")

	pb.addMessage(message1)
	pb.addMessage(message2)
	pb.addMessage(message3)

	packets1 := <-out
	packets2 := <-out
	assert.Len(t, packets1, 1)
	assert.Len(t, packets2, 1)
	assert.Equal(t, []byte(fmt.Sprintf("%s\n%s", message1, message2)), packets1[0].Contents)
	assert.Equal(t, []byte("Z"), packets2[0].Contents)
}

func TestPacketBufferEmpty(t *testing.T) {
	pb, out := buildPacketAssembler()
	message1 := []byte("")
	message2 := []byte("test2")

	pb.addMessage(message1)
	pb.addMessage(message2)

	packets := <-out
	assert.Len(t, packets, 1)
	assert.Equal(t, []byte("test2"), packets[0].Contents)
}

func TestPacketBufferEmptySecond(t *testing.T) {
	pb, out := buildPacketAssembler()
	message1 := []byte("test1")
	message2 := []byte("")

	pb.addMessage(message1)
	pb.addMessage(message2)

	packets := <-out
	assert.Len(t, packets, 1)
	assert.Equal(t, []byte("test1\n"), packets[0].Contents)
}

func BenchmarkPacketsBufferFlush(b *testing.B) {
	packet := generateRandomPacket(4)

	for i := 0; i < b.N; i++ {
		pb, out := buildPacketAssembler()

		for i := 0; i < 100; i++ {
			pb.addMessage(packet)

			// let's empty the packets channel to make sure it is not blocking
			for len(out) > 0 {
				<-out
			}
		}
	}
}
