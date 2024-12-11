// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO(NDM) Fix revive linter
package metadata

const (
	ifNameOID = "1.3.6.1.2.1.31.1.1.1.1"
)

// resourceIndex is a map of fields to be used to get a list of indexes for a specific resource
var resourceIndex = map[string]string{
	"interface": ifNameOID,
}

// GetIndexOIDForResource returns the index OID for a specific resource
func GetIndexOIDForResource(resource string) string {
	return resourceIndex[resource]
}
