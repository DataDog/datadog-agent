// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package inventoryhaagentimpl

import (
	"github.com/DataDog/datadog-agent/pkg/util/scrubber"
)

func scrub(s string) string {
	// Errors come from internal use of a Reader interface. Since we are reading from a buffer, no errors
	// are possible.
	scrubString, _ := scrubber.ScrubString(s)
	return scrubString
}

func copyAndScrub(o haAgentMetadata) haAgentMetadata {
	data := make(haAgentMetadata)
	for k, v := range o {
		if s, ok := v.(string); ok {
			data[k] = scrub(s)
		} else {
			data[k] = v
		}
	}

	return data
}
