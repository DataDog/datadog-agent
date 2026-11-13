package ntlm

import (
	"bytes"
	"errors"

	"github.com/DataDog/datadog-agent/pkg/logs/internal/smb/thirdparty/gosmb2/internal/utf16le"
)

type ChallengeMessage struct {
	raw        []byte
	flags      uint32
	info       *targetInfoEncoder
	targetName []byte
}

// Unmarshal parses the ChallengeMessage in cmsg and returns the result.
func UnmarshalChallengeMessage(cmsg, nmsg []byte, targetSPN string) (*ChallengeMessage, error) {
	//        ChallengeMessage
	//   0-8: Signature
	//  8-12: MessageType
	// 12-20: TargetNameFields
	// 20-24: NegotiateFlags
	// 24-32: ServerChallenge
	// 32-40: _
	// 40-48: TargetInfoFields
	// 48-56: Version
	//   56-: Payload
	// NegotiateFlags occupies bytes 12-16 of NEGOTIATE_MESSAGE ([MS-NLMP] 2.2.1.1).
	if len(cmsg) < 48 || len(nmsg) < 16 {
		return nil, errors.New("message length is too short")
	}

	if !bytes.Equal(cmsg[:8], signature) {
		return nil, errors.New("invalid signature")
	}

	if le.Uint32(cmsg[8:12]) != NtLmChallenge {
		return nil, errors.New("invalid message type")
	}

	clientFlags := le.Uint32(nmsg[12:16])
	serverFlags := le.Uint32(cmsg[20:24])
	if clientFlags&NTLMSSP_NEGOTIATE_SIGN != 0 && serverFlags&NTLMSSP_NEGOTIATE_SIGN == 0 {
		return nil, errors.New("server did not negotiate requested signing")
	}
	flags := clientFlags & serverFlags

	if flags&NTLMSSP_REQUEST_TARGET == 0 {
		return nil, errors.New("invalid negotiate flags")
	}

	// MS-NLMP 2.2.1.2 requires ignoring TargetNameMaxLen and
	// TargetInfoMaxLen on receipt; only the lengths locate payload data.
	targetNameLen := le.Uint16(cmsg[12:14])          // cmsg.TargetNameLen
	targetNameBufferOffset := le.Uint32(cmsg[16:20]) // cmsg.TargetNameBufferOffset
	// MS-NLMP 2.2.1.2 requires Unicode target names to have even
	// lengths and offsets. OEM names do not have this constraint.
	if serverFlags&NTLMSSP_NEGOTIATE_UNICODE != 0 && (targetNameLen&1 != 0 || targetNameBufferOffset&1 != 0) {
		return nil, errors.New("invalid target name alignment")
	}
	if targetNameLen > 0 && targetNameBufferOffset < 48 {
		return nil, errors.New("invalid target name format")
	}
	targetNameEnd := uint64(targetNameBufferOffset) + uint64(targetNameLen)
	if targetNameEnd > uint64(len(cmsg)) {
		return nil, errors.New("invalid target name format")
	}
	targetName := cmsg[targetNameBufferOffset:targetNameEnd] // cmsg.TargetName

	if flags&NTLMSSP_NEGOTIATE_TARGET_INFO == 0 {
		return nil, errors.New("invalid negotiate flags")
	}

	targetInfoLen := le.Uint16(cmsg[40:42])          // cmsg.TargetInfoLen
	targetInfoBufferOffset := le.Uint32(cmsg[44:48]) // cmsg.TargetInfoBufferOffset
	if targetInfoBufferOffset < 48 {
		return nil, errors.New("invalid target info format")
	}
	targetInfoEnd := uint64(targetInfoBufferOffset) + uint64(targetInfoLen)
	if targetInfoEnd > uint64(len(cmsg)) {
		return nil, errors.New("invalid target info format")
	}
	targetInfo := cmsg[targetInfoBufferOffset:targetInfoEnd] // cmsg.TargetInfo
	info := newTargetInfoEncoder(targetInfo, utf16le.EncodeStringToBytes(targetSPN))
	if info == nil {
		return nil, errors.New("invalid target info format")
	}

	return &ChallengeMessage{
		raw:        cmsg,
		flags:      flags,
		info:       info,
		targetName: targetName,
	}, nil
}
