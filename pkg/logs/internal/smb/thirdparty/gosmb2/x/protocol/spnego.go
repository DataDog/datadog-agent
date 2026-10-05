package protocol

import (
	"encoding/asn1"
	"errors"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/smb/thirdparty/gosmb2/x/wire"

	"github.com/DataDog/datadog-agent/pkg/logs/internal/smb/thirdparty/gosmb2/internal/spnego"
)

// negTokenResp.negState values (RFC 2478/4178 negotiation results)
const (
	negStateAcceptCompleted  asn1.Enumerated = 0
	negStateAcceptIncomplete asn1.Enumerated = 1
	negStateReject           asn1.Enumerated = 2
	negStateRequestMIC       asn1.Enumerated = 3
)

type spnegoClient struct {
	mechs        []Initiator
	mechTypes    []asn1.ObjectIdentifier
	selectedMech Initiator
	micRequired  bool
	micReceived  bool
	micSent      bool
}

func newSpnegoClient(mechs []Initiator) *spnegoClient {
	mechTypes := make([]asn1.ObjectIdentifier, len(mechs))
	for i, mech := range mechs {
		mechTypes[i] = mech.OID()
	}
	return &spnegoClient{
		mechs:     mechs,
		mechTypes: mechTypes,
	}
}

func (c *spnegoClient) initSecContext() (negTokenInitBytes []byte, err error) {
	if len(c.mechs) == 0 {
		return nil, errors.New("spnego: no mechanisms provided")
	}
	mechToken, err := c.mechs[0].InitSecContext()
	if err != nil {
		return nil, err
	}
	negTokenInitBytes, err = spnego.EncodeNegTokenInit(c.mechTypes, mechToken)
	if err != nil {
		return nil, err
	}
	return negTokenInitBytes, nil
}

func (c *spnegoClient) acceptSecContext(token []byte, complete bool) ([]byte, error) {
	resp, err := spnego.DecodeNegTokenResp(token)
	if err != nil {
		return nil, err
	}
	if resp.NegState < 0 || resp.NegState > negStateRequestMIC || resp.NegState == negStateReject {
		return nil, invalidResponse(wire.SMB2_SESSION_SETUP, "server rejected the negotiation or sent an invalid state")
	}
	first := c.selectedMech == nil
	if len(resp.SupportedMech) != 0 {
		if !first && !resp.SupportedMech.Equal(c.selectedMech.OID()) {
			return nil, invalidResponse(wire.SMB2_SESSION_SETUP, "server changed the authentication mechanism")
		}
		for n, oid := range c.mechTypes {
			if oid.Equal(resp.SupportedMech) {
				c.selectedMech = c.mechs[n]
				break
			}
		}
	}
	if c.selectedMech == nil {
		return nil, invalidResponse(wire.SMB2_SESSION_SETUP, "server selected an unsupported mechanism")
	}
	if resp.NegState == negStateRequestMIC {
		if !first {
			return nil, invalidResponse(wire.SMB2_SESSION_SETUP, "unexpected repeated MIC request")
		}
		c.micRequired = true
	}
	if !c.selectedMech.OID().Equal(c.mechTypes[0]) {
		c.micRequired = true
	}
	var output []byte
	if len(resp.ResponseToken) != 0 {
		output, err = c.selectedMech.AcceptSecContext(resp.ResponseToken)
		if err != nil {
			return nil, err
		}
	} else if !c.selectedMech.Complete() {
		return nil, invalidResponse(wire.SMB2_SESSION_SETUP, "server didn't provide a response token")
	}
	ms, err := asn1.Marshal(c.mechTypes)
	if err != nil {
		return nil, err
	}
	if len(resp.MechListMIC) != 0 {
		if c.micReceived {
			return nil, invalidResponse(wire.SMB2_SESSION_SETUP, "duplicate mechanism list MIC")
		}
		if err := c.selectedMech.VerifyMIC(ms, resp.MechListMIC); err != nil {
			return nil, err
		}
		c.micReceived = true
		c.micRequired = true
	}
	if complete {
		if resp.NegState != negStateAcceptCompleted || len(output) != 0 || !c.selectedMech.Complete() {
			return nil, invalidResponse(wire.SMB2_SESSION_SETUP, "security context is not complete")
		}
		if c.micRequired && (!c.micReceived || !c.micSent) {
			return nil, invalidResponse(wire.SMB2_SESSION_SETUP, "mechanism list MIC exchange is incomplete")
		}
		return nil, nil
	}
	if resp.NegState == negStateAcceptCompleted {
		return nil, invalidResponse(wire.SMB2_SESSION_SETUP, "SPNEGO completed before SESSION_SETUP")
	}
	var mic []byte
	// With the preferred mechanism, RFC 4178 permits omitting the MIC.
	// Once requested or received, both peers must exchange and verify it.
	if (c.micRequired || len(output) != 0) && c.selectedMech.Complete() && !c.micSent {
		mic, err = c.selectedMech.GetMIC(ms)
		if err != nil {
			return nil, err
		}
		if c.micRequired && len(mic) == 0 {
			return nil, invalidResponse(wire.SMB2_SESSION_SETUP, "mechanism did not generate a required MIC")
		}
		c.micSent = len(mic) != 0
	}
	state := negStateAcceptIncomplete
	if c.selectedMech.Complete() && len(output) == 0 {
		state = negStateAcceptCompleted
	}
	if len(output) == 0 && len(mic) == 0 && !(c.selectedMech.Complete() && len(resp.ResponseToken) != 0) {
		return nil, invalidResponse(wire.SMB2_SESSION_SETUP, "authentication made no progress")
	}
	return spnego.EncodeNegTokenResp(state, nil, output, mic)
}

func (c *spnegoClient) sessionKey() []byte {
	if c.selectedMech == nil {
		return nil
	}
	return c.selectedMech.SessionKey()
}
