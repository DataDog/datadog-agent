// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package gitleaks

import (
	"io/ioutil"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// multiple lines here, and prefixes/suffixes to each secret, in order to
// check that the redaction is applying to the correct bits.
const (
	dirty = `hello
Split the data by line.  Note that this does not copy the original data.
fake dynatrace secret is here: dt0c01.ST2EY72KQINMH574WMNVI7YN.G3DFPBEJYMODIDAEX454M7YWBUVEFOWKPRVMWFASS64NFH52PX6BNDVFFM572RZM!
fake age secret is AGE-SECRET-KEY-1QQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQ <-- there
last line`

	clean = `hello
Split the data by line.  Note that this does not copy the original data.
fake dynatrace secret is here: ************************************************************************************************!
fake age secret is ************************************************************************** <-- there
last line`
)

func TestScrubBytes(t *testing.T) {
	scrubber, err := New()
	require.NoError(t, err)

	scrubbed, err := scrubber.ScrubBytes([]byte(dirty))

	require.NoError(t, err)
	require.Equal(t, clean, string(scrubbed))
}

func TestScrubFile(t *testing.T) {
	dir := t.TempDir()
	filename := filepath.Join(dir, "test.yml")
	ioutil.WriteFile(filename, []byte(dirty), 0666)

	scrubber, err := New()
	require.NoError(t, err)

	res, err := scrubber.ScrubFile(filename)
	require.NoError(t, err)
	require.Equal(t, clean, string(res))
}

func TestScrubLine_Gitter(t *testing.T) {
	scrubber, err := New()
	require.NoError(t, err)

	dirty := "gitter_api_secret = abcdabcdabcdabcdabcdabcdabcdabcdabcdabcd"
	clean := "************************************************************" // XXX!!
	scrubbed := scrubber.ScrubLine(dirty)

	require.Equal(t, clean, string(scrubbed))
}

func TestScrubLine_Age(t *testing.T) {
	scrubber, err := New()
	require.NoError(t, err)

	dirty := "look -> AGE-SECRET-KEY-1QQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQ <-"
	clean := "look -> ************************************************************************** <-"
	scrubbed := scrubber.ScrubLine(dirty)

	require.Equal(t, clean, string(scrubbed))
}

func TestScrubbing(t *testing.T) {
	test := func(lb, cb int, dirty, clean string) func(t *testing.T) {
		return func(t *testing.T) {
			t.Run("ScrubBytes", func(t *testing.T) {
				lineBase = lb
				colBase = cb
				scrubber, err := New()
				require.NoError(t, err)

				scrubbed, err := scrubber.ScrubBytes([]byte(dirty))

				require.NoError(t, err)
				require.Equal(t, clean, string(scrubbed))
			})
		}
	}

	// XXX the first two arguments to `test(..)` set the lineBase and colBase,
	// meaning whether the StartLine/EndLine are 0 or 1-indexed, and whether
	// StartColumn/EndColumn are 0 or 1-indexed.  As you can see, this varies
	// depending on the number of lines in the input, and sometimes they are
	// 2-indexed!
	//
	// I suspect there are off-by-one errors in
	//  https://github.com/zricethezav/gitleaks/blob/afc89f91c0795f9763920d40848d770e44341b06/detect/location.go#L13
	// but I don't understand well enough what it's trying to do to be sure.

	t.Run("multiline-multisecret", test(0, 2,
		`hello
Split the data by line.  Note that this does not copy the original data.
fake dynatrace secret is here: dt0c01.ST2EY72KQINMH574WMNVI7YN.G3DFPBEJYMODIDAEX454M7YWBUVEFOWKPRVMWFASS64NFH52PX6BNDVFFM572RZM!
fake age secret is AGE-SECRET-KEY-1QQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQ <-- there
last line`,
		`hello
Split the data by line.  Note that this does not copy the original data.
fake dynatrace secret is here: ************************************************************************************************!
fake age secret is ************************************************************************** <-- there
last line`,
	))

	t.Run("multiline-nosecret", test(0, 2,
		"Hello.\nThere are no secrets here.\nEnjoy!",
		"Hello.\nThere are no secrets here.\nEnjoy!",
	))

	t.Run("3line-gitter", test(0, 2,
		"Hello.\n>> gitter_api_secret = abcdabcdabcdabcdabcdabcdabcdabcdabcdabcd  <<\nEnjoy!",
		// NOTE: this pattern seems to require and match the first trailing space
		"Hello.\n>> ************************************************************* <<\nEnjoy!",
	))

	t.Run("2line-gitter-line2", test(0, 1,
		">> gitter_api_secret = abcdabcdabcdabcdabcdabcdabcdabcdabcdabcd  <<\nEnjoy!",
		">> ************************************************************* <<\nEnjoy!",
	))

	t.Run("2line-gitter-line1", test(0, 2,
		"Hello\n>> gitter_api_secret = abcdabcdabcdabcdabcdabcdabcdabcdabcdabcd  <<",
		"Hello\n>> ************************************************************* <<",
	))

	t.Run("single-line-dynatrace", test(1, 1,
		`fake dynatrace secret is here: dt0c01.ST2EY72KQINMH574WMNVI7YN.G3DFPBEJYMODIDAEX454M7YWBUVEFOWKPRVMWFASS64NFH52PX6BNDVFFM572RZM!`,
		`fake dynatrace secret is here: ************************************************************************************************!`,
	))
	t.Run("single-line-gitter", test(1, 1,
		`gitter_api_secret = abcdabcdabcdabcdabcdabcdabcdabcdabcdabcd`,
		`************************************************************`,
	))
}
