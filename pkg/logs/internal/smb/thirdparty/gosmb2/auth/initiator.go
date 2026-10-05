// Package auth provides SMB authentication mechanisms and credentials.
package auth

import (
	"encoding/asn1"
	"errors"

	"github.com/DataDog/datadog-agent/pkg/logs/internal/smb/thirdparty/gosmb2/internal/ntlm"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/smb/thirdparty/gosmb2/internal/spnego"
)

// Initiator performs one authentication handshake. Create a fresh initiator
// for each session; it must not be used by concurrent handshakes.
type Initiator interface {
	OID() asn1.ObjectIdentifier
	InitSecContext() ([]byte, error)
	AcceptSecContext([]byte) ([]byte, error)
	GetMIC([]byte) ([]byte, error)
	VerifyMIC([]byte, []byte) error
	Complete() bool
	SessionKey() []byte
}

// ntlmInitiator implements session-setup through NTLMv2.
// It doesn't support NTLMv1. You can use Hash instead of Password.
type ntlmInitiator struct {
	User        string
	Password    string
	Hash        []byte
	Domain      *string
	Workstation string
	TargetSPN   string

	ntlm       *ntlm.Client
	seqNum     uint32
	recvSeqNum uint32
	complete   bool
}

func (i *ntlmInitiator) OID() asn1.ObjectIdentifier {
	return spnego.NlmpOid
}

// IsAnonymous reports whether the initiator authenticates without credentials,
// which makes the server establish an anonymous session that cannot sign.
func (i *ntlmInitiator) IsAnonymous() bool {
	return i.User == "" && i.Password == "" && i.Hash == nil
}

func (i *ntlmInitiator) InitSecContext() ([]byte, error) {
	i.seqNum, i.recvSeqNum, i.complete = 0, 0, false
	i.ntlm = &ntlm.Client{
		User:        i.User,
		Password:    i.Password,
		Hash:        append([]byte(nil), i.Hash...),
		Domain:      i.Domain,
		Workstation: i.Workstation,
		TargetSPN:   i.TargetSPN,
	}
	nmsg, err := i.ntlm.Negotiate()
	if err != nil {
		return nil, err
	}
	return nmsg, nil
}

func (i *ntlmInitiator) AcceptSecContext(sc []byte) ([]byte, error) {
	if i.ntlm == nil || i.complete {
		return nil, errors.New("ntlm: unexpected authentication token")
	}
	amsg, err := i.ntlm.Authenticate(sc)
	if err != nil {
		return nil, err
	}
	i.complete = true
	return amsg, nil
}

func (i *ntlmInitiator) GetMIC(message []byte) ([]byte, error) {
	if !i.complete || i.ntlm == nil || i.ntlm.Session() == nil {
		return nil, errors.New("ntlm: authentication is incomplete")
	}
	var mic []byte
	mic, i.seqNum = i.ntlm.Session().Sign(message, i.seqNum)
	return mic, nil
}

func (i *ntlmInitiator) SessionKey() []byte {
	if i.ntlm == nil || i.ntlm.Session() == nil {
		return nil
	}
	return i.ntlm.Session().SessionKey()
}

func (i *ntlmInitiator) Complete() bool { return i.complete }

func (i *ntlmInitiator) VerifyMIC(message, mic []byte) error {
	if !i.complete || i.ntlm == nil || i.ntlm.Session() == nil {
		return errors.New("ntlm: authentication is incomplete")
	}
	ok, next := i.ntlm.Session().Verify(mic, message, i.recvSeqNum)
	if !ok {
		return errors.New("ntlm: invalid mechanism list MIC")
	}
	i.recvSeqNum = next
	return nil
}
