// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package scrubber

import (
	"io/ioutil"
	"os"
	"path"
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"
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
	filename := path.Join(t.TempDir(), "redacted")

	redacted := `dd_url: https://app.datadoghq.com
api_key: ***************************aaaaa
proxy: http://user:********@host:1234
password: ********
auth_token: ********
log_level: info`

	w, err := NewWriter(filename, os.ModePerm, true)
	require.NoError(t, err)

	n, err := w.Write([]byte(input))
	require.NoError(t, err)
	require.Equal(t, len(input), n)

	err = w.Flush()
	require.NoError(t, err)

	got, err := ioutil.ReadFile(filename)
	require.NoError(t, err)

	require.Equal(t, redacted, string(got))
}

func TestRedactingWriterReplacers(t *testing.T) {
	filename := path.Join(t.TempDir(), "redacted")

	redacted := `dd_url: https://app.datadoghq.com
api_key: ***************************aaaaa
proxy: http://USERISREDACTEDTOO:********@foo:bar
password: ********
auth_token: ********
log_level: info`

	w, err := NewWriter(filename, os.ModePerm, true)
	require.NoError(t, err)

	w.RegisterReplacer(Replacer{
		Regex: regexp.MustCompile(`user`),
		ReplFunc: func(s []byte) []byte {
			return []byte("USERISREDACTEDTOO")
		},
	})
	w.RegisterReplacer(Replacer{
		Regex: regexp.MustCompile(`@.*\:[0-9]+`),
		ReplFunc: func(s []byte) []byte {
			return []byte("@foo:bar")
		},
	})

	n, err := w.Write([]byte(input))
	require.NoError(t, err)
	require.Equal(t, len(input), n)

	err = w.Flush()
	require.NoError(t, err)

	got, err := ioutil.ReadFile(filename)
	require.NoError(t, err)

	require.Equal(t, redacted, string(got))

}
func TestRedactingNothing(t *testing.T) {
	filename := path.Join(t.TempDir(), "redacted")

	// nothing to redact here
	content := `dd_url: https://app.datadoghq.com
log_level: info`

	w, err := NewWriter(filename, os.ModePerm, true)
	require.NoError(t, err)

	n, err := w.Write([]byte(content))
	require.NoError(t, err)
	require.Equal(t, n, len(content))

	err = w.Flush()
	require.NoError(t, err)

	got, err := ioutil.ReadFile(filename)
	require.NoError(t, err)

	require.Equal(t, content, string(got))
}
