// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flare

import (
	"archive/zip"
)

// Flare contains all the information sent by the Datadog Agent when using the Flare command
// zipFileMap is a mapping between filenames and *zip.File obtained from zip.Reader struct.
type Flare struct {
	Email        string
	ZipFileMap   map[string]*zip.File
	AgentVersion string
	Hostname     string
}
