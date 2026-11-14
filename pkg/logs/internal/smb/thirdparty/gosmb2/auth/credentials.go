package auth

import (
	"context"
	"fmt"
)

// NTLMCredential creates NTLM initiators using the same account for each
// server selected by a DFS referral.
type NTLMCredential struct {
	User     string
	Password string
	Hash     []byte
	// Domain selects the authentication domain. Nil uses the server's
	// challenge TargetName; a pointer to an empty string uses an empty domain.
	Domain      *string
	Workstation string
	// TargetSPN defaults to cifs/<server> when nil. An empty string omits the target name.
	TargetSPN *string
}

func (c NTLMCredential) NewInitiator(ctx context.Context, serverName string) (Initiator, error) {
	if ctx == nil {
		panic("nil context")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if c.Hash != nil && len(c.Hash) != 16 {
		return nil, fmt.Errorf("auth: NTLM hash must be 16 bytes: %w", errInvalidCredential)
	}
	spn := "cifs/" + serverName
	if c.TargetSPN != nil {
		spn = *c.TargetSPN
	}
	domain := c.Domain
	if domain != nil {
		value := *domain
		domain = &value
	}
	return &ntlmInitiator{
		User:        c.User,
		Password:    c.Password,
		Hash:        append([]byte(nil), c.Hash...),
		Domain:      domain,
		Workstation: c.Workstation,
		TargetSPN:   spn,
	}, nil
}
