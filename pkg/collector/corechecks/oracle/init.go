// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build oracle

package oracle

import (
	"database/sql"
	"fmt"
	"regexp"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/benbjohnson/clock"
)

type vInstance struct {
	HostName     sql.NullString `db:"HOST_NAME"`
	InstanceName string         `db:"INSTANCE_NAME"`
	VersionFull  string         `db:"VERSION_FULL"`
}

const minMultitenantVersion = "12"

func (c *Check) init() error {
	tags := make([]string, len(c.configTags))
	copy(tags, c.configTags)

	if c.db == nil {
		return fmt.Errorf("database connection not initialized")
	}

	var i vInstance
	err := getWrapper(c, &i, "SELECT /* DD */ host_name, instance_name, version version_full FROM v$instance")
	if err != nil {
		isPrivilegeError, err2 := handlePrivilegeError(c, err)
		if !c.dbmEnabled && isPrivilegeError {
			c.legacyIntegrationCompatibilityMode = true
			log.Warnf("%s missing privileges detected, falling back to deprecated Oracle integration %s", c.logPrompt, err2.Error())
			c.initialized = true
			if strings.HasPrefix(strings.ToUpper(c.config.Username), "C##") {
				c.multitenant = true
			} else {
				c.connectedToPdb = true
			}
			c.tagsWithoutDbRole = make([]string, len(tags))
			copy(c.tagsWithoutDbRole, tags)
			return nil
		}
		return fmt.Errorf("%s failed to query v$instance: %w", c.logPrompt, err)
	}
	c.dbVersion = i.VersionFull
	if isDbVersionGreaterOrEqualThan(c, "18") {
		err = getWrapper(c, &c.dbVersion, "SELECT /* DD */ version_full FROM v$instance")
		if err != nil {
			return fmt.Errorf("%s failed to query full version: %w", c.logPrompt, err)
		}
	}

	if i.HostName.Valid {
		tags = append(tags, fmt.Sprintf("real_hostname:%s", i.HostName.String))
	}
	tags = append(tags, fmt.Sprintf("oracle_version:%s", c.dbVersion))

	var d vDatabase
	if isDbVersionGreaterOrEqualThan(c, minMultitenantVersion) {
		err = getWrapper(c, &d, "SELECT /* DD */ lower(name) name, cdb FROM v$database")
	} else {
		err = getWrapper(c, &d, "SELECT /* DD */ lower(name) name FROM v$database")
		d.Cdb = "NO"
	}
	if err != nil {
		return fmt.Errorf("%s failed to query v$database: %w", c.logPrompt, err)
	}
	c.cdbName = d.Name
	tags = append(tags, fmt.Sprintf("cdb:%s", c.cdbName))

	if c.config.ReportedHostname != "" {
		c.dbResolvedHostname = c.config.ReportedHostname
	} else if i.HostName.Valid {
		c.dbResolvedHostname = i.HostName.String
	}
	if !c.config.ExcludeHostname {
		c.dbHostname = c.dbResolvedHostname
	}
	if c.dbHostname == "" {
		log.Errorf("%s failed to determine hostname, consider setting reported_hostname", c.logPrompt)
	}
	c.dbInstanceIdentifier = c.createDatabaseIdentifier()
	tags = append(tags, "database_instance:"+c.dbInstanceIdentifier)

	tags = append(tags, fmt.Sprintf("dd.internal.resource:database_instance:%s", c.dbHostname))
	isMultitenant := true
	if d.Cdb == "NO" {
		isMultitenant = false
	}
	c.multitenant = isMultitenant

	c.logPrompt = fmt.Sprintf("%s@%s> ", c.cdbName, c.dbHostname)

	// Check if PDB
	if isDbVersionGreaterOrEqualThan(c, minMultitenantVersion) {
		var connectionType string
		err = getWrapper(c, &connectionType, "select decode(sys_context('USERENV','CON_ID'),1,'CDB','PDB') TYPE from DUAL")
		if err != nil {
			return fmt.Errorf("failed to query connection type: %w", err)
		}
		if connectionType == "PDB" {
			c.connectedToPdb = true
		}
	} else {
		c.connectedToPdb = true
	}

	// determine hosting type
	ht := selfManaged

	if isDbVersionGreaterOrEqualThan(c, "19") {
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
		if ht == selfManaged && isMultitenant {
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
	c.tagsWithoutDbRole = make([]string, len(tags))
	copy(c.tagsWithoutDbRole, tags)
	copy(c.tags, tags)

	c.fqtEmitted = getFqtEmittedCache()
	c.planEmitted = getPlanEmittedCache(c)
	c.clock = clock.New()
	c.initialized = true

	return nil
}

func (c *Check) createDatabaseIdentifier() string {
	tags := make(map[string]string, len(c.tags))
	for _, tag := range c.tags {
		if strings.Contains(tag, ":") {
			parts := strings.SplitN(tag, ":", 2)
			if len(parts) == 2 {
				if tags[parts[0]] != "" {
					tags[parts[0]] = fmt.Sprintf("%s,%s", tags[parts[0]], parts[1])
				} else {
					tags[parts[0]] = parts[1]
				}
			}
		}
	}
	tags["resolved_hostname"] = c.dbResolvedHostname
	tags["server"] = c.config.Server
	tags["port"] = fmt.Sprintf("%d", c.config.Port)
	tags["cdb_name"] = c.cdbName
	tags["service_name"] = c.config.ServiceName

	identifier := c.config.DatabaseIdentifier.Template

	re := regexp.MustCompile(`\$([a-z_]+)`)
	matches := re.FindAllString(identifier, -1)
	for _, match := range matches {
		key := strings.TrimPrefix(match, "$")
		if value, ok := tags[key]; ok && value != "" {
			identifier = strings.ReplaceAll(identifier, match, value)
		} else {
			log.Warnf("%s failed to replace %s in database identifier template", c.logPrompt, match)
		}
	}

	return identifier
}
