// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package flare

import (
	"bufio"
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRedactingWriter(t *testing.T) {
	input := `dd_url: https://app.datadoghq.com
api_key: aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
proxy: http://user:password@host:port
password: foo
auth_token: bar
# comment to strip
log_level: info`
	redacted := `dd_url: https://app.datadoghq.com
api_key: ***************************aaaaa
proxy: http://user:********@host:port
password: ********
auth_token: ********
log_level: info
`

	buf := bytes.NewBuffer([]byte{})
	r := RedactingWriter{
		targetBuf: bufio.NewWriter(buf),
	}

	n, err := r.Write([]byte(input))
	assert.Nil(t, err)
	err = r.Flush()
	assert.Nil(t, err)
	assert.Equal(t, n, len(redacted))
	assert.Equal(t, buf.String(), redacted)

}
