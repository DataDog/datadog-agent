// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package queryactionsimpl

import "reflect"

// DODatabaseConfig represents a single database credential entry configured under
// data_observability.query_actions.databases in datadog.yaml.
type DODatabaseConfig struct {
	Host                  string `mapstructure:"host"`
	Port                  int    `mapstructure:"port"`
	DBName                string `mapstructure:"dbname"`
	Username              string `mapstructure:"username"`
	Password              string `mapstructure:"password"`
	SSL                   any    `mapstructure:"ssl"`
	SSLMode               string `mapstructure:"ssl_mode"`
	SSLCert               string `mapstructure:"ssl_cert"`
	SSLKey                string `mapstructure:"ssl_key"`
	SSLRootCert           string `mapstructure:"ssl_root_cert"`
	TLS                   any    `mapstructure:"tls"`
	TLSVerify             any    `mapstructure:"tls_verify"`
	TLSCert               string `mapstructure:"tls_cert"`
	TLSKey                string `mapstructure:"tls_key"`
	TLSCACert             string `mapstructure:"tls_ca_cert"`
	AWS                   any    `mapstructure:"aws"`
	ManagedAuthentication any    `mapstructure:"managed_authentication"`
}

// fieldByMapstructureTag maps mapstructure tag names to struct field indices for DODatabaseConfig.
var fieldByMapstructureTag map[string]int

func init() {
	fieldByMapstructureTag = make(map[string]int)
	t := reflect.TypeOf(DODatabaseConfig{})
	for i := 0; i < t.NumField(); i++ {
		tag := t.Field(i).Tag.Get("mapstructure")
		if tag != "" {
			fieldByMapstructureTag[tag] = i
		}
	}
}

// toInstanceMap converts a DODatabaseConfig to a map[string]any using the same keys
// as dbCredentialAllowList. Fields typed as `any` (interface) are included whenever
// non-nil, even if the underlying value is a zero like false or 0. Concrete-typed
// fields (string, int) are included only when non-zero.
func (d *DODatabaseConfig) toInstanceMap() map[string]any {
	out := make(map[string]any)
	v := reflect.ValueOf(d).Elem()
	for _, key := range dbCredentialAllowList {
		idx, ok := fieldByMapstructureTag[key]
		if !ok {
			continue
		}
		field := v.Field(idx)
		// For interface (any) fields, check IsNil rather than IsZero so that
		// explicit false / 0 values (e.g. tls_verify: false) are preserved.
		if field.Kind() == reflect.Interface {
			if field.IsNil() {
				continue
			}
		} else if field.IsZero() {
			continue
		}
		out[key] = field.Interface()
	}
	return out
}

// matchesIdentifier checks if this database config matches the given DB identifier
// by host and dbname, same logic as the existing matchesIdentifier for postgres instances.
// Port is intentionally not checked — the RC payload's DBIdentifier does not carry a port,
// and the existing postgres-check matching also ignores port.
func (d *DODatabaseConfig) matchesIdentifier(dbID *DBIdentifier) bool {
	return d.Host == dbID.Host && d.DBName == dbID.DBName
}
