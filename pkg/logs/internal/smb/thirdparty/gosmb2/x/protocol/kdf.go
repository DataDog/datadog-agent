// ref: NIST SP 800-108 5.1

package protocol

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
)

// KDF in Counter Mode with h = 256 and r = 32.
func kdf(ki, label, context []byte, keySize int) []byte {
	if keySize <= 0 {
		return nil
	}

	var outputBits [4]byte
	binary.BigEndian.PutUint32(outputBits[:], uint32(keySize*8))

	out := make([]byte, 0, ((keySize+31)/32)*32)
	var counter [4]byte
	for i := uint32(1); len(out) < keySize; i++ {
		h := hmac.New(sha256.New, ki)
		binary.BigEndian.PutUint32(counter[:], i)
		h.Write(counter[:])
		h.Write(label)
		h.Write([]byte{0x00})
		h.Write(context)
		h.Write(outputBits[:])
		out = h.Sum(out)
	}

	return out[:keySize]
}
