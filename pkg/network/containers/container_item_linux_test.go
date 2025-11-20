// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package containers

import (
	"bufio"
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const exampleFile = `;
; /etc/resolv.conf for dnssecond
;
domain           doc.com
# test inline comment
nameserver       111.22.3.5
nameserver       123.45.6.1
`

func BenchmarkStripResolvConf(b *testing.B) {
	f, err := os.CreateTemp(b.TempDir(), "resolvconf")
	require.NoError(b, err)
	_, err = f.WriteString(exampleFile)
	_ = f.Close()
	require.NoError(b, err)

	b.Run("string", func(b *testing.B) {
		for b.Loop() {
			_, err = readResolvConfPath(f.Name())
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	r := resolvStripper{buf: make([]byte, 0, bufio.MaxScanTokenSize+1)}
	b.Run("scanner", func(b *testing.B) {
		for b.Loop() {
			f, err = os.Open(f.Name())
			if err != nil {
				b.Fatal(err)
			}
			_, err = r.stripResolvConfFile(f)
			f.Close()
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

type resolvStripper struct {
	buf []byte
}

func (r *resolvStripper) stripResolvConfFile(f *os.File) (string, error) {
	scanner := bufio.NewScanner(f)
	scanner.Buffer(r.buf, bufio.MaxScanTokenSize)
	var sb strings.Builder
	if stat, err := f.Stat(); err == nil {
		sb.Grow(int(stat.Size()))
	}

	for scanner.Scan() {
		trim := bytes.TrimSpace(scanner.Bytes())
		if len(trim) == 0 {
			continue
		}
		if trim[0] == ';' || trim[0] == '#' {
			continue
		}
		sb.Write(trim)
		sb.WriteByte('\n')
	}
	full := sb.String()
	// remove final newline
	return full[:len(full)-1], nil
}
