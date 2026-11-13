// CCM Mode, defined in
// NIST Special Publication SP 800-38C.

package ccm

import (
	"crypto/cipher"
	"crypto/subtle"
	"errors"
	"sync"
)

type ccm struct {
	c         cipher.Block
	macPool   sync.Pool
	nonceSize int
	tagSize   int
}

// NewCCMWithNonceAndTagSizes returns the given 128-bit, block cipher wrapped in Counter with CBC-MAC Mode, which accepts nonces of the given length.
// the formatting of this function is defined in SP800-38C, Appendix A.
// Each arguments have own valid range:
//
//	nonceSize should be one of the {7, 8, 9, 10, 11, 12, 13}.
//	tagSize should be one of the {4, 6, 8, 10, 12, 14, 16}.
//	Otherwise, it panics.
//
// The maximum payload size is defined as 1<<uint((15-nonceSize)*8)-1.
// If the given payload size exceeds the limit, it returns a error (Seal returns nil instead).
// The payload size is defined as len(plaintext) on Seal, len(ciphertext)-tagSize on Open.
func NewCCMWithNonceAndTagSizes(c cipher.Block, nonceSize, tagSize int) (cipher.AEAD, error) {
	if c.BlockSize() != 16 {
		return nil, errors.New("cipher: CCM mode requires 128-bit block cipher")
	}

	if !(7 <= nonceSize && nonceSize <= 13) {
		return nil, errors.New("cipher: invalid nonce size")
	}

	if !(4 <= tagSize && tagSize <= 16 && tagSize&1 == 0) {
		return nil, errors.New("cipher: invalid tag size")
	}

	return &ccm{
		c: c,
		macPool: sync.Pool{
			New: func() any {
				return newMAC(c)
			},
		},
		nonceSize: nonceSize,
		tagSize:   tagSize,
	}, nil
}

func (ccm *ccm) NonceSize() int {
	return ccm.nonceSize
}

func (ccm *ccm) Overhead() int {
	return ccm.tagSize
}

func (ccm *ccm) Seal(dst, nonce, plaintext, data []byte) []byte {
	if len(nonce) != ccm.nonceSize {
		panic("cipher: incorrect nonce length given to CCM")
	}

	// AEAD interface doesn't provide a way to return errors.
	// So it returns nil instead.
	if maxUvarint(15-ccm.nonceSize) < uint64(len(plaintext)) {
		return nil
	}

	ret, ciphertext := sliceForAppend(dst, len(plaintext)+16)

	// Formatting of the Counter Blocks are defined in A.3.
	Ctr := make([]byte, 16)               // Ctr0
	Ctr[0] = byte(15 - ccm.nonceSize - 1) // [q-1]3
	copy(Ctr[1:], nonce)                  // N

	S0 := ciphertext[len(plaintext):] // S0
	ccm.c.Encrypt(S0, Ctr)

	Ctr[15] = 1 // Ctr1

	ctr := cipher.NewCTR(ccm.c, Ctr)

	// Authenticate before in-place encryption overwrites the plaintext.
	T := ccm.getTag(Ctr, data, plaintext)

	ctr.XORKeyStream(ciphertext, plaintext)

	xorBytes(S0, S0, T) // T^S0

	return ret[:len(dst)+len(plaintext)+ccm.tagSize]
}

func (ccm *ccm) Open(dst, nonce, ciphertext, data []byte) ([]byte, error) {
	if len(nonce) != ccm.nonceSize {
		panic("cipher: incorrect nonce length given to CCM")
	}

	if len(ciphertext) < ccm.tagSize {
		return nil, errors.New("cipher: incorrect ciphertext length given to CCM")
	}

	if maxUvarint(15-ccm.nonceSize) < uint64(len(ciphertext)-ccm.tagSize) {
		return nil, errors.New("cipher: len(ciphertext)-tagSize exceeds the maximum payload size")
	}

	ret, plaintext := sliceForAppend(dst, len(ciphertext)-ccm.tagSize)

	// Formatting of the Counter Blocks are defined in A.3.
	Ctr := make([]byte, 16)               // Ctr0
	Ctr[0] = byte(15 - ccm.nonceSize - 1) // [q-1]3
	copy(Ctr[1:], nonce)                  // N

	S0 := make([]byte, 16) // S0
	ccm.c.Encrypt(S0, Ctr)

	Ctr[15] = 1 // Ctr1

	ctr := cipher.NewCTR(ccm.c, Ctr)

	ctr.XORKeyStream(plaintext, ciphertext[:len(plaintext)])

	T := ccm.getTag(Ctr, data, plaintext)

	xorBytes(T, T, S0)

	if subtle.ConstantTimeCompare(T[:ccm.tagSize], ciphertext[len(plaintext):]) != 1 {
		return nil, errors.New("crypto/ccm: message authentication failed")
	}

	return ret, nil
}

// getTag reuses a Ctr block for making the B0 block because of some parts are the same.
// For more details, see A.2 and A.3.
func (ccm *ccm) getTag(Ctr, data, plaintext []byte) []byte {
	m := ccm.macPool.Get().(*mac)
	defer ccm.macPool.Put(m)
	m.Reset()

	B := Ctr                                                // B0
	B[0] |= byte(((ccm.tagSize - 2) / 2) << 3)              // [(t-2)/2]3
	putUvarint(B[1+ccm.nonceSize:], uint64(len(plaintext))) // Q

	if len(data) > 0 {
		B[0] |= 1 << 6 // Adata

		m.Write(B)

		// The associated data length encoding is defined in RFC 3610 2.2
		// (also NIST SP 800-38C A.2.2): 0 < a < 2^16-2^8 uses two octets,
		// 2^16-2^8 <= a < 2^32 uses 0xfffe followed by four octets, and
		// 2^32 <= a uses 0xffff followed by eight octets.
		if len(data) < (1<<16 - 1<<8) {
			putUvarint(B[:2], uint64(len(data)))

			m.Write(B[:2])
		} else if uint64(len(data)) < (uint64(1) << 32) {
			B[0] = 0xff
			B[1] = 0xfe
			putUvarint(B[2:6], uint64(len(data)))

			m.Write(B[:6])
		} else {
			B[0] = 0xff
			B[1] = 0xff
			putUvarint(B[2:10], uint64(len(data)))

			m.Write(B[:10])
		}
		m.Write(data)
		m.PadZero()
	} else {
		m.Write(B)
	}

	m.Write(plaintext)
	m.PadZero()

	return m.Sum(nil)
}

func maxUvarint(n int) uint64 {
	return 1<<uint(n*8) - 1
}

// put uint64 as big endian.
func putUvarint(bs []byte, u uint64) {
	for i := range bs {
		bs[i] = byte(u >> uint(8*(len(bs)-1-i)))
	}
}
