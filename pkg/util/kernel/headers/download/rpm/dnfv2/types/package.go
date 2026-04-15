// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package types

// Metadata is a DNFv2 metadata
type Metadata struct {
	Packages []*Package `xml:"package"`
}

// Package is a DNFv2 package
type Package struct {
	Type     string     `xml:"type,attr"`
	Name     string     `xml:"name"`
	Arch     string     `xml:"arch"`
	Checksum Checksum   `xml:"checksum"`
	Location Location   `xml:"location"`
	Provides []Provides `xml:"format>provides>entry"`
}

// Version is a DNFv2 version
type Version struct {
	Epoch string `xml:"epoch,attr"`
	Ver   string `xml:"ver,attr"`
	Rel   string `xml:"rel,attr"`
}

// Provides is a DNFv2 provides
type Provides struct {
	Name string `xml:"name,attr"`
	Version
}
