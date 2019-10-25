package dogstatsd

// Packet represents a statsd packet ready to process,
// with its origin metadata if applicable.
//
// As the Packet object is reused in a sync.Pool, we keep the
// underlying buffer reference to avoid re-sizing the slice
// before reading
type Packet struct {
	Contents       []byte // Contents, might contain several messages
	buffer         []byte // Underlying buffer for data read
	Origin         string // Origin container if identified
	referenceCount int
	pool           *PacketPool
}

func (p *Packet) release() {
	p.referenceCount--
	if p.referenceCount < 1 && p.pool != nil {
		p.pool.Put(p)
	}
}

func (p *Packet) borrow() {
	p.referenceCount++
}

// Packets is a slice of packet pointers
type Packets []*Packet
