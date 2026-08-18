// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package collectors

import (
	"context"
	"fmt"

	configfilesdiscoveryimpl "github.com/DataDog/datadog-agent/comp/core/configfilesdiscovery/impl"
)

// PostgresIntegrationName is the Autodiscovery check name for PostgreSQL.
const PostgresIntegrationName = "postgres"

type postgresConfigCollector struct{}

// postgresEnvAllow lists the only environment variables this collector ever
// forwards. PGDATA and other PG*/POSTGRES* variables otherwise carry
// credentials, connection strings, or arbitrary argument bags.
var postgresEnvAllow = map[string]struct{}{
	"PGDATA":                    {},
	"POSTGRES_DB":               {},
	"POSTGRES_USER":             {},
	"POSTGRES_INITDB_WALDIR":    {},
}

// postgresEnvDeny is defense in depth and documentation: the allow-list above
// already excludes every name here. Keeping the exclusion explicit protects
// against the allow-list being loosened later without revisiting these.
var postgresEnvDeny = map[string]struct{}{
	"POSTGRES_PASSWORD":      {},
	"POSTGRES_PASSWORD_FILE": {},
	"POSTGRES_INITDB_ARGS":   {},
	"PGPASSWORD":             {},
	"PGPASSFILE":             {},
	"PGSERVICE":              {},
	"PGSERVICEFILE":          {},
	"PGOPTIONS":              {},
	"PGSSLKEY":               {},
	"PGSSLCERT":              {},
	"PGSSLROOTCERT":          {},
	"PGREQUIRESSL":           {},
	"DATABASE_URL":           {},
	"POSTGRES_URL":           {},
}

// NewPostgres returns a collector that reads selected, non-secret PostgreSQL
// environment variables. It does not read postgresql.conf; config-file
// collection is intentionally out of scope.
func NewPostgres() configfilesdiscoveryimpl.ConfigCollector {
	return postgresConfigCollector{}
}

// CanCollectFromProcess always returns false: this collector has no
// process-command-line-based collection to retry.
func (postgresConfigCollector) CanCollectFromProcess(configfilesdiscoveryimpl.TargetCommandline) bool {
	return false
}

func (postgresConfigCollector) Collect(ctx context.Context, reader configfilesdiscoveryimpl.ConfigReader) (configfilesdiscoveryimpl.CollectedConfig, error) {
	envVars, err := readEnvVars(ctx, reader, includePostgresEnvVar)
	if err != nil {
		return configfilesdiscoveryimpl.CollectedConfig{}, fmt.Errorf("read postgres env vars: %w", err)
	}
	if len(envVars) == 0 {
		return configfilesdiscoveryimpl.CollectedConfig{}, nil
	}

	return configfilesdiscoveryimpl.CollectedConfig{
		EnvVars: envVars,
	}, nil
}

func includePostgresEnvVar(name string) bool {
	if configfilesdiscoveryimpl.IsSecretEnvVarName(name) {
		return false
	}
	if _, denied := postgresEnvDeny[name]; denied {
		return false
	}
	_, allowed := postgresEnvAllow[name]
	return allowed
}
