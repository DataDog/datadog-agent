// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020-present Datadog, Inc.

//go:build ec2

// Package aws contains database-monitoring specific RDS discovery logic
package aws

import "fmt"

// Instance represents an Aurora or RDS instance
type Instance struct {
	Id         string
	Endpoint   string
	Port       int32
	IamEnabled bool
	Engine     string
	DbName     string
	DbmEnabled bool
}

// dbNameFromEngine returns the default database name for a given engine type
func dbNameFromEngine(engine string) (string, error) {
	switch engine {
	case mysqlEngine:
		fallthrough
	case auroraMysqlEngine:
		return "mysql", nil
	case postgresqlEngine:
		fallthrough
	case auroraPostgresqlEngine:
		return "postgres", nil
	default:
		return "", fmt.Errorf("unsupported engine type: %s", engine)
	}
}
