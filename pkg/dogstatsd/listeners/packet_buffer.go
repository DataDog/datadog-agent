package listeners

import (
	"sync"
	"time"
)

const messageSeparator = byte('\n')

// packetBuffer merges multiple incoming datagrams into one "Packet" object to
// save space and make number of message in a single "Packet" more predictable
type packetBuffer struct {
	packet        *Packet
	packetLength  int
	pool          *PacketPool
	packetsBuffer *packetsBuffer
	flushTimer    *time.Ticker
	closeChannel  chan struct{}
	sync.Mutex
}

func newPacketBuffer(pool *PacketPool, flushTimer time.Duration, packetsBuffer *packetsBuffer) *packetBuffer {
	packetBuffer := &packetBuffer{
		packet:        pool.Get(),
		pool:          pool,
		packetsBuffer: packetsBuffer,
		flushTimer:    time.NewTicker(flushTimer),
		closeChannel:  make(chan struct{}),
	}
	go packetBuffer.flushLoop()
	return packetBuffer
}

func (p *packetBuffer) flushLoop() {
	for {
		select {
		case <-p.flushTimer.C:
			p.Lock()
			p.flush()
			p.Unlock()
		case <-p.closeChannel:
			return
		}
	}
}

func (p *packetBuffer) addMessage(message []byte) {
	p.Lock()
	if p.packetLength == 0 {
		p.packetLength = copy(p.packet.buffer, message)
	} else if len(p.packet.buffer) >= len(message)+p.packetLength+1 {
		p.packet.buffer[p.packetLength] = messageSeparator
		n := copy(p.packet.buffer[p.packetLength+1:], message)
		p.packetLength += n + 1
	} else {
		p.flush()
		p.packetLength = copy(p.packet.buffer, message)
	}
	p.Unlock()
}

func (p *packetBuffer) flush() {
	if p.packetLength == 0 {
		return
	}
	p.packet.Contents = p.packet.buffer[:p.packetLength]
	p.packetsBuffer.append(p.packet)
	p.packet = p.pool.Get()
	p.packetLength = 0
}

func (p *packetBuffer) close() {
	p.Lock()
	close(p.closeChannel)
	p.Unlock()
}
