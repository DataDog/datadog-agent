// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020-present Datadog, Inc.

//go:build ec2

// Package aws contains database-monitoring specific RDS discovery logic
package aws

import (
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/rds/types"
)

// Instance represents an Aurora or RDS instance
type Instance struct {
	ID           string
	ClusterID    string
	Endpoint     string
	Port         int32
	IamEnabled   bool
	Engine       string
	DbName       string
	GlobalViewDb string
	DbmEnabled   bool
}

// dbNameFromEngine returns the default database name for a given engine type
func dbNameFromEngine(engine string) (string, error) {
	switch engine {
	case mysqlEngine:
		fallthrough
	case auroraMysqlEngine:
		return "mysql", nil
	case postgresEngine:
		fallthrough
	case auroraPostgresqlEngine:
		return "postgres", nil
	default:
		return "", fmt.Errorf("unsupported engine type: %s", engine)
	}
}

func makeInstance(db types.DBInstance, config Config) (*Instance, error) {
	if db.Endpoint == nil || db.Endpoint.Address == nil {
		return nil, fmt.Errorf("DBInstance %v missing endpoint", db)
	}
	// Add to list of instances for the cluster
	instance := Instance{
		Endpoint: *db.Endpoint.Address,
	}

	if db.DBInstanceIdentifier != nil {
		instance.ID = *db.DBInstanceIdentifier
	}

	// Set the cluster ID if it is present
	if db.DBClusterIdentifier != nil {
		instance.ClusterID = *db.DBClusterIdentifier
	}

	// Set if IAM is configured for the endpoint
	if db.IAMDatabaseAuthenticationEnabled != nil {
		instance.IamEnabled = *db.IAMDatabaseAuthenticationEnabled
	}
	// Set the port, if it is known
	if db.Endpoint.Port != nil {
		instance.Port = *db.Endpoint.Port
	}
	if db.Engine != nil {
		instance.Engine = *db.Engine
	}
	if db.DBName != nil {
		instance.DbName = *db.DBName
	} else {
		if db.Engine != nil {
			defaultDBName, err := dbNameFromEngine(*db.Engine)
			if err != nil {
				return nil, fmt.Errorf("error getting default db name from engine: %v", err)
			}

			instance.DbName = defaultDBName
		} else {
			// This should never happen, as engine is a required field in the API
			// but we should handle it.
			return nil, fmt.Errorf("engine is nil for instance %v", db)
		}
	}
	for _, tag := range db.TagList {
		tagString := ""
		if tag.Key != nil {
			tagString += *tag.Key
		}
		if tag.Value != nil {
			tagString += ":" + *tag.Value
		}
		if tag.Key != nil && *tag.Key == config.GlobalViewDbTag && tag.Value != nil {
			instance.GlobalViewDb = *tag.Value
		}
		if tagString == config.DbmTag {
			instance.DbmEnabled = true
			break
		}
	}
	return &instance, nil
}
