// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package model

import (
	"bytes"
	"fmt"
)

func decodeDNS(raw []byte) string {
	rawLen := len(raw)
	var rep bytes.Buffer
	i := 0
	atStart := true

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
			break
		}

		if rawLen < i+labelLen {
			// out of bounds
			break
		}

		labelRaw := raw[i : i+labelLen]

		if !atStart {
			rep.WriteRune('.')
		}
		for _, c := range labelRaw {
			if isSimpleSpecialChar(c) {
				rep.WriteRune('\\')
				rep.WriteByte(c)
			} else if c < ' ' || '~' < c {
				rep.WriteString(fmt.Sprintf("\\%d", c))
			} else {
				rep.WriteByte(c)
			}
		}

		atStart = false
		i += labelLen
	}

	return rep.String()
}

func isSimpleSpecialChar(b byte) bool {
	switch b {
	case '.', ' ', '\'', '@', ';', '(', ')', '"', '\\':
		return true
	}
	return false
}
