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
	//nolint:revive // TODO(NET) Fix revive linter
	ConnTagGnuTLS = http.GnuTLS
	//nolint:revive // TODO(NET) Fix revive linter
	ConnTagOpenSSL = http.OpenSSL
	//nolint:revive // TODO(NET) Fix revive linter
	ConnTagGo = http.Go
	//nolint:revive // TODO(NET) Fix revive linter
	ConnTagJava = http.Java
	//nolint:revive // TODO(NET) Fix revive linter
	ConnTagTLS = http.TLS
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

//nolint:revive // TODO(NET) Fix revive linter
func IsTLSTag(staticTags uint64) bool {
	return staticTags&(ConnTagGnuTLS|ConnTagOpenSSL|ConnTagGo|ConnTagJava|ConnTagTLS) > 0
}
