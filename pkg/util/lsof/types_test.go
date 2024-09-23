// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package lsof

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFilesString(t *testing.T) {
	files := Files{
		{"3", "REG", "r-", "-rwx------", 0, "/some/file"},
		{"mem", "REG", "r-xp", "-r--------", 8, "/usr/lib/aarch64-linux-gnu/libutil.so.1"},
	}

	expected := `FD  Type Size OpenPerm FilePerm   Name                                    
3   REG  0    r-       -rwx------ /some/file                              
mem REG  8    r-xp     -r-------- /usr/lib/aarch64-linux-gnu/libutil.so.1 
`

	require.Equal(t, expected, files.String())
}
