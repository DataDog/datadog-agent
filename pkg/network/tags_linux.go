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
	// ConnTagGnuTLS is the tag for GnuTLS connections
	ConnTagGnuTLS = http.GnuTLS
	// ConnTagOpenSSL is the tag for OpenSSL connections
	ConnTagOpenSSL = http.OpenSSL
	// ConnTagGo is the tag for GO TLS connections
	ConnTagGo = http.Go
	// ConnTagTLS is the tag for TLS connections in general
	ConnTagTLS = http.TLS
	// ConnTagIstio is the tag for Istio TLS connections
	ConnTagIstio = http.Istio
	// ConnTagNodeJS is the tag for NodeJS TLS connections
	ConnTagNodeJS = http.NodeJS
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

// IsTLSTag return if the tag is a TLS tag
func IsTLSTag(staticTags uint64) bool {
	return staticTags&(ConnTagGnuTLS|ConnTagOpenSSL|ConnTagGo|ConnTagTLS|ConnTagIstio|ConnTagNodeJS) > 0
}
