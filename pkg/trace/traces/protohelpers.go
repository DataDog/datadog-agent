package traces

// TODO: This thing sucks. Refactor this to something more generic, can add some functionality to molecule
// possibly?
type protoEncoder struct {
	buf []byte
}

func newProtoEncoder() *protoEncoder {
	// TODO: Presize buf.
	return &protoEncoder{}
}

func (p *protoEncoder) Write(b []byte) (int, error) {
	p.buf = append(p.buf, b...)
	return len(b), nil
}

func (p *protoEncoder) encodeTagAndWireType(tag int32, wireType int8) {
	v := uint64((int64(tag) << 3) | int64(wireType))
	p.encodeVarint(v)
}

func (p *protoEncoder) encodeVarint(x uint64) {
	for x >= 1<<7 {
		p.buf = append(p.buf, uint8(x&0x7f|0x80))
		x >>= 7
	}
	p.buf = append(p.buf, uint8(x))
}

func (p *protoEncoder) encodeFixed64(x uint64) error {
	p.buf = append(p.buf,
		uint8(x),
		uint8(x>>8),
		uint8(x>>16),
		uint8(x>>24),
		uint8(x>>32),
		uint8(x>>40),
		uint8(x>>48),
		uint8(x>>56))
	return nil
}

func (p *protoEncoder) encodeRawBytes(b []byte) {
	p.encodeVarint(uint64(len(b)))
	p.buf = append(p.buf, b...)
}

func (p *protoEncoder) reset(b []byte) {
	p.buf = b
}
