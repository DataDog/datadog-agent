// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package repo

import (
	"bytes"
	"errors"

	"github.com/ProtonMail/go-crypto/openpgp"
	"github.com/sassoftware/go-rpmutils"
)

// verifyPackage enforces gpgcheck: a package needs a signature from a configured
// trusted key that authenticates its payload. rpmutils.Verify alone also accepts
// unsigned packages whose unkeyed digests are merely self-consistent.
// A header-only signature is sufficient only when the signed header contains a
// SHA-2 payload digest. MD5 and SHA-1 are excluded here because their collision
// resistance is broken; an unsigned or weakly bound payload is not authenticated.
// Combined header-and-payload signatures do not rely on this payload-digest list.
func verifyPackage(data []byte, keys openpgp.EntityList) error {
	if len(keys) == 0 {
		return errors.New("RPM signature verification requires a trusted signing key")
	}
	header, signatures, err := rpmutils.Verify(bytes.NewReader(data), keys)
	if err != nil {
		return err
	}
	for _, sig := range signatures {
		if sig.Signer == nil {
			continue
		}
		if !sig.HeaderOnly {
			return nil
		}
		// A header-only signature must bind the payload through a strong digest
		// in the signed general header. Verify has already checked that digest.
		digests, err := header.GetStrings(rpmutils.PAYLOADDIGEST)
		if err != nil || len(digests) == 0 || digests[0] == "" {
			continue
		}
		algorithms, err := header.GetUint32s(rpmutils.PAYLOADDIGESTALGO)
		if err != nil || len(algorithms) == 0 {
			continue
		}
		switch algorithms[0] {
		case rpmutils.HASH_SHA224, rpmutils.HASH_SHA256, rpmutils.HASH_SHA384, rpmutils.HASH_SHA512:
			return nil
		}
	}
	return errors.New("RPM has no trusted signature authenticating its payload")
}
