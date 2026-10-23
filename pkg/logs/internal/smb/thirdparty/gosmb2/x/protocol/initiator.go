package protocol

import "encoding/asn1"

// Initiator supplies the security mechanism used by SESSION_SETUP. Concrete
// implementations live in the auth package; this interface keeps the
// protocol package independent from credential construction.
type Initiator interface {
	OID() asn1.ObjectIdentifier
	InitSecContext() ([]byte, error)
	AcceptSecContext([]byte) ([]byte, error)
	GetMIC([]byte) ([]byte, error)
	VerifyMIC([]byte, []byte) error
	Complete() bool
	SessionKey() []byte
}

// anonymousInitiator is optional so existing mechanisms need not expose an
// anonymous marker. Implementations that do expose it must use the exported
// IsAnonymous method.
type anonymousInitiator interface {
	IsAnonymous() bool
}
