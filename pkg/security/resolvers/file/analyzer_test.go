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
		// Linux x86_64 tests
		{
			name:                "Linux x86_64 static",
			filepath:            "linux_amd64_static",
			expectedType:        model.ELFExecutable,
			expectedABI:         model.Bit64,
			expectedArch:        model.X8664,
			expectedLink:        model.Static,
			expectedUPXPacked:   false,
			expectedGarble:      false,
			expectedCompression: model.NoCompression,
		},
		{
			name:                "Linux x86_64 static UPX",
			filepath:            "linux_amd64_static-upx-best",
			expectedType:        model.ELFExecutable,
			expectedABI:         model.Bit64,
			expectedArch:        model.X8664,
			expectedLink:        model.Static,
			expectedUPXPacked:   true,
			expectedGarble:      false,
			expectedCompression: model.NoCompression,
		},
		{
			name:                "Linux x86_64 dynamic",
			filepath:            "linux_amd64_dyn",
			expectedType:        model.ELFExecutable,
			expectedABI:         model.Bit64,
			expectedArch:        model.X8664,
			expectedLink:        model.Dynamic,
			expectedUPXPacked:   false,
			expectedGarble:      false,
			expectedCompression: model.NoCompression,
		},
		{
			name:                "Linux x86_64 dynamic UPX",
			filepath:            "linux_amd64_dyn-upx-best",
			expectedType:        model.ELFExecutable,
			expectedABI:         model.Bit64,
			expectedArch:        model.X8664,
			expectedLink:        model.Static,
			expectedUPXPacked:   true,
			expectedGarble:      false,
			expectedCompression: model.NoCompression,
		},

		// Linux x86 tests
		{
			name:                "Linux x86 static",
			filepath:            "linux_386_static",
			expectedType:        model.ELFExecutable,
			expectedABI:         model.Bit32,
			expectedArch:        model.X86,
			expectedLink:        model.Static,
			expectedUPXPacked:   false,
			expectedGarble:      false,
			expectedCompression: model.NoCompression,
		},
		{
			name:                "Linux x86 static UPX",
			filepath:            "linux_386_static-upx-best",
			expectedType:        model.ELFExecutable,
			expectedABI:         model.Bit32,
			expectedArch:        model.X86,
			expectedLink:        model.Static,
			expectedUPXPacked:   true,
			expectedGarble:      false,
			expectedCompression: model.NoCompression,
		},
		{
			name:                "Linux x86 dynamic",
			filepath:            "linux_386_dyn",
			expectedType:        model.ELFExecutable,
			expectedABI:         model.Bit32,
			expectedArch:        model.X86,
			expectedLink:        model.Dynamic,
			expectedUPXPacked:   false,
			expectedGarble:      false,
			expectedCompression: model.NoCompression,
		},
		{
			name:                "Linux x86 dynamic UPX",
			filepath:            "linux_386_dyn-upx-best",
			expectedType:        model.ELFExecutable,
			expectedABI:         model.Bit32,
			expectedArch:        model.X86,
			expectedLink:        model.Static,
			expectedUPXPacked:   true,
			expectedGarble:      false,
			expectedCompression: model.NoCompression,
		},

		// Linux ARM64 tests
		{
			name:                "Linux ARM64 static",
			filepath:            "linux_arm64_static",
			expectedType:        model.ELFExecutable,
			expectedABI:         model.Bit64,
			expectedArch:        model.ARM64,
			expectedLink:        model.Static,
			expectedUPXPacked:   false,
			expectedGarble:      false,
			expectedCompression: model.NoCompression,
		},
		{
			name:                "Linux ARM64 static UPX",
			filepath:            "linux_arm64_static-upx-best",
			expectedType:        model.ELFExecutable,
			expectedABI:         model.Bit64,
			expectedArch:        model.ARM64,
			expectedLink:        model.Static,
			expectedUPXPacked:   true,
			expectedGarble:      false,
			expectedCompression: model.NoCompression,
		},
		{
			name:                "Linux ARM64 dynamic",
			filepath:            "linux_arm64_dyn",
			expectedType:        model.ELFExecutable,
			expectedABI:         model.Bit64,
			expectedArch:        model.ARM64,
			expectedLink:        model.Dynamic,
			expectedUPXPacked:   false,
			expectedGarble:      false,
			expectedCompression: model.NoCompression,
		},
		{
			name:                "Linux ARM64 dynamic UPX",
			filepath:            "linux_arm64_dyn-upx-best",
			expectedType:        model.ELFExecutable,
			expectedABI:         model.Bit64,
			expectedArch:        model.ARM64,
			expectedLink:        model.Static,
			expectedUPXPacked:   true,
			expectedGarble:      false,
			expectedCompression: model.NoCompression,
		},

		// Linux ARM tests
		{
			name:                "Linux ARM static",
			filepath:            "linux_arm_static",
			expectedType:        model.ELFExecutable,
			expectedABI:         model.Bit32,
			expectedArch:        model.ARM,
			expectedLink:        model.Static,
			expectedUPXPacked:   false,
			expectedGarble:      false,
			expectedCompression: model.NoCompression,
		},
		{
			name:                "Linux ARM static UPX",
			filepath:            "linux_arm_static-upx-best",
			expectedType:        model.ELFExecutable,
			expectedABI:         model.Bit32,
			expectedArch:        model.ARM,
			expectedLink:        model.Static,
			expectedUPXPacked:   true,
			expectedGarble:      false,
			expectedCompression: model.NoCompression,
		},
		{
			name:                "Linux ARM dynamic",
			filepath:            "linux_arm_dyn",
			expectedType:        model.ELFExecutable,
			expectedABI:         model.Bit32,
			expectedArch:        model.ARM,
			expectedLink:        model.Dynamic,
			expectedUPXPacked:   false,
			expectedGarble:      false,
			expectedCompression: model.NoCompression,
		},
		{
			name:                "Linux ARM dynamic UPX",
			filepath:            "linux_arm_dyn-upx-best",
			expectedType:        model.ELFExecutable,
			expectedABI:         model.Bit32,
			expectedArch:        model.ARM,
			expectedLink:        model.Static,
			expectedUPXPacked:   true,
			expectedGarble:      false,
			expectedCompression: model.NoCompression,
		},

		// Windows x86_64 tests
		{
			name:                "Windows x86_64 static",
			filepath:            "windows_amd64_static_exe",
			expectedType:        model.PEExecutable,
			expectedABI:         model.Bit64,
			expectedArch:        model.X8664,
			expectedLink:        model.Static,
			expectedUPXPacked:   false,
			expectedGarble:      false,
			expectedCompression: model.NoCompression,
		},
		{
			name:                "Windows x86_64 static UPX",
			filepath:            "windows_amd64_static.exe-upx-best",
			expectedType:        model.PEExecutable,
			expectedABI:         model.Bit64,
			expectedArch:        model.X8664,
			expectedLink:        model.Static,
			expectedUPXPacked:   true,
			expectedGarble:      false,
			expectedCompression: model.NoCompression,
		},
		{
			name:                "Windows x86_64 dynamic",
			filepath:            "windows_amd64_dyn_exe",
			expectedType:        model.PEExecutable,
			expectedABI:         model.Bit64,
			expectedArch:        model.X8664,
			expectedLink:        model.Dynamic,
			expectedUPXPacked:   false,
			expectedGarble:      false,
			expectedCompression: model.NoCompression,
		},
		{
			name:                "Windows x86_64 dynamic UPX",
			filepath:            "windows_amd64_dyn.exe-upx-best",
			expectedType:        model.PEExecutable,
			expectedABI:         model.Bit64,
			expectedArch:        model.X8664,
			expectedLink:        model.Static,
			expectedUPXPacked:   true,
			expectedGarble:      false,
			expectedCompression: model.NoCompression,
		},

		// Windows x86 tests
		{
			name:                "Windows x86 static",
			filepath:            "windows_386_static_exe",
			expectedType:        model.PEExecutable,
			expectedABI:         model.Bit32,
			expectedArch:        model.X86,
			expectedLink:        model.Static,
			expectedUPXPacked:   false,
			expectedGarble:      false,
			expectedCompression: model.NoCompression,
		},
		{
			name:                "Windows x86 static UPX",
			filepath:            "windows_386_static.exe-upx-best",
			expectedType:        model.PEExecutable,
			expectedABI:         model.Bit32,
			expectedArch:        model.X86,
			expectedLink:        model.Static,
			expectedUPXPacked:   true,
			expectedGarble:      false,
			expectedCompression: model.NoCompression,
		},
		{
			name:                "Windows x86 dynamic",
			filepath:            "windows_386_dyn_exe",
			expectedType:        model.PEExecutable,
			expectedABI:         model.Bit32,
			expectedArch:        model.X86,
			expectedLink:        model.Dynamic,
			expectedUPXPacked:   false,
			expectedGarble:      false,
			expectedCompression: model.NoCompression,
		},
		{
			name:                "Windows x86 dynamic UPX",
			filepath:            "windows_386_dyn.exe-upx-best",
			expectedType:        model.PEExecutable,
			expectedABI:         model.Bit32,
			expectedArch:        model.X86,
			expectedLink:        model.Static,
			expectedUPXPacked:   true,
			expectedGarble:      false,
			expectedCompression: model.NoCompression,
		},

		// macOS tests
		{
			name:                "macOS x86_64 dynamic",
			filepath:            "macos_amd64_dyn",
			expectedType:        model.MachOExecutable,
			expectedABI:         model.Bit64,
			expectedArch:        model.X8664,
			expectedLink:        model.Dynamic,
			expectedUPXPacked:   false,
			expectedGarble:      false,
			expectedCompression: model.NoCompression,
		},
		{
			name:                "macOS x86_64 dynamic UPX",
			filepath:            "macos_amd64_dyn-upx-best",
			expectedType:        model.MachOExecutable,
			expectedABI:         model.Bit64,
			expectedArch:        model.X8664,
			expectedLink:        model.Dynamic,
			expectedUPXPacked:   true,
			expectedGarble:      false,
			expectedCompression: model.NoCompression,
		},
		{
			name:                "macOS ARM64 dynamic",
			filepath:            "macos_arm64_dyn",
			expectedType:        model.MachOExecutable,
			expectedABI:         model.Bit64,
			expectedArch:        model.ARM64,
			expectedLink:        model.Dynamic,
			expectedUPXPacked:   false,
			expectedGarble:      false,
			expectedCompression: model.NoCompression,
		},
		{
			name:                "macOS ARM64 dynamic UPX",
			filepath:            "macos_arm64_dyn-upx-best",
			expectedType:        model.MachOExecutable,
			expectedABI:         model.Bit64,
			expectedArch:        model.ARM64,
			expectedLink:        model.Dynamic,
			expectedUPXPacked:   true,
			expectedGarble:      false,
			expectedCompression: model.NoCompression,
		},

		// macOS garble tests
		{
			name:                "macOS x86_64 dynamic garble",
			filepath:            "macos_amd64_dyn_garble",
			expectedType:        model.MachOExecutable,
			expectedABI:         model.Bit64,
			expectedArch:        model.X8664,
			expectedLink:        model.Dynamic,
			expectedUPXPacked:   false,
			expectedGarble:      true,
			expectedCompression: model.NoCompression,
		},
		{
			name:                "macOS x86_64 dynamic garble UPX",
			filepath:            "macos_amd64_dyn_garble-upx-best",
			expectedType:        model.MachOExecutable,
			expectedABI:         model.Bit64,
			expectedArch:        model.X8664,
			expectedLink:        model.Dynamic,
			expectedUPXPacked:   true,
			expectedGarble:      true,
			expectedCompression: model.NoCompression,
		},
		{
			name:                "macOS ARM64 dynamic garble",
			filepath:            "macos_arm64_dyn_garble",
			expectedType:        model.MachOExecutable,
			expectedABI:         model.Bit64,
			expectedArch:        model.ARM64,
			expectedLink:        model.Dynamic,
			expectedUPXPacked:   false,
			expectedGarble:      true,
			expectedCompression: model.NoCompression,
		},
		{
			name:                "macOS ARM64 dynamic garble UPX",
			filepath:            "macos_arm64_dyn_garble-upx-best",
			expectedType:        model.MachOExecutable,
			expectedABI:         model.Bit64,
			expectedArch:        model.ARM64,
			expectedLink:        model.Dynamic,
			expectedUPXPacked:   true,
			expectedGarble:      true,
			expectedCompression: model.NoCompression,
		},

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

		// Linux x86_64 garble tests
		{
			name:                "Linux x86_64 static garble",
			filepath:            "linux_amd64_static_garble",
			expectedType:        model.ELFExecutable,
			expectedABI:         model.Bit64,
			expectedArch:        model.X8664,
			expectedLink:        model.Static,
			expectedUPXPacked:   false,
			expectedGarble:      true,
			expectedCompression: model.NoCompression,
		},
		{
			name:                "Linux x86_64 static garble UPX",
			filepath:            "linux_amd64_static_garble-upx-best",
			expectedType:        model.ELFExecutable,
			expectedABI:         model.Bit64,
			expectedArch:        model.X8664,
			expectedLink:        model.Static,
			expectedUPXPacked:   true,
			expectedGarble:      true,
			expectedCompression: model.NoCompression,
		},

		// Linux x86 garble tests
		{
			name:                "Linux x86 static garble",
			filepath:            "linux_386_static_garble",
			expectedType:        model.ELFExecutable,
			expectedABI:         model.Bit32,
			expectedArch:        model.X86,
			expectedLink:        model.Static,
			expectedUPXPacked:   false,
			expectedGarble:      true,
			expectedCompression: model.NoCompression,
		},
		{
			name:                "Linux x86 static garble UPX",
			filepath:            "linux_386_static_garble-upx-upx-best",
			expectedType:        model.ELFExecutable,
			expectedABI:         model.Bit32,
			expectedArch:        model.X86,
			expectedLink:        model.Static,
			expectedUPXPacked:   true,
			expectedGarble:      true,
			expectedCompression: model.NoCompression,
		},

		// Linux ARM64 garble tests
		{
			name:                "Linux ARM64 static garble",
			filepath:            "linux_arm64_static_garble",
			expectedType:        model.ELFExecutable,
			expectedABI:         model.Bit64,
			expectedArch:        model.ARM64,
			expectedLink:        model.Static,
			expectedUPXPacked:   false,
			expectedGarble:      true,
			expectedCompression: model.NoCompression,
		},
		{
			name:                "Linux ARM64 static garble UPX",
			filepath:            "linux_arm64_static_garble-upx-best",
			expectedType:        model.ELFExecutable,
			expectedABI:         model.Bit64,
			expectedArch:        model.ARM64,
			expectedLink:        model.Static,
			expectedUPXPacked:   true,
			expectedGarble:      true,
			expectedCompression: model.NoCompression,
		},

		// Linux ARM garble tests
		{
			name:                "Linux ARM static garble",
			filepath:            "linux_arm_static_garble",
			expectedType:        model.ELFExecutable,
			expectedABI:         model.Bit32,
			expectedArch:        model.ARM,
			expectedLink:        model.Static,
			expectedUPXPacked:   false,
			expectedGarble:      true,
			expectedCompression: model.NoCompression,
		},
		{
			name:                "Linux ARM static garble UPX",
			filepath:            "linux_arm_static_garble-upx-best",
			expectedType:        model.ELFExecutable,
			expectedABI:         model.Bit32,
			expectedArch:        model.ARM,
			expectedLink:        model.Static,
			expectedUPXPacked:   true,
			expectedGarble:      true,
			expectedCompression: model.NoCompression,
		},

		// Windows x86_64 garble tests
		{
			name:                "Windows x86_64 static garble",
			filepath:            "windows_amd64_static_garble_exe",
			expectedType:        model.PEExecutable,
			expectedABI:         model.Bit64,
			expectedArch:        model.X8664,
			expectedLink:        model.Static,
			expectedUPXPacked:   false,
			expectedGarble:      true,
			expectedCompression: model.NoCompression,
		},
		{
			name:                "Windows x86_64 static garble UPX",
			filepath:            "windows_amd64_static_garble.exe-upx-best",
			expectedType:        model.PEExecutable,
			expectedABI:         model.Bit64,
			expectedArch:        model.X8664,
			expectedLink:        model.Static,
			expectedUPXPacked:   true,
			expectedGarble:      true,
			expectedCompression: model.NoCompression,
		},

		// Windows x86 garble tests
		{
			name:                "Windows x86 static garble",
			filepath:            "windows_386_static_garble_exe",
			expectedType:        model.PEExecutable,
			expectedABI:         model.Bit32,
			expectedArch:        model.X86,
			expectedLink:        model.Static,
			expectedUPXPacked:   false,
			expectedGarble:      true,
			expectedCompression: model.NoCompression,
		},
		{
			name:                "Windows x86 static garble UPX",
			filepath:            "windows_386_static_garble.exe-upx-best",
			expectedType:        model.PEExecutable,
			expectedABI:         model.Bit32,
			expectedArch:        model.X86,
			expectedLink:        model.Static,
			expectedUPXPacked:   true,
			expectedGarble:      true,
			expectedCompression: model.NoCompression,
		},

		// Windows ARM64 garble tests
		{
			name:                "Windows ARM64 static garble",
			filepath:            "windows_arm64_static_garble_exe",
			expectedType:        model.PEExecutable,
			expectedABI:         model.Bit64,
			expectedArch:        model.ARM64,
			expectedLink:        model.Static,
			expectedUPXPacked:   false,
			expectedGarble:      true,
			expectedCompression: model.NoCompression,
		},

		// Windows ARM garble tests
		{
			name:                "Windows ARM static garble",
			filepath:            "windows_arm_static_garble_exe",
			expectedType:        model.PEExecutable,
			expectedABI:         model.Bit32,
			expectedArch:        model.ARM,
			expectedLink:        model.Static,
			expectedUPXPacked:   false,
			expectedGarble:      true,
			expectedCompression: model.NoCompression,
		},

		// Windows ARM64 tests
		{
			name:                "Windows ARM64 static",
			filepath:            "windows_arm64_static_exe",
			expectedType:        model.PEExecutable,
			expectedABI:         model.Bit64,
			expectedArch:        model.ARM64,
			expectedLink:        model.Static,
			expectedUPXPacked:   false,
			expectedGarble:      false,
			expectedCompression: model.NoCompression,
		},

		// Windows ARM tests
		{
			name:                "Windows ARM static",
			filepath:            "windows_arm_static_exe",
			expectedType:        model.PEExecutable,
			expectedABI:         model.Bit32,
			expectedArch:        model.ARM,
			expectedLink:        model.Static,
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
