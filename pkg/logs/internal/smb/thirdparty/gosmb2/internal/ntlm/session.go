package ntlm

import (
	"bytes"
	"crypto/rc4"
	"errors"
	"slices"
)

type Session struct {
	isClientSide bool

	user string

	negotiateFlags     uint32
	exportedSessionKey []byte
	clientSigningKey   []byte
	serverSigningKey   []byte

	clientHandle *rc4.Cipher
	serverHandle *rc4.Cipher
}

func (s *Session) User() string {
	return s.user
}

func (s *Session) SessionKey() []byte {
	return slices.Clone(s.exportedSessionKey)
}

func (s *Session) Overhead() int {
	return 16
}

func (s *Session) Sign(message []byte, seqNum uint32) ([]byte, uint32) {
	if s.negotiateFlags&NTLMSSP_NEGOTIATE_SIGN == 0 {
		return nil, 0
	}

	if s.isClientSide {
		return mac(nil, s.negotiateFlags, s.clientHandle, s.clientSigningKey, seqNum, message)
	}
	return mac(nil, s.negotiateFlags, s.serverHandle, s.serverSigningKey, seqNum, message)
}

func (s *Session) Verify(signature, message []byte, seqNum uint32) (bool, uint32) {
	if s.negotiateFlags&NTLMSSP_NEGOTIATE_SIGN == 0 {
		if signature == nil {
			return true, 0
		}
		return false, 0
	}

	if s.isClientSide {
		ret, seqNum := mac(nil, s.negotiateFlags, s.serverHandle, s.serverSigningKey, seqNum, message)
		if !bytes.Equal(signature, ret) {
			return false, 0
		}
		return true, seqNum
	}
	ret, seqNum := mac(nil, s.negotiateFlags, s.clientHandle, s.clientSigningKey, seqNum, message)
	if !bytes.Equal(signature, ret) {
		return false, 0
	}
	return true, seqNum
}

func (s *Session) Seal(dst, plaintext []byte, seqNum uint32) ([]byte, uint32) {
	ret, ciphertext := sliceForAppend(dst, len(plaintext)+16)

	switch {
	case s.negotiateFlags&NTLMSSP_NEGOTIATE_SEAL != 0:
		// [MS-NLMP] 3.1.1.1 and 3.4.3 require encryption and MAC to use
		// the same sealing handle for the sender's direction, in that order.
		if s.isClientSide {
			s.clientHandle.XORKeyStream(ciphertext[16:], plaintext)
		} else {
			s.serverHandle.XORKeyStream(ciphertext[16:], plaintext)
		}

		if s.isClientSide {
			_, seqNum = mac(ciphertext[:0], s.negotiateFlags, s.clientHandle, s.clientSigningKey, seqNum, plaintext)
		} else {
			_, seqNum = mac(ciphertext[:0], s.negotiateFlags, s.serverHandle, s.serverSigningKey, seqNum, plaintext)
		}
	case s.negotiateFlags&NTLMSSP_NEGOTIATE_SIGN != 0:
		copy(ciphertext[16:], plaintext)

		if s.isClientSide {
			_, seqNum = mac(ciphertext[:0], s.negotiateFlags, s.clientHandle, s.clientSigningKey, seqNum, plaintext)
		} else {
			_, seqNum = mac(ciphertext[:0], s.negotiateFlags, s.serverHandle, s.serverSigningKey, seqNum, plaintext)
		}
	default:
		copy(ciphertext[16:], plaintext)
	}

	return ret, seqNum
}

func (s *Session) Unseal(dst, ciphertext []byte, seqNum uint32) ([]byte, uint32, error) {
	// Both signature formats in [MS-NLMP] 2.2.2.9.1 and 2.2.2.9.2 are 16 bytes.
	if len(ciphertext) < 16 {
		return nil, seqNum, errors.New("invalid sealed message length")
	}

	ret, plaintext := sliceForAppend(dst, len(ciphertext)-16)

	switch {
	case s.negotiateFlags&NTLMSSP_NEGOTIATE_SEAL != 0:
		// [MS-NLMP] 3.1.1.1 and 3.4.3 require decryption and MAC to use
		// the same sealing handle for the incoming direction, in that order.
		if s.isClientSide {
			s.serverHandle.XORKeyStream(plaintext, ciphertext[16:])
		} else {
			s.clientHandle.XORKeyStream(plaintext, ciphertext[16:])
		}

		var sum []byte

		if s.isClientSide {
			sum, seqNum = mac(nil, s.negotiateFlags, s.serverHandle, s.serverSigningKey, seqNum, plaintext)
		} else {
			sum, seqNum = mac(nil, s.negotiateFlags, s.clientHandle, s.clientSigningKey, seqNum, plaintext)
		}
		if !bytes.Equal(ciphertext[:16], sum) {
			return nil, 0, errors.New("signature mismatch")
		}
	case s.negotiateFlags&NTLMSSP_NEGOTIATE_SIGN != 0:
		copy(plaintext, ciphertext[16:])

		var sum []byte

		if s.isClientSide {
			sum, seqNum = mac(nil, s.negotiateFlags, s.serverHandle, s.serverSigningKey, seqNum, plaintext)
		} else {
			sum, seqNum = mac(nil, s.negotiateFlags, s.clientHandle, s.clientSigningKey, seqNum, plaintext)
		}
		if !bytes.Equal(ciphertext[:16], sum) {
			return nil, 0, errors.New("signature mismatch")
		}
	default:
		copy(plaintext, ciphertext[16:])
		for _, s := range ciphertext[:16] {
			if s != 0x0 {
				return nil, 0, errors.New("signature mismatch")
			}
		}
	}

	return ret, seqNum, nil
}
