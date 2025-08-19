// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package util

import (
	"strings"

	taggertypes "github.com/DataDog/datadog-agent/comp/core/tagger/types"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// ServiceNameFromTags returns the standard tag 'service' corresponding to a container
// It returns an empty string if tag not found
func ServiceNameFromTags(ctrName string, taggerEntity taggertypes.EntityID, taggerFunc func(entity taggertypes.EntityID) ([]string, error)) string {
	standardTags, err := taggerFunc(taggerEntity)
	if err != nil {
		log.Debugf("Couldn't get standard tags for container '%s': %v", ctrName, err)
		return ""
	}
	prefix := "service:"
	for _, tag := range standardTags {
		if strings.HasPrefix(tag, prefix) {
			return tag[len(prefix):]
		}
	}
	return ""
}
