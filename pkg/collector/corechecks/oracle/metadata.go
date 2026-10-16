// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build oracle

package oracle

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/oracle/common"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/oracle/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type dbInstanceMetadata struct {
	Dbm            bool   `json:"dbm"`
	ConnectionHost string `json:"connection_host"`
}

type dbInstanceEvent struct {
	Host               string             `json:"host,omitempty"`
	DatabaseInstance   string             `json:"database_instance"`
	AgentVersion       string             `json:"agent_version"`
	Dbms               string             `json:"dbms"`
	Kind               string             `json:"kind"`
	CollectionInterval int64              `json:"collection_interval"`
	DbmsVersion        string             `json:"dbms_version"`
	Tags               []string           `json:"tags"`
	Timestamp          float64            `json:"timestamp"`
	Metadata           dbInstanceMetadata `json:"metadata"`
}

func sendDbInstanceMetadata(c *Check) error {
	configTags := make([]string, 0, len(c.config.Tags)+1)
	for _, tag := range c.config.Tags {
		// The discovered CDB scopes raw activity PDB names to query metric PDBs.
		// A conflicting user tag must not make that mapping ambiguous.
		if c.cdbName != "" && strings.HasPrefix(strings.ToLower(tag), "cdb:") {
			continue
		}
		configTags = append(configTags, tag)
	}
	if c.cdbName != "" {
		configTags = append(configTags, "cdb:"+c.cdbName)
	}
	m := dbInstanceMetadata{
		Dbm:            c.dbmEnabled,
		ConnectionHost: config.GetConnectData(c.config.InstanceConfig),
	}
	e := dbInstanceEvent{
		Host:               c.dbHostname,
		DatabaseInstance:   c.dbInstanceIdentifier,
		AgentVersion:       c.agentVersion,
		Dbms:               common.IntegrationName,
		Kind:               "database_instance",
		CollectionInterval: c.config.InstanceConfig.DatabaseInstanceCollectionInterval,
		DbmsVersion:        c.dbVersion,
		Tags:               configTags,
		Timestamp:          float64(time.Now().UnixMilli()),
		Metadata:           m,
	}

	payloadBytes, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata payload: %w", err)
	}

	sender, err := c.GetSender()
	if err != nil {
		return fmt.Errorf("failed to get sender for metadata payload: %w", err)
	}
	sender.EventPlatformEvent(payloadBytes, "dbm-metadata")
	log.Debugf("%s dbm-metadata payload %s", c.logPrompt, strings.ReplaceAll(string(payloadBytes), "@", "XX"))
	sender.Commit()

	return nil
}
