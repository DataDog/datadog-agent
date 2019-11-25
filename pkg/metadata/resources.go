// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package metadata

import (
	"errors"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/metadata/resources"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/util"
)

// ResourcesCollector sends the old metadata payload used in the
// Agent v5
type ResourcesCollector struct{}

// Send collects the data needed and submits the payload
func (rp *ResourcesCollector) Send(s *serializer.Serializer) error {
	hostname, _ := util.GetHostname()

	res := resources.GetPayload(hostname)
	if res == nil {
		return errors.New("empty processes metadata")
	}
	payload := map[string]interface{}{
		"resources": resources.GetPayload(hostname),
	}
	if err := s.SendJSONToV1Intake(payload); err != nil {
		return fmt.Errorf("unable to serialize processes metadata payload, %s", err)
	}
	return nil
}

func init() {
	RegisterCollector("resources", new(ResourcesCollector))
}
