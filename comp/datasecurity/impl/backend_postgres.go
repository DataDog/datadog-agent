// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package datasecurityimpl

type postgresBackend struct{}

func (postgresBackend) Kind() backendKind { return backendPostgres }

func (postgresBackend) IntegrationName() string { return "postgres" }

func (postgresBackend) Detect(payload rcPayload) bool {
	for _, task := range payload.Tasks {
		if task.ScanData.Postgres != nil && task.ScanData.Postgres.Query != "" {
			return true
		}
	}
	return false
}

func (postgresBackend) ConnectHost(instance map[string]any) string {
	return stringField(instance, "host")
}

func (postgresBackend) ApplyRuntimeConnection(
	instance map[string]any,
	runtimeCfg *runtimeInstanceConfig,
) error {
	cfg := extractPostgresCredentials(instance)
	runtimeCfg.Postgres = &cfg
	return nil
}

func extractPostgresCredentials(instance map[string]any) postgresConfig {
	host := stringField(instance, "host")
	if host == "" {
		host = "localhost"
	}
	return postgresConfig{
		Host:     host,
		Port:     portIntWithDefault(instance, 5432),
		Username: stringField(instance, "username", "user"),
		Password: stringField(instance, "password"),
		Dbname:   stringField(instance, "dbname", "database"),
	}
}
