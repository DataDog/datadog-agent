// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package types

// Repomd is a DNFv2 repo data collection
type Repomd struct {
	Data []RepomdData `xml:"data"`
}

// RepomdData is a DNFv2 repo data
type RepomdData struct {
	Type         string   `xml:"type,attr"`
	Size         int      `xml:"size"`
	OpenSize     int      `xml:"open-size"`
	Location     Location `xml:"location"`
	Checksum     Checksum `xml:"checksum"`
	OpenChecksum Checksum `xml:"open-checksum"`
}

// Location is a DNFv2 location
type Location struct {
	Href string `xml:"href,attr"`
}

// MetaLink is a DNFv2 metalink
type MetaLink struct {
	Files MetaLinkFiles `xml:"files"`
}

// MetaLinkFiles is a DNFv2 collection of MetaLinkFiles
type MetaLinkFiles struct {
	Files []MetaLinkFile `xml:"file"`
}

// MetaLinkFile is a DNFv2 metalink file
type MetaLinkFile struct {
	Name      string                `xml:"name,attr"`
	Resources MetaLinkFileResources `xml:"resources"`
}

// MetaLinkFileResources is a DNFv2 meta link file resources
type MetaLinkFileResources struct {
	Urls []MetaLinkFileResourceURL `xml:"url"`
}

// MetaLinkFileResourceURL is a DNFv2 meta link file resource URL
type MetaLinkFileResourceURL struct {
	Protocol   string `xml:"protocol,attr"`
	Preference int    `xml:"preference,attr"`
	URL        string `xml:",chardata"`
}
