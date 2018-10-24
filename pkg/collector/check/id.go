// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package check

import (
	"fmt"
	"hash/fnv"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
)

// ID is the representation of the unique ID of a Check instance
type ID string

// Identify returns an unique ID for a check and its configuration
func Identify(check Check, instance integration.Data, initConfig integration.Data) ID {
	return BuildID(check.String(), check.ExtraString(), instance, initConfig)
}

// BuildID returns an unique ID for a check name and its configuration
func BuildID(checkName string, extraID string, instance, initConfig integration.Data) ID {
	h := fnv.New64()
	h.Write([]byte(instance))
	h.Write([]byte(initConfig))

	id := fmt.Sprintf("%s:%x %s", checkName, h.Sum64(), extraID)
	return ID(id)
}

// IDToCheckName returns the check name from a check ID
func IDToCheckName(id ID) string {
	return strings.SplitN(string(id), ":", 2)[0]
}
