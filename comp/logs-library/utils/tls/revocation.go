// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tlsutil

import (
	"bytes"
	"crypto/x509"
	"fmt"
)

// checkChainsRevocation rejects a client certificate when every chain that
// verified against the trust store contains a revoked certificate. A single
// clean chain is enough to accept: cross-signed PKIs can produce several valid
// chains, and revoking one intermediate should not invalidate a path that does
// not go through it.
func checkChainsRevocation(chains [][]*x509.Certificate, crls []*x509.RevocationList) error {
	if len(crls) == 0 {
		return nil
	}
	var firstErr error
	for _, chain := range chains {
		err := checkChainRevocation(chain, crls)
		if err == nil {
			return nil
		}
		if firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// checkChainRevocation reports an error if any certificate in chain is listed in
// a revocation list published by that certificate's issuer. chain is ordered
// leaf-first, so each certificate's issuer is its successor; the final
// certificate is the trust anchor and has no issuer in the chain to attest for
// it, so it is not checked.
//
// A revocation list is only consulted for a certificate it could plausibly
// cover: its issuer name must match the candidate issuer and its signature must
// verify against that issuer's key. An unrelated or tampered list therefore
// cannot cause a spurious rejection.
func checkChainRevocation(chain []*x509.Certificate, crls []*x509.RevocationList) error {
	for i := 0; i+1 < len(chain); i++ {
		cert, issuer := chain[i], chain[i+1]
		for _, crl := range crls {
			if !bytes.Equal(crl.RawIssuer, issuer.RawSubject) {
				continue
			}
			if err := crl.CheckSignatureFrom(issuer); err != nil {
				continue
			}
			for _, entry := range crl.RevokedCertificateEntries {
				if entry.SerialNumber.Cmp(cert.SerialNumber) == 0 {
					return &RevokedCertificateError{Certificate: cert, Entry: entry}
				}
			}
		}
	}
	return nil
}

// RevokedCertificateError reports that a presented certificate appears in a
// certificate revocation list.
type RevokedCertificateError struct {
	Certificate *x509.Certificate
	Entry       x509.RevocationListEntry
}

func (e *RevokedCertificateError) Error() string {
	return fmt.Sprintf("certificate %q (serial %s, issuer %q) was revoked at %s with reason code %d",
		e.Certificate.Subject.CommonName, e.Certificate.SerialNumber,
		e.Certificate.Issuer.CommonName, e.Entry.RevocationTime.UTC(), e.Entry.ReasonCode)
}
