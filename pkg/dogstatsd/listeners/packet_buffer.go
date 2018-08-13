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
	closeChannel  chan struct{}
	m             sync.Mutex
}

func newPacketBuffer(bufferSize uint, flushTimer time.Duration, outputChannel chan Packets) *packetBuffer {
	pb := &packetBuffer{
		bufferSize:    bufferSize,
		flushTimer:    time.NewTicker(flushTimer),
		outputChannel: outputChannel,
		packets:       make(Packets, 0, bufferSize),
		closeChannel:  make(chan struct{}),
	}
	go pb.flushLoop()
	return pb
}

func (pb *packetBuffer) flushLoop() {
	for {
		select {
		case <-pb.flushTimer.C:
			pb.m.Lock()
			pb.flush()
			pb.m.Unlock()
		case <-pb.closeChannel:
			return
		}
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

func (pb *packetBuffer) close() {
	close(pb.closeChannel)
}
