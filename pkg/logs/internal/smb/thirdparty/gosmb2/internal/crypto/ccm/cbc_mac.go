package ccm

import (
	"crypto/cipher"
	"crypto/subtle"
)

// CBC-MAC implementation
type mac struct {
	ci []byte
	p  int
	c  cipher.Block
}

func newMAC(c cipher.Block) *mac {
	return &mac{
		c:  c,
		ci: make([]byte, c.BlockSize()),
	}
}

func (m *mac) Reset() {
	clear(m.ci)
	m.p = 0
}

func (m *mac) Write(p []byte) (n int, err error) {
	n = len(p)
	for len(p) > 0 {
		if m.p >= len(m.ci) {
			m.c.Encrypt(m.ci, m.ci)
			m.p = 0
		}
		written := subtle.XORBytes(m.ci[m.p:], m.ci[m.p:], p)
		m.p += written
		p = p[written:]
	}
	return n, nil
}

// PadZero emulates zero byte padding.
func (m *mac) PadZero() {
	if m.p != 0 {
		m.c.Encrypt(m.ci, m.ci)
		m.p = 0
	}
}

func (m *mac) Sum(in []byte) []byte {
	return append(in, m.ci...)
}

func (m *mac) Size() int { return len(m.ci) }

func (m *mac) BlockSize() int { return 16 }
