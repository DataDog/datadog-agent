// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package network

import (
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http"
)

const (
	ConnTagGnuTLS  = http.GnuTLS
	ConnTagOpenSSL = http.OpenSSL
	ConnTagGo      = http.Go
	ConnTagJava    = http.Java
	ConnTagTLS     = http.TLS
)

// GetStaticTags return the string list of static tags from network.ConnectionStats.Tags
func GetStaticTags(staticTags uint64) (tags []string) {
	for tag, str := range http.StaticTags {
		if (staticTags & tag) > 0 {
			tags = append(tags, str)
		}
	}
	return tags
}

func IsTLSTag(staticTags uint64) bool {
	return staticTags&(ConnTagGnuTLS|ConnTagOpenSSL|ConnTagGo|ConnTagJava|ConnTagTLS) > 0
}
