// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package collectors

import (
	"context"
	"fmt"
	"regexp"

	configfilesdiscoveryimpl "github.com/DataDog/datadog-agent/comp/core/configfilesdiscovery/impl"
)

// PostgresIntegrationName is the Autodiscovery check name for PostgreSQL.
const PostgresIntegrationName = "postgres"

type postgresConfigCollector struct{}

// postgresEnvAllow lists the documented PostgreSQL server and official-image
// settings whose values are configuration metadata. Bitnami's POSTGRESQL_*
// settings are handled by postgresBitnamiEnvAllow below.
//
// Keep POSTGRES_* explicit: that namespace contains passwords, credentials,
// and free-form argument values.
var postgresEnvAllow = map[string]struct{}{
	// PostgreSQL server environment variables.
	"PGCLIENTENCODING":    {},
	"PGDATA":              {},
	"PGDATESTYLE":         {},
	"PGPORT":              {},
	"PG_OOM_ADJUST_FILE":  {},
	"PG_OOM_ADJUST_VALUE": {},

	// Official PostgreSQL image variables.
	"POSTGRES_DB":            {},
	"POSTGRES_INITDB_WALDIR": {},
	"POSTGRES_USER":          {},

	// Bitnami POSTGRES_* aliases for the settings above.
	"POSTGRES_CLUSTER_APP_NAME":         {},
	"POSTGRES_INITDB_WAL_DIR":           {},
	"POSTGRES_MASTER_HOST":              {},
	"POSTGRES_MASTER_PORT_NUMBER":       {},
	"POSTGRES_NUM_SYNCHRONOUS_REPLICAS": {},
	"POSTGRES_PORT_NUMBER":              {},
	"POSTGRES_REPLICATION_MODE":         {},
	"POSTGRES_REPLICATION_USER":         {},
	"POSTGRES_SHUTDOWN_MODE":            {},
	"POSTGRES_SYNCHRONOUS_COMMIT_MODE":  {},
}

// postgresBitnamiEnvAllow accepts Bitnami's configuration namespace. The
// shared secret-name policy and postgresEnvDeny reject unsafe settings first.
var postgresBitnamiEnvAllow = regexp.MustCompile(`^POSTGRESQL_[A-Z0-9_]+$`)

// postgresEnvDeny covers documented settings whose values can contain a secret
// or an unbounded argument string. The common secret-name filter rejects the
// password, key and certificate variants before this predicate is reached.
var postgresEnvDeny = map[string]struct{}{
	"PGOPTIONS":                            {},
	"PGPASSFILE":                           {},
	"PGSERVICE":                            {},
	"PGSERVICEFILE":                        {},
	"POSTGRES_HOST_AUTH_METHOD":            {},
	"POSTGRES_INITDB_ARGS":                 {},
	"POSTGRESQL_EXTRA_FLAGS":               {},
	"POSTGRESQL_INITDB_ARGS":               {},
	"POSTGRESQL_LDAP_BIND_PASSFILE_PATH":   {},
	"POSTGRESQL_LDAP_URL":                  {},
	"POSTGRESQL_PGHBA_AUTH_METHOD":         {},
	"POSTGRESQL_REPLICATION_PASSFILE_PATH": {},
	"POSTGRESQL_TLS_CA_FILE":               {},
	"POSTGRESQL_TLS_CRL_FILE":              {},
}

var postgresBitnamiEnvDeny = regexp.MustCompile(
	`^POSTGRESQL_(?:.*_)?(?:ARGS|AUTH_METHOD|CONNECTION_STRING|DSN|FLAGS|OPTS|PASSFILE(?:_PATH)?|URI|URL)$`,
)

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
	if postgresBitnamiEnvDeny.MatchString(name) {
		return false
	}
	_, allowed := postgresEnvAllow[name]
	return allowed || postgresBitnamiEnvAllow.MatchString(name)
}
