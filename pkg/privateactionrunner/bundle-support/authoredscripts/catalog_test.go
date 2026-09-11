// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !windows

package authoredscripts

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDescriptorValidate(t *testing.T) {
	validDescriptor := Descriptor{
		Package: "com.datadoghq.authoredscripts.test",
		Version: "0.0.1",
		URL:     "oci://registry.example.test/authored-script:0.0.1",
		SHA256:  "ea7829a6ebdaa464eb4fbfff4c72e6e63176df58a430a4b0b8dfb66f0e57149c",
	}
	tests := []struct {
		name        string
		descriptor  Descriptor
		errorString string
	}{
		{name: "valid", descriptor: validDescriptor},
		{name: "package", descriptor: Descriptor{Version: validDescriptor.Version, URL: validDescriptor.URL, SHA256: validDescriptor.SHA256}, errorString: "package is required"},
		{name: "version", descriptor: Descriptor{Package: validDescriptor.Package, URL: validDescriptor.URL, SHA256: validDescriptor.SHA256}, errorString: "version is required"},
		{name: "URL", descriptor: Descriptor{Package: validDescriptor.Package, Version: validDescriptor.Version, SHA256: validDescriptor.SHA256}, errorString: "URL is required"},
		{name: "SHA-256", descriptor: Descriptor{Package: validDescriptor.Package, Version: validDescriptor.Version, URL: validDescriptor.URL}, errorString: "SHA-256 digest is required"},
		{name: "invalid SHA-256", descriptor: Descriptor{Package: validDescriptor.Package, Version: validDescriptor.Version, URL: validDescriptor.URL, SHA256: "not-a-digest"}, errorString: "invalid authored-script SHA-256 digest"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.descriptor.Validate()
			if test.errorString == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, test.errorString)
		})
	}
}
