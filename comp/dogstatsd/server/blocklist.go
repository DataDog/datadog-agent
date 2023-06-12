// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package server

import (
	"strings"
)

type blocklist struct {
	data        []string
	matchPrefix bool
}

func newBlocklist(data []string, matchPrefix bool) blocklist {
	return blocklist{
		data:        data,
		matchPrefix: matchPrefix,
	}
}

func (b *blocklist) test(name string) bool {
	if b.matchPrefix {
		for _, item := range b.data {
			if strings.HasPrefix(name, item) {
				return true
			}
		}
	} else {
		for _, item := range b.data {
			if name == item {
				return true
			}
		}
	}

	return false
}
