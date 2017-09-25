// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package metadata

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/metadata/v5"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/util"
)

// HostCollector fills and sends the old metadata payload used in the
// Agent v5
type HostCollector struct{}

// Send collects the data needed and submits the payload
func (hp *HostCollector) Send(s *serializer.Serializer) error {
	hostname, _ := util.GetHostname()

	payload := v5.GetPayload(hostname)
	if err := s.SendMetadata(payload); err != nil {
		return fmt.Errorf("unable to submit host metadata payload, %s", err)
	}
	return nil
}

func init() {
	catalog["host"] = new(HostCollector)
}
