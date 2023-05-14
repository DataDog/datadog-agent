// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package check

import (
	"encoding/binary"
	"fmt"
	"hash/fnv"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
)

// ID is the representation of the unique ID of a Check instance
type ID string

// BuildID returns an unique ID for a check name and its configuration
func BuildID(checkName string, integrationConfigDigest uint64, instance, initConfig integration.Data) ID {
	// Hash is returned in BigEndian
	digestBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(digestBytes, integrationConfigDigest)

	h := fnv.New64()
	_, _ = h.Write(digestBytes)
	_, _ = h.Write([]byte(instance))
	_, _ = h.Write([]byte(initConfig))
	name := instance.GetNameForInstance()

	if name != "" {
		return ID(fmt.Sprintf("%s:%s:%x", checkName, name, h.Sum64()))
	}

	return ID(fmt.Sprintf("%s:%x", checkName, h.Sum64()))
}

// IDToCheckName returns the check name from a check ID
func IDToCheckName(id ID) string {
	return strings.SplitN(string(id), ":", 2)[0]
}
