// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build secrets,!windows

package secrets

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWrongPath(t *testing.T) {
	require.NotNil(t, checkRights("does not exists"))
}

func TestGroupOtherRights(t *testing.T) {
	tmpfile, err := ioutil.TempFile("", "agent-collector-test")
	require.Nil(t, err)

	// file exists
	require.NotNil(t, checkRights("/does not exists"))

	require.Nil(t, os.Chmod(tmpfile.Name(), 0700))
	require.Nil(t, checkRights(tmpfile.Name()))

	// we should at least be able to execute it
	require.Nil(t, os.Chmod(tmpfile.Name(), 0100))
	require.Nil(t, checkRights(tmpfile.Name()))

	// owner have exec right
	require.Nil(t, os.Chmod(tmpfile.Name(), 0600))
	require.NotNil(t, checkRights(tmpfile.Name()))

	// group should have no right
	require.Nil(t, os.Chmod(tmpfile.Name(), 0710))
	require.NotNil(t, checkRights(tmpfile.Name()))

	// other should have no right
	require.Nil(t, os.Chmod(tmpfile.Name(), 0701))
	require.NotNil(t, checkRights(tmpfile.Name()))
}
