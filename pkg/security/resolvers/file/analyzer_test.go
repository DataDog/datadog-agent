// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package file

import (
	"path"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

func TestAnalyzeFile(t *testing.T) {
	tests := []struct {
		name                string
		filepath            string
		expectedType        model.FileType
		expectedABI         model.ABI
		expectedArch        model.Architecture
		expectedLink        model.LinkageType
		expectedUPXPacked   bool
		expectedGarble      bool
		expectedCompression model.CompressionType
	}{
		// Archive and text file tests
		{
			name:                "7z archive",
			filepath:            "test_7z",
			expectedType:        model.Compressed,
			expectedABI:         model.UnknownABI,
			expectedArch:        model.UnknownArch,
			expectedLink:        model.None,
			expectedUPXPacked:   false,
			expectedGarble:      false,
			expectedCompression: model.SevenZip,
		},
		{
			name:                "bzip2 archive",
			filepath:            "test_bz2",
			expectedType:        model.Compressed,
			expectedABI:         model.UnknownABI,
			expectedArch:        model.UnknownArch,
			expectedLink:        model.None,
			expectedUPXPacked:   false,
			expectedGarble:      false,
			expectedCompression: model.BZip2,
		},
		{
			name:                "gzip archive",
			filepath:            "test_gz",
			expectedType:        model.Compressed,
			expectedABI:         model.UnknownABI,
			expectedArch:        model.UnknownArch,
			expectedLink:        model.None,
			expectedUPXPacked:   false,
			expectedGarble:      false,
			expectedCompression: model.GZip,
		},
		{
			name:                "shell script",
			filepath:            "test_sh",
			expectedType:        model.ShellScript,
			expectedABI:         model.UnknownABI,
			expectedArch:        model.UnknownArch,
			expectedLink:        model.None,
			expectedUPXPacked:   false,
			expectedGarble:      false,
			expectedCompression: model.NoCompression,
		},
		{
			name:                "shell script with .exe extension",
			filepath:            "test_sh_exe",
			expectedType:        model.ShellScript,
			expectedABI:         model.UnknownABI,
			expectedArch:        model.UnknownArch,
			expectedLink:        model.None,
			expectedUPXPacked:   false,
			expectedGarble:      false,
			expectedCompression: model.NoCompression,
		},
		{
			name:                "tar archive",
			filepath:            "test_tar",
			expectedType:        model.Binary,
			expectedABI:         model.UnknownABI,
			expectedArch:        model.UnknownArch,
			expectedLink:        model.None,
			expectedUPXPacked:   false,
			expectedGarble:      false,
			expectedCompression: model.NoCompression,
		},
		{
			name:                "tgz archive",
			filepath:            "test_tgz",
			expectedType:        model.Compressed,
			expectedABI:         model.UnknownABI,
			expectedArch:        model.UnknownArch,
			expectedLink:        model.None,
			expectedUPXPacked:   false,
			expectedGarble:      false,
			expectedCompression: model.GZip,
		},
		{
			name:                "text file",
			filepath:            "test_txt",
			expectedType:        model.Text,
			expectedABI:         model.UnknownABI,
			expectedArch:        model.UnknownArch,
			expectedLink:        model.None,
			expectedUPXPacked:   false,
			expectedGarble:      false,
			expectedCompression: model.NoCompression,
		},
		{
			name:                "xz archive",
			filepath:            "test_xz",
			expectedType:        model.Compressed,
			expectedABI:         model.UnknownABI,
			expectedArch:        model.UnknownArch,
			expectedLink:        model.None,
			expectedUPXPacked:   false,
			expectedGarble:      false,
			expectedCompression: model.XZ,
		},
		{
			name:                "zip archive",
			filepath:            "test_zip",
			expectedType:        model.Compressed,
			expectedABI:         model.UnknownABI,
			expectedArch:        model.UnknownArch,
			expectedLink:        model.None,
			expectedUPXPacked:   false,
			expectedGarble:      false,
			expectedCompression: model.Zip,
		},

		// Empty file test
		{
			name:                "Empty file",
			filepath:            "test_empty",
			expectedType:        model.Empty,
			expectedABI:         model.UnknownABI,
			expectedArch:        model.UnknownArch,
			expectedLink:        model.None,
			expectedUPXPacked:   false,
			expectedGarble:      false,
			expectedCompression: model.NoCompression,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info, err := AnalyzeFile(path.Join("testdata", tt.filepath), nil, true)
			assert.NoError(t, err)

			if err == nil {
				assert.Equal(t, int(tt.expectedType), info.Type, "Type mismatch")
				assert.Equal(t, int(tt.expectedABI), info.ABI, "ABI mismatch")
				assert.Equal(t, int(tt.expectedArch), info.Architecture, "Architecture mismatch")
				assert.Equal(t, int(tt.expectedLink), info.Linkage, "Linkage mismatch")
				assert.Equal(t, tt.expectedUPXPacked, info.IsUPXPacked, "IsUPXPacked mismatch")
				assert.Equal(t, tt.expectedGarble, info.IsGarbleObfuscated, "IsGarbleObfuscated mismatch")
				assert.Equal(t, int(tt.expectedCompression), info.Compression, "Compression type mismatch")
			}
		})
	}
}

func TestAnalyzeFileNonExistent(t *testing.T) {
	_, err := AnalyzeFile("testdata/nonexistent_file", nil, true)
	assert.Error(t, err, "Expected error for non-existent file")
}
