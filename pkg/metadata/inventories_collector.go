// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package metadata

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery"
	checkCollector "github.com/DataDog/datadog-agent/pkg/collector"
	"github.com/DataDog/datadog-agent/pkg/metadata/inventories"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// InventoriesCollector fills and sends the inventory metadata payload
type InventoriesCollector struct{}

// Send collects the data needed and submits the payload
func (hp *InventoriesCollector) Send(s *serializer.Serializer) error {
	log.Errorf("Collecting inventories !")
	ac := autodiscovery.GetCurrentAutoConfig()
	coll := checkCollector.GetCurrentCollector()

	hostname, err := util.GetHostname()
	if err != nil {
		return fmt.Errorf("unable to submit inventories metadata payload, no hostname: %s", err)
	}

	payload := inventories.GetPayload(hostname, ac, coll)
	if err := s.SendMetadata(payload); err != nil {
		return fmt.Errorf("unable to submit inventories metadata payload, %s", err)
	}
	return nil
}

func init() {
	RegisterCollector("inventories", new(InventoriesCollector))
}
