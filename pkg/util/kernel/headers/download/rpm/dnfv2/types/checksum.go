// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package types

// Checksum is a DNFv2 checksum
type Checksum struct {
	Hash string `xml:",chardata"`
	Type string `xml:"type,attr"`
}
