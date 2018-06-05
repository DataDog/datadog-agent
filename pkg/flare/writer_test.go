// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package flare

import (
	"bufio"
	"bytes"
	"regexp"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/stretchr/testify/assert"
)

const (
	input = `dd_url: https://app.datadoghq.com
api_key: aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
proxy: http://user:password@host:1234
password: foo
auth_token: bar
# comment to strip
log_level: info`
)

func TestRedactingWriter(t *testing.T) {
	redacted := `dd_url: https://app.datadoghq.com
api_key: ***************************aaaaa
proxy: http://user:********@host:1234
password: ********
auth_token: ********
log_level: info`

	buf := bytes.NewBuffer([]byte{})
	w := RedactingWriter{
		targetBuf: bufio.NewWriter(buf),
	}

	n, err := w.Write([]byte(input))
	assert.Nil(t, err)
	err = w.Flush()
	assert.Nil(t, err)
	assert.Equal(t, len(redacted), n)
	assert.Equal(t, redacted, buf.String())

}

func TestRedactingWriterReplacers(t *testing.T) {
	redacted := `dd_url: https://app.datadoghq.com
api_key: ***************************aaaaa
proxy: http://USERISREDACTEDTOO:********@foo:bar
password: ********
auth_token: ********
log_level: info`

	buf := bytes.NewBuffer([]byte{})
	w := RedactingWriter{
		targetBuf: bufio.NewWriter(buf),
	}

	w.RegisterReplacer(log.Replacer{
		Regex: regexp.MustCompile(`user`),
		ReplFunc: func(s []byte) []byte {
			return []byte("USERISREDACTEDTOO")
		},
	})
	w.RegisterReplacer(log.Replacer{
		Regex: regexp.MustCompile(`@.*\:[0-9]+`),
		ReplFunc: func(s []byte) []byte {
			return []byte("@foo:bar")
		},
	})

	n, err := w.Write([]byte(input))
	assert.Nil(t, err)
	err = w.Flush()
	assert.Nil(t, err)
	assert.Equal(t, len(redacted), n)
	assert.Equal(t, redacted, buf.String())

}
func TestRedactingNothing(t *testing.T) {
	src := `dd_url: https://app.datadoghq.com
log_level: info`
	dst := `dd_url: https://app.datadoghq.com
log_level: info`

	buf := bytes.NewBuffer([]byte{})
	w := RedactingWriter{
		targetBuf: bufio.NewWriter(buf),
	}

	n, err := w.Write([]byte(src))
	assert.Nil(t, err)
	err = w.Flush()
	assert.Nil(t, err)
	assert.Equal(t, n, len(dst))
	assert.Equal(t, dst, buf.String())
}
