// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build oracle

package oracle

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type vDatabase struct {
	Name string `db:"NAME"`
	Cdb  string `db:"CDB"`
}

type vInstance struct {
	HostName     sql.NullString `db:"HOST_NAME"`
	InstanceName string         `db:"INSTANCE_NAME"`
	VersionFull  string         `db:"VERSION_FULL"`
}

func (c *Check) init() error {
	tags := make([]string, len(c.configTags))
	copy(tags, c.configTags)

	if c.db == nil {
		return fmt.Errorf("database connection not initialized")
	}

	var d vDatabase
	err := getWrapper(c, &d, "SELECT /* DD */ lower(name) name, cdb FROM v$database")
	if err != nil {
		return fmt.Errorf("%s failed to query v$database: %w", c.logPrompt, err)
	}
	c.cdbName = d.Name
	tags = append(tags, fmt.Sprintf("cdb:%s", c.cdbName))
	isMultitenant := true
	if d.Cdb == "NO" {
		isMultitenant = false
	}
	c.multitenant = isMultitenant

	var i vInstance
	err = getWrapper(c, &i, "SELECT /* DD */ host_name, instance_name, version version_full FROM v$instance")
	if err != nil {
		return fmt.Errorf("%s failed to query v$instance: %w", c.logPrompt, err)
	}
	c.dbVersion = i.VersionFull
	if isDbVersionGreaterOrEqualThan(c, "18") {
		err = getWrapper(c, &c.dbVersion, "SELECT /* DD */ version_full FROM v$instance")
		if err != nil {
			return fmt.Errorf("%s failed to query full version: %w", c.logPrompt, err)
		}
	}

	if c.config.ReportedHostname != "" {
		c.dbHostname = c.config.ReportedHostname
	} else {
		if i.HostName.Valid {
			c.dbHostname = i.HostName.String
		} else {
			// host_name is null on Oracle Autonomous Database
			c.dbHostname = i.InstanceName
		}
	}
	if i.HostName.Valid {
		tags = append(tags, fmt.Sprintf("real_hostname:%s", i.HostName.String))
	}
	tags = append(tags, fmt.Sprintf("host:%s", c.dbHostname), fmt.Sprintf("oracle_version:%s", c.dbVersion))

	c.logPrompt = fmt.Sprintf("%s@%s> ", c.cdbName, c.dbHostname)

	// Check if PDB
	var connectionType string
	err = getWrapper(c, &connectionType, "select decode(sys_context('USERENV','CON_ID'),1,'CDB','PDB') TYPE from DUAL")
	if err != nil {
		return fmt.Errorf("failed to query connection type: %w", err)
	}
	if connectionType == "PDB" {
		c.connectedToPdb = true
	}

	// determine hosting type
	ht := selfManaged

	if isMultitenant {
		// is RDS?
		if ht == selfManaged {
			// Is RDS?
			if c.filePath == "" {
				var path string
				err = getWrapper(c, &path, "SELECT SUBSTR(name, 1, 10) path FROM v$datafile WHERE rownum = 1")
				if err != nil {
					return fmt.Errorf("failed to query v$datafile: %w", err)
				}
				if path == "/rdsdbdata" {
					ht = rds
				}
			}
		}

		// is OCI?
		if ht == selfManaged {
			var cloudRows int
			if c.connectedToPdb {
				err = getWrapper(c, &cloudRows, "select 1 from v$pdbs where cloud_identity like '%oraclecloud%' and rownum = 1")
				if err != nil {
					log.Errorf("%s failed to query v$pdbs: %s", c.logPrompt, err)
				}
				if cloudRows == 1 {
					err = getWrapper(c, &cloudRows, "select 1 from cdb_services where name like '%oraclecloud%' and rownum = 1")
					if err != nil {
						log.Errorf("failed to query cdb_services: %s", err)
					}
				}
			}
			if cloudRows == 1 {
				ht = oci
			}
		}
	}

	tags = append(tags, fmt.Sprintf("hosting_type:%s", ht))
	c.hostingType = ht
	c.tags = make([]string, len(tags))
	copy(c.tags, tags)
	c.tagsString = strings.Join(tags, ",")

	c.fqtEmitted = getFqtEmittedCache()
	c.planEmitted = getPlanEmittedCache(c)
	c.initialized = true

	return nil
}
