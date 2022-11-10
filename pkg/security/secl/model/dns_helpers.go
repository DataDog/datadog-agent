// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package model

import (
	"bytes"
	"errors"
)

var (
	// ErrDNSNamePointerNotSupported reported because pointer compression is not supported
	ErrDNSNamePointerNotSupported = errors.New("dns name pointer compression is not supported")
	// ErrDNSNameOutOfBounds reported because name out of bound
	ErrDNSNameOutOfBounds = errors.New("dns name out of bound")
	// ErrDNSNameNonPrintableASCII reported because name non-printable ascii
	ErrDNSNameNonPrintableASCII = errors.New("dns name non-printable ascii")
)

func decodeDNSName(raw []byte) (string, error) {
	var (
		i       = 0
		rawLen  = len(raw)
		atStart = true
		rep     bytes.Buffer
		err     error
	)

LOOP:
	for i < rawLen {
		// Parse label length
		labelLen := int(raw[i])
		i++

		if labelLen == 0 {
			// end of name
			break
		}

		if labelLen&0xC0 != 0 {
			// pointer compression, we do not support this yet
			err = ErrDNSNamePointerNotSupported
			break
		}

		if rawLen < i+labelLen {
			// out of bounds
			err = ErrDNSNameOutOfBounds
			break
		}

		labelRaw := raw[i : i+labelLen]

		if !atStart {
			rep.WriteRune('.')
		}
		for _, c := range labelRaw {
			if c < ' ' || '~' < c {
				// non-printable ascii char
				err = ErrDNSNameNonPrintableASCII
				break LOOP
			}
		}
		rep.Write(labelRaw)

		atStart = false
		i += labelLen
	}

	return rep.String(), err
}
