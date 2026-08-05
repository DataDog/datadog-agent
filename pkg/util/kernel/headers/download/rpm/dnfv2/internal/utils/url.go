// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package utils

import (
	"net/url"
	"path"
	"strings"
)

// URLJoinPath joins a base URL with the `elems`
func URLJoinPath(base string, elems ...string) (string, error) {
	u, err := url.Parse(base)
	if err != nil {
		return "", err
	}

	u.Path = path.Join(append([]string{u.Path}, elems...)...)
	return u.String(), nil
}

// URLHasSuffix returns whether the path of rawUrl has the provided suffix.
func URLHasSuffix(rawURL string, suffix string) bool {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return strings.HasSuffix(rawURL, suffix)
	}

	return strings.HasSuffix(parsed.Path, suffix)
}
