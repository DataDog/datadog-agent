package listeners

import (
	"sync"
	"time"
)

type packetBuffer struct {
	packets       Packets
	flushTimer    *time.Ticker
	bufferSize    uint
	outputChannel chan Packets
	m             sync.Mutex
}

func newPacketBuffer(bufferSize uint, flushTimer time.Duration, outputChannel chan Packets) *packetBuffer {
	pb := &packetBuffer{
		bufferSize:    bufferSize,
		flushTimer:    time.NewTicker(flushTimer),
		outputChannel: outputChannel,
		packets:       make(Packets, 0, bufferSize),
	}
	go pb.flushLoop()
	return pb
}

func (pb *packetBuffer) flushLoop() {
	for {
		<-pb.flushTimer.C
		pb.m.Lock()
		pb.flush()
		pb.m.Unlock()
	}
}

func (pb *packetBuffer) append(packet *Packet) {
	pb.m.Lock()
	defer pb.m.Unlock()
	if uint(len(pb.packets)) == pb.bufferSize {
		pb.flush()
	}
	pb.packets = append(pb.packets, packet)

}

func (pb *packetBuffer) flush() {
	if len(pb.packets) > 0 {
		pb.outputChannel <- pb.packets
		pb.packets = make(Packets, 0, pb.bufferSize)
	}
}
