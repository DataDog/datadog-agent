package ntlm

import (
	"crypto/hmac"
	"crypto/md5"
	"crypto/rand"
	"crypto/rc4"
	"hash"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/logs/internal/smb/thirdparty/gosmb2/internal/utf16le"
)

// NTLM v2 client
type Client struct {
	User        string
	Password    string
	Hash        []byte
	Domain      *string // nil uses the challenge TargetName; non-nil is used verbatim.
	Workstation string  // e.g "localhost", "HOME-PC"

	TargetSPN string // SPN ::= "service/hostname[:port]"; e.g "cifs/remotehost:1020"

	nmsg    []byte
	session *Session
}

func (c *Client) Negotiate() (nmsg []byte, err error) {
	//        NegotiateMessage
	//   0-8: Signature
	//  8-12: MessageType
	// 12-16: NegotiateFlags
	// 16-24: DomainNameFields
	// 24-32: WorkstationFields
	// 32-40: Version
	//   40-: Payload

	off := 32 + 8

	nmsg = make([]byte, off)

	copy(nmsg[:8], signature)
	le.PutUint32(nmsg[8:12], NtLmNegotiate)
	le.PutUint32(nmsg[12:16], defaultFlags)

	copy(nmsg[32:], version)

	c.nmsg = nmsg

	return nmsg, nil
}

func (c *Client) Authenticate(cmsg []byte) (amsg []byte, err error) {
	challengeMessage, err := UnmarshalChallengeMessage(cmsg, c.nmsg, c.TargetSPN)
	if err != nil {
		return nil, err
	}
	info := challengeMessage.info
	flags := challengeMessage.flags

	//        AuthenticateMessage
	//   0-8: Signature
	//  8-12: MessageType
	// 12-20: LmChallengeResponseFields
	// 20-28: NtChallengeResponseFields
	// 28-36: DomainNameFields
	// 36-44: UserNameFields
	// 44-52: WorkstationFields
	// 52-60: EncryptedRandomSessionKeyFields
	// 60-64: NegotiateFlags
	// 64-72: Version
	// 72-88: MIC
	//   88-: Payload

	off := 64 + 8 + 16

	domain := challengeMessage.targetName
	if c.Domain != nil {
		domain = utf16le.EncodeStringToBytes(*c.Domain)
	}
	user := utf16le.EncodeStringToBytes(c.User)
	workstation := utf16le.EncodeStringToBytes(c.Workstation)

	// LmChallengeResponseLen = 24
	// NtChallengeResponseLen =
	//   len(Response) = 16
	//	 len(NTLMv2ClientChallenge) =
	//     min len size = 28
	//     target info size
	//     padding = 4
	// len(EncryptedRandomSessionKey) = 0 or 16

	var (
		lmChallengeResponseLen       = 24
		ntChallengeResponseLen       = 16 + (28 + info.size() + 4)
		encryptedRandomSessionKeyLen = 16
		ntlmV2ResponseLen            = 16
	)
	if c.User == "" && c.Password == "" && c.Hash == nil {
		lmChallengeResponseLen = 1 // MS-NLMP 3.3.2: anonymous LM response is Z(1).
		ntChallengeResponseLen = 0
	}

	amsg = make([]byte, off+len(domain)+len(user)+len(workstation)+
		lmChallengeResponseLen+
		ntChallengeResponseLen+
		encryptedRandomSessionKeyLen)

	copy(amsg[:8], signature)
	le.PutUint32(amsg[8:12], NtLmAuthenticate)

	if domain != nil {
		len := copy(amsg[off:], domain)
		le.PutUint16(amsg[28:30], uint16(len))
		le.PutUint16(amsg[30:32], uint16(len))
		le.PutUint32(amsg[32:36], uint32(off))
		off += len
	}

	if user != nil {
		len := copy(amsg[off:], user)
		le.PutUint16(amsg[36:38], uint16(len))
		le.PutUint16(amsg[38:40], uint16(len))
		le.PutUint32(amsg[40:44], uint32(off))
		off += len
	}

	if workstation != nil {
		len := copy(amsg[off:], workstation)
		le.PutUint16(amsg[44:46], uint16(len))
		le.PutUint16(amsg[46:48], uint16(len))
		le.PutUint32(amsg[48:52], uint32(off))
		off += len
	}

	var h hash.Hash

	var (
		hashKey     []byte
		userEncoded = utf16le.EncodeStringToBytes(strings.ToUpper(c.User))
	)
	if c.Hash != nil {
		hashKey = ntowfv2Hash(userEncoded, c.Hash, domain)
	} else {
		password := utf16le.EncodeStringToBytes(c.Password)
		hashKey = ntowfv2(userEncoded, password, domain)
	}
	h = hmac.New(md5.New, hashKey)

	//        LMv2Response
	//  0-16: Response
	// 16-24: ChallengeFromClient

	le.PutUint16(amsg[12:14], uint16(lmChallengeResponseLen))
	le.PutUint16(amsg[14:16], uint16(lmChallengeResponseLen))
	le.PutUint32(amsg[16:20], uint32(off))

	off += lmChallengeResponseLen

	if ntChallengeResponseLen > 0 {
		//        NTLMv2Response
		//  0-16: Response
		//   16-: NTLMv2ClientChallenge
		ntChallengeResponse := amsg[off : len(amsg)-encryptedRandomSessionKeyLen]
		ntlmv2ClientChallenge := ntChallengeResponse[ntlmV2ResponseLen:]

		//        NTLMv2ClientChallenge
		//   0-1: RespType
		//   1-2: HiRespType
		//   2-4: _
		//   4-8: _
		//  8-16: TimeStamp
		// 16-24: ChallengeFromClient
		// 24-28: _
		//   28-: AvPairs

		serverChallenge := cmsg[24:32]

		clientChallenge := ntlmv2ClientChallenge[16:24]

		_, err := rand.Read(clientChallenge)
		if err != nil {
			return nil, err
		}

		timeStamp, ok := info.InfoMap[MsvAvTimestamp]
		if !ok {
			timeStamp = ntlmv2ClientChallenge[8:16]
			le.PutUint64(timeStamp, uint64((time.Now().UnixNano()/100)+116444736000000000))
		}

		encodeNtlmv2Response(ntChallengeResponse, h, serverChallenge, clientChallenge, timeStamp, info)

		le.PutUint16(amsg[20:22], uint16(ntChallengeResponseLen))
		le.PutUint16(amsg[22:24], uint16(ntChallengeResponseLen))
		le.PutUint32(amsg[24:28], uint32(off))

		off = len(amsg) - encryptedRandomSessionKeyLen
		h.Reset()
		h.Write(ntChallengeResponse[:16])
	}
	sessionBaseKey := h.Sum(nil)

	keyExchangeKey := sessionBaseKey // if ntlm version == 2

	if c.User == "" && c.Password == "" && c.Hash == nil {
		keyExchangeKey = anonymousKeyExchangeKey()
	}

	session := new(Session)

	session.isClientSide = true

	session.user = c.User
	session.negotiateFlags = flags

	if flags&NTLMSSP_NEGOTIATE_KEY_EXCH != 0 {
		session.exportedSessionKey = make([]byte, 16)
		_, err := rand.Read(session.exportedSessionKey)
		if err != nil {
			return nil, err
		}
		cipher, err := rc4.NewCipher(keyExchangeKey)
		if err != nil {
			return nil, err
		}
		encryptedRandomSessionKey := amsg[off:]
		cipher.XORKeyStream(encryptedRandomSessionKey, session.exportedSessionKey)

		le.PutUint16(amsg[52:54], 16)          // amsg.EncryptedRandomSessionKeyLen
		le.PutUint16(amsg[54:56], 16)          // amsg.EncryptedRandomSessionKeyMaxLen
		le.PutUint32(amsg[56:60], uint32(off)) // amsg.EncryptedRandomSessionKeyBufferOffset
	} else {
		session.exportedSessionKey = keyExchangeKey
	}

	le.PutUint32(amsg[60:64], flags)

	copy(amsg[64:], version)
	h = hmac.New(md5.New, session.exportedSessionKey)
	h.Write(c.nmsg)
	h.Write(cmsg)
	h.Write(amsg)
	_ = h.Sum(amsg[:72]) // amsg.MIC

	{
		session.clientSigningKey = signKey(flags, session.exportedSessionKey, true)
		session.serverSigningKey = signKey(flags, session.exportedSessionKey, false)

		session.clientHandle, err = rc4.NewCipher(sealKey(flags, session.exportedSessionKey, true))
		if err != nil {
			return nil, err
		}

		session.serverHandle, err = rc4.NewCipher(sealKey(flags, session.exportedSessionKey, false))
		if err != nil {
			return nil, err
		}
	}

	c.session = session

	return amsg, nil
}

func (c *Client) Session() *Session {
	return c.session
}
