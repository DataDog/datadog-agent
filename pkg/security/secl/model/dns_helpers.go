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
	errDNSNamePointerNotSupported = errors.New("dns name pointer compression is not supported")
	errDNSNameOutOfBounds         = errors.New("dns name out of bound")
	errDNSNameNonPrintableASCII   = errors.New("dns name non-printable ascii")
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
			err = errDNSNamePointerNotSupported
			break
		}

		if rawLen < i+labelLen {
			// out of bounds
			err = errDNSNameOutOfBounds
			break
		}

		labelRaw := raw[i : i+labelLen]

		if !atStart {
			rep.WriteRune('.')
		}
		for _, c := range labelRaw {
			if c < ' ' || '~' < c {
				// non-printable ascii char
				err = errDNSNameNonPrintableASCII
				break LOOP
			}
		}
		rep.Write(labelRaw)

		atStart = false
		i += labelLen
	}

	return rep.String(), err
}
