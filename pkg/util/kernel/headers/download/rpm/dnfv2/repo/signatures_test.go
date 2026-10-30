// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package repo

import (
	"bytes"
	_ "embed"
	"encoding/binary"
	"testing"

	"github.com/ProtonMail/go-crypto/openpgp"
	"github.com/sassoftware/go-rpmutils"
	"github.com/stretchr/testify/require"
)

//go:embed testdata/simple-1.0.1-1.i386.rpm
var unsignedRPM []byte

// TestVerifyPackageRequiresSignature starts with an unsigned RPM that passes the
// dependency's digest checks. It then signs the same bytes and checks the trusted
// signer, an unrelated signer, and a payload modified after signing separately.
func TestVerifyPackageRequiresSignature(t *testing.T) {
	key, _ := signingKey(t)
	keys := openpgp.EntityList{key}

	t.Run("unsigned package", func(t *testing.T) {
		_, signatures, err := rpmutils.Verify(bytes.NewReader(unsignedRPM), keys)
		require.NoError(t, err)
		require.Empty(t, signatures)
		require.ErrorContains(t, verifyPackage(unsignedRPM, keys), "no trusted signature")
	})

	t.Run("missing trusted keys", func(t *testing.T) {
		require.ErrorContains(t, verifyPackage(unsignedRPM, nil), "trusted signing key")
		require.Error(t, verifyPackage(unsignedRPM, openpgp.EntityList{}))
	})

	// Replace the fixture's signature header, keeping its original header/payload.
	header, err := rpmutils.SignRpmStream(bytes.NewReader(unsignedRPM), key.PrivateKey, nil)
	require.NoError(t, err)
	signatureHeader, err := header.DumpSignatureHeader(false)
	require.NoError(t, err)
	signed := append(signatureHeader, unsignedRPM[header.OriginalSignatureHeaderSize():]...)

	t.Run("trusted signer", func(t *testing.T) {
		require.NoError(t, verifyPackage(signed, keys))
	})

	t.Run("unrelated signer", func(t *testing.T) {
		other, _ := signingKey(t)
		require.Error(t, verifyPackage(signed, openpgp.EntityList{other}))
	})

	t.Run("modified payload", func(t *testing.T) {
		modified := append([]byte(nil), signed...)
		modified[len(modified)-1] ^= 0xff
		require.Error(t, verifyPackage(modified, keys))
	})
}

//go:embed testdata/sha256.rpm
var sha256RPM []byte

// headerOnlyRPM removes the combined signature from the signature header,
// retaining a real signature over the unchanged general header. The signature
// header follows the 96-byte RPM lead and has a 16-byte prefix, then 16-byte index
// entries and their data. We filter the two combined-signature tags, reuse the
// data section (so retained offsets stay valid), and restore eight-byte padding.
func headerOnlyRPM(t *testing.T, original []byte, key *openpgp.Entity) []byte {
	t.Helper()
	header, err := rpmutils.SignRpmStream(bytes.NewReader(original), key.PrivateKey, nil)
	require.NoError(t, err)
	blob, err := header.DumpSignatureHeader(false)
	require.NoError(t, err)
	count := int(binary.BigEndian.Uint32(blob[104:108]))
	size := int(binary.BigEndian.Uint32(blob[108:112]))
	result := append([]byte(nil), blob[:112]...)
	kept := uint32(0)
	for i := 0; i < count; i++ {
		entry := blob[112+16*i : 112+16*(i+1)]
		tag := binary.BigEndian.Uint32(entry[:4])
		if tag == 1002 || tag == 1005 {
			continue
		}
		result = append(result, entry...)
		kept++
	}
	binary.BigEndian.PutUint32(result[104:108], kept)
	result = append(result, blob[112+16*count:112+16*count+size]...)
	for len(result)%8 != 0 {
		result = append(result, 0)
	}
	return append(result, original[header.OriginalSignatureHeaderSize():]...)
}

// TestHeaderSignatureMustAuthenticatePayload contrasts two header-only signed
// fixtures: SHA-256 binds the first payload to its signed header; the older
// fixture has no strong payload binding and must be rejected despite its signature.
func TestHeaderSignatureMustAuthenticatePayload(t *testing.T) {
	key, _ := signingKey(t)
	keys := openpgp.EntityList{key}
	t.Run("signed SHA-256 payload digest", func(t *testing.T) {
		require.NoError(t, verifyPackage(headerOnlyRPM(t, sha256RPM, key), keys))
	})
	t.Run("no strong payload binding", func(t *testing.T) {
		require.ErrorContains(t, verifyPackage(headerOnlyRPM(t, unsignedRPM, key), keys), "authenticating its payload")
	})
}
