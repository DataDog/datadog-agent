// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tlsutil

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// revocationTestCA issues certificates and revocation lists for the tests below.
type revocationTestCA struct {
	key    crypto.Signer
	cert   *x509.Certificate
	serial int64
}

func newRevocationTestCA(t *testing.T, cn string, parent *revocationTestCA) *revocationTestCA {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: cn},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}

	signerCert, signerKey := tmpl, crypto.Signer(key)
	if parent != nil {
		tmpl.SerialNumber = big.NewInt(parent.nextSerial())
		signerCert, signerKey = parent.cert, parent.key
	}

	der, err := x509.CreateCertificate(rand.Reader, tmpl, signerCert, key.Public(), signerKey)
	require.NoError(t, err)
	cert, err := x509.ParseCertificate(der)
	require.NoError(t, err)

	return &revocationTestCA{key: key, cert: cert, serial: 100}
}

func (ca *revocationTestCA) nextSerial() int64 {
	ca.serial++
	return ca.serial
}

func (ca *revocationTestCA) issueLeaf(t *testing.T, cn string) *x509.Certificate {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(ca.nextSerial()),
		Subject:      pkix.Name{CommonName: cn},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, ca.cert, key.Public(), ca.key)
	require.NoError(t, err)
	cert, err := x509.ParseCertificate(der)
	require.NoError(t, err)
	return cert
}

func (ca *revocationTestCA) issueCRL(t *testing.T, certs ...*x509.Certificate) *x509.RevocationList {
	t.Helper()

	entries := make([]x509.RevocationListEntry, 0, len(certs))
	for _, cert := range certs {
		entries = append(entries, x509.RevocationListEntry{
			SerialNumber:   cert.SerialNumber,
			RevocationTime: time.Now().Add(-time.Minute),
			ReasonCode:     1, // keyCompromise
		})
	}

	der, err := x509.CreateRevocationList(rand.Reader, &x509.RevocationList{
		Number:                    big.NewInt(1),
		ThisUpdate:                time.Now().Add(-time.Hour),
		NextUpdate:                time.Now().Add(time.Hour),
		RevokedCertificateEntries: entries,
	}, ca.cert, ca.key)
	require.NoError(t, err)

	crl, err := x509.ParseRevocationList(der)
	require.NoError(t, err)
	return crl
}

func TestCheckChainRevocation(t *testing.T) {
	ca := newRevocationTestCA(t, "revocation-ca", nil)
	leaf := ca.issueLeaf(t, "client")
	other := ca.issueLeaf(t, "other-client")
	chain := []*x509.Certificate{leaf, ca.cert}

	t.Run("no lists accepts", func(t *testing.T) {
		assert.NoError(t, checkChainRevocation(chain, nil))
	})

	t.Run("list without the leaf accepts", func(t *testing.T) {
		assert.NoError(t, checkChainRevocation(chain, []*x509.RevocationList{ca.issueCRL(t, other)}))
	})

	t.Run("revoked leaf is rejected", func(t *testing.T) {
		err := checkChainRevocation(chain, []*x509.RevocationList{ca.issueCRL(t, leaf)})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "was revoked")
		assert.Contains(t, err.Error(), leaf.SerialNumber.String())

		var revoked *RevokedCertificateError
		require.ErrorAs(t, err, &revoked)
		assert.Equal(t, leaf.SerialNumber, revoked.Certificate.SerialNumber)
	})

	t.Run("revoked intermediate is rejected", func(t *testing.T) {
		root := newRevocationTestCA(t, "revocation-root", nil)
		intermediate := newRevocationTestCA(t, "revocation-intermediate", root)
		intermediateLeaf := intermediate.issueLeaf(t, "deep-client")

		deepChain := []*x509.Certificate{intermediateLeaf, intermediate.cert, root.cert}
		crl := root.issueCRL(t, intermediate.cert)

		err := checkChainRevocation(deepChain, []*x509.RevocationList{crl})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "revocation-intermediate")
	})

	// A list signed by a different key must never be trusted, even when its
	// issuer name matches: otherwise anyone able to write the CRL file could
	// revoke arbitrary certificates, or an operator copying the wrong file
	// would lock out valid clients.
	t.Run("list from an impostor issuer is ignored", func(t *testing.T) {
		impostor := newRevocationTestCA(t, "revocation-ca", nil)
		impostorCRL := impostor.issueCRL(t, leaf)
		require.Equal(t, ca.cert.RawSubject, impostor.cert.RawSubject, "test requires matching issuer names")

		assert.NoError(t, checkChainRevocation(chain, []*x509.RevocationList{impostorCRL}))
	})

	// The trust anchor has no issuer inside the chain, so nothing in the chain
	// can attest to a list revoking it.
	t.Run("self-signed root in the chain is not checked", func(t *testing.T) {
		assert.NoError(t, checkChainRevocation([]*x509.Certificate{ca.cert}, []*x509.RevocationList{ca.issueCRL(t, ca.cert)}))
	})
}

func TestCheckChainsRevocation(t *testing.T) {
	ca := newRevocationTestCA(t, "chains-ca", nil)
	leaf := ca.issueLeaf(t, "client")
	revokedCRL := ca.issueCRL(t, leaf)

	t.Run("empty list set accepts", func(t *testing.T) {
		chains := [][]*x509.Certificate{{leaf, ca.cert}}
		assert.NoError(t, checkChainsRevocation(chains, nil))
	})

	t.Run("all chains revoked is rejected", func(t *testing.T) {
		chains := [][]*x509.Certificate{{leaf, ca.cert}}
		require.Error(t, checkChainsRevocation(chains, []*x509.RevocationList{revokedCRL}))
	})

	// Cross-signed PKIs yield several valid chains. Revoking an intermediate on
	// one path must not invalidate a path that does not traverse it.
	t.Run("one clean chain accepts", func(t *testing.T) {
		altCA := newRevocationTestCA(t, "alternate-ca", nil)
		altLeaf := altCA.issueLeaf(t, "client")

		chains := [][]*x509.Certificate{
			{leaf, ca.cert},
			{altLeaf, altCA.cert},
		}
		assert.NoError(t, checkChainsRevocation(chains, []*x509.RevocationList{revokedCRL}))
	})
}
