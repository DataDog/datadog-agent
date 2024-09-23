// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	_ "embed" //nolint:revive
	"sort"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"gopkg.in/ini.v1"
)

//go:embed peer_tags.ini
var peerTagFile []byte

// basePeerTags is the base set of peer tag precursors (tags from which peer tags
// are derived) we aggregate on when peer tag aggregation is enabled.
var basePeerTags = func() []string {
	var precursors []string = []string{"_dd.base_service"}

	cfg, err := ini.Load(peerTagFile)
	if err != nil {
		log.Error("Error loading file for peer tags: ", err)
		return precursors
	}
	peerTags := cfg.Section("dd.apm.peer.tags").Keys()

	for _, t := range peerTags {
		ps := strings.Split(t.Value(), ",")
		precursors = append(precursors, ps...)
	}
	sort.Strings(precursors)

	return precursors
}()

func preparePeerTags(tags []string) []string {
	if len(tags) == 0 {
		return nil
	}
	var deduped []string
	seen := make(map[string]struct{})
	for _, t := range tags {
		if _, ok := seen[t]; !ok {
			seen[t] = struct{}{}
			deduped = append(deduped, t)
		}
	}
	sort.Strings(deduped)
	return deduped
}
