// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metadata

import (
	"fmt"
	"runtime"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	configUtils "github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// run the Agent checks metadata collector every 600 seconds (10 minutes). AgentChecksCollector implements the
	// CollectorWithFirstRun and will send its first payload after a minute.
	agentChecksMetadataCollectorInterval = 600
)

type collector struct {
	os          string
	interval    time.Duration
	ignoreError bool
}

var (
	// default collectors by os
	defaultCollectors = map[string]collector{
		"agent_checks": {os: "*", interval: agentChecksMetadataCollectorInterval * time.Second},
	}

	// AllDefaultCollectors the names of all the available default collectors
	AllDefaultCollectors = []string{}
)

func init() {
	for collectorName := range defaultCollectors {
		AllDefaultCollectors = append(AllDefaultCollectors, collectorName)
	}
}

// addCollector adds a collector by name to the Scheduler
func addCollector(name string, intl time.Duration, sch *Scheduler) error {
	if err := sch.AddCollector(name, intl); err != nil {
		return fmt.Errorf("Unable to add '%s' metadata provider: %v", name, err)
	}
	log.Infof("Scheduled metadata provider '%v' to run every %v", name, intl)
	return nil
}

// addDefaultCollector adds one of the default collectors to the Scheduler
func addDefaultCollector(name string, sch *Scheduler) error {
	if cInfo, ok := defaultCollectors[name]; ok {
		if cInfo.os != "*" && runtime.GOOS != cInfo.os {
			return nil
		}
		err := sch.AddCollector(name, cInfo.interval)
		if err != nil && !cInfo.ignoreError {
			log.Warnf("Could not add metadata provider for %s: %v", name, err)
			return err
		}
		log.Debugf("Scheduled default metadata provider '%v' to run every %v", name, cInfo.interval)
		return nil
	}
	return fmt.Errorf("Unknown default metadata provider '%s'", name)
}

// SetupMetadataCollection initializes the metadata scheduler and its
// collectors based on the config. This function also starts the default
// collectors listed in 'additionalCollectors' if they're not listed in the
// configuration.
func SetupMetadataCollection(sch *Scheduler, additionalCollectors []string) error {
	if !config.Datadog.GetBool("enable_metadata_collection") {
		log.Warnf("Metadata collection disabled, only do that if another agent/dogstatsd is running on this host")
		return nil
	}

	collectorAdded := map[string]interface{}{}
	C, err := configUtils.GetMetadataProviders(config.Datadog)
	if err == nil {
		log.Debugf("Adding configured providers to the metadata collector")
		for _, c := range C {
			if c.Interval == 0 {
				log.Infof("Interval of metadata provider '%v' set to 0, skipping provider", c.Name)
				continue
			}

			intl := c.Interval * time.Second
			if err := addCollector(c.Name, intl, sch); err != nil {
				log.Error(err.Error())
			} else {
				collectorAdded[c.Name] = nil
			}
		}
	} else {
		log.Errorf("Unable to parse metadata_providers config: %v", err)
	}

	// Adding default collectors if they were not listed in the configuration
	for _, name := range additionalCollectors {
		if _, ok := collectorAdded[name]; ok {
			continue
		}

		if err := addDefaultCollector(name, sch); err != nil {
			return err
		}
	}
	return nil
}
