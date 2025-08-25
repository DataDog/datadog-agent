// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package softwareinventoryimpl

import (
	"encoding/json"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/inventory/software"
	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
)

// Payload represents the complete software inventory payload sent to the Datadog backend.
// This structure contains all the metadata about software installed on the host system,
// including the hostname, timestamp of collection, and the actual software inventory data.
type Payload struct {
	// Hostname is the name of the host system where the software inventory was collected.
	// This field helps identify which system the inventory data belongs to.
	Hostname string `json:"hostname"`

	// Timestamp is the Unix timestamp (in nanoseconds) when the inventory data was collected.
	// This field provides temporal context for when the software inventory was gathered.
	Timestamp int64 `json:"timestamp"`

	// Metadata contains the actual software inventory data collected from the host system.
	// This includes detailed information about each installed software application.
	Metadata HostSoftware `json:"host_software"`
}

// HostSoftware represents the software inventory data for a single host.
// This structure contains the list of all software entries found on the host system,
// providing a comprehensive view of installed applications and their metadata.
type HostSoftware struct {
	// Software is a list of software entries representing all installed applications
	// found on the host system. Each entry contains detailed information about
	// a specific software installation, including name, version, installation date,
	// publisher, and other relevant metadata.
	Software []software.Entry `json:"software"`
}

// MarshalJSON implements custom JSON marshaling for the Payload type.
// This method ensures proper JSON serialization while avoiding infinite recursion
// that could occur with the default Go JSON marshaling due to the embedded types.
func (p *Payload) MarshalJSON() ([]byte, error) {
	type PayloadAlias Payload
	return json.Marshal((*PayloadAlias)(p))
}

// SplitPayload implements marshaler.AbstractMarshaler#SplitPayload.
// This method is called when the payload needs to be split into smaller chunks
// for transmission. In the case of software inventory, the payload cannot be
// split further as it represents a complete inventory snapshot that should be
// transmitted as a single unit to maintain data integrity.
func (p *Payload) SplitPayload(_ int) ([]marshaler.AbstractMarshaler, error) {
	return nil, fmt.Errorf("could not split inventories software payload any more, payload is too big for intake")
}
