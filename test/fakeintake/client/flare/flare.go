// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flare

import (
	"archive/zip"
	"path/filepath"
)

// Flare contains all the information sent by the Datadog Agent when using the Flare command
// zipFileMap is a mapping between filenames and *zip.File obtained from zip.Reader struct.
type Flare struct {
	Email        string
	ZipFileMap   map[string]*zip.File
	AgentVersion string
	Hostname     string
}

// FileExists returns true if the filename exists in the flare archive
func (flare *Flare) FileExists(filename string) bool {
	fullpath := filepath.Join(flare.Hostname, filename)

	_, found := flare.ZipFileMap[fullpath]
	return found
}
