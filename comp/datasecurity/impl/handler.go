// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package datasecurityimpl

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"google.golang.org/protobuf/encoding/protojson"

	eventplatform "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/def"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	sdspb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/sds"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/pkg/util/sds"
	"github.com/jackc/pgx/v5/pgxpool"
	yaml "gopkg.in/yaml.v3"
)

const (
	// postgresIntegrationName is the integration whose instance we look up to
	// run the scan query. Only postgres is supported for now.
	postgresIntegrationName = "postgres"

	// dataSecuritySection is the key of the postgres instance section that
	// carries the per-instance opt-in (enabled: true).
	dataSecuritySection = "data_security"

	// queryTimeout bounds both the connection and the query execution.
	queryTimeout = 30 * time.Second

	// resourceTypeRDSInstance is the SdsResultPayload resource type for an AWS
	// RDS instance. The sds-intake routes results with this type to the backend
	// RdsExtractor, which reads the scan location from the rdsTable oneof.
	resourceTypeRDSInstance = "aws_rds_instance"
)

// rcPayload is the DEBUG RC payload this component understands:
//
//	{
//	  "tasks": [
//	    {
//	      "scanning_rules": [ { "id": "...", "name": "...", "regex": "..." }, ... ],
//	      "scan_data": { "postgres": { "query": "SELECT ..." } }
//	    }
//	  ]
//	}
type rcPayload struct {
	Tasks []rcTask `json:"tasks"`
}

type rcTask struct {
	ScanningRules []rcScanningRule `json:"scanning_rules"`
	ScanData      rcScanData       `json:"scan_data"`
}

type rcScanningRule struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Regex string `json:"regex"`
}

type rcScanData struct {
	Postgres *rcPostgresScanData `json:"postgres"`
}

type rcPostgresScanData struct {
	Query string `json:"query"`
	Table string `json:"table"`
}

// queryResult holds a column-oriented query result and the metadata needed to
// build the SDS result payload. Columns maps a column name to the list of
// values returned for that column across all rows, e.g.
// {"id": [1, 2, 3], "email": ["alice@example.com", ...]}.
type queryResult struct {
	taskID    string
	dbHost    string
	dbName    string
	tableName string
	columns   map[string][]interface{}
	rowCount  int
	durationS float64
}

// onUpdate is invoked by the RC client with the full set of active configs for
// the DEBUG product.
func (c *component) onUpdate(updates map[string]state.RawConfig, applyStatus func(string, state.ApplyStatus)) {
	for path, rawConfig := range updates {
		var payload rcPayload
		if err := json.Unmarshal(rawConfig.Config, &payload); err != nil {
			c.log.Warnf("datasecurity: failed to unmarshal DEBUG config %s: %v", path, err)
			applyStatus(path, state.ApplyStatus{State: state.ApplyStateError, Error: err.Error()})
			continue
		}

		if err := c.applyConfig(path, payload); err != nil {
			c.log.Warnf("datasecurity: cannot apply DEBUG config from %s: %v", path, err)
			applyStatus(path, state.ApplyStatus{State: state.ApplyStateError, Error: err.Error()})
			continue
		}

		applyStatus(path, state.ApplyStatus{State: state.ApplyStateAcknowledged})
	}
}

// applyConfig initializes an SDS scanner from the rules carried by the payload,
// runs the postgres scan query, scans the result and builds the SDS result
// protobuf payload, forwards it to the sds-result intake, then destroys the
// scanner. The scanner lives only for the duration of the RC handling.
func (c *component) applyConfig(path string, payload rcPayload) error {
	var rules []sds.RuleDefinition
	var query, table string
	for _, t := range payload.Tasks {
		for _, r := range t.ScanningRules {
			rules = append(rules, sds.RuleDefinition{
				ID:    r.ID,
				Name:  r.Name,
				Regex: r.Regex,
			})
		}
		if query == "" && t.ScanData.Postgres != nil {
			query = t.ScanData.Postgres.Query
			table = t.ScanData.Postgres.Table
		}
	}

	if len(rules) == 0 {
		return errors.New("no scanning rules in payload")
	}

	// Initialize a scanner from the rules and destroy it once we are done
	// handling this config.
	scanner, err := sds.NewScanner(rules)
	if err != nil {
		return fmt.Errorf("creating scanner: %w", err)
	}
	defer func() {
		if err := scanner.Close(); err != nil {
			c.log.Warnf("datasecurity: failed to close scanner for %s: %v", path, err)
		}
	}()
	c.log.Infof("datasecurity: created scanner with %d rule(s) for %s", len(rules), path)

	if query == "" {
		return nil
	}

	instance, ok := c.findPostgresInstance()
	if !ok {
		return errors.New("no postgres instance with data_security.enabled: true found")
	}

	host := stringField(instance, "host")
	if host == "" {
		host = "localhost"
	}
	portStr := portString(instance)
	dbname := stringField(instance, "dbname", "database")
	user := stringField(instance, "username", "user")
	password := stringField(instance, "password")
	c.log.Infof("datasecurity: postgres data_security instance host=%s port=%s dbname=%s user=%q password=%q", host, portStr, dbname, user, password)

	columns, rowCount, durationS, err := runPostgresQuery(host, portStr, dbname, user, password, query)
	if err != nil {
		return fmt.Errorf("running query: %w", err)
	}

	if columnsJSON, err := json.Marshal(columns); err == nil {
		c.log.Infof("datasecurity: query returned %d row(s): %s", rowCount, string(columnsJSON))
	}

	result := queryResult{
		taskID:    path,
		dbHost:    host,
		dbName:    dbname,
		tableName: table,
		columns:   columns,
		rowCount:  rowCount,
		durationS: durationS,
	}

	out, err := c.buildSDSResultPayload(scanner, result)
	if err != nil {
		return err
	}

	return c.sendSDSResult(result.taskID, result.rowCount, out)
}

// buildSDSResultPayload scans the column-oriented result with the scanner and
// returns the protobuf-marshaled SdsResultPayload ready to forward to the SDS
// (Data Security) intake.
func (c *component) buildSDSResultPayload(scanner sds.Scanner, result queryResult) ([]byte, error) {
	tableMatches, err := scanColumns(scanner, result.columns)
	if err != nil {
		return nil, fmt.Errorf("scanning query result for task %q: %w", result.taskID, err)
	}

	// The instance endpoint (host) identifies the RDS instance; fall back to the
	// database name when it is not set.
	resourceName := result.dbHost
	if resourceName == "" {
		resourceName = result.dbName
	}

	payload := &sdspb.SdsResultPayload{
		// AGENTLESS is the only source the sds-intake accepts today, so
		// Agent-emitted results impersonate the agentless scanner.
		ScanSource: sdspb.SdsResultPayload_AGENTLESS,
		ScanningSource: &sdspb.ScanningSource{
			Source: &sdspb.ScanningSource_Agentless_{
				Agentless: &sdspb.ScanningSource_Agentless{
					Version: "1.0.0",
					Region:  "us-east-1",
				},
			},
		},
		Timestamp: time.Now().UnixMilli(),
		// Type "aws_rds_instance" routes the result to the backend RdsExtractor,
		// which reads the location from the rdsTable oneof below.
		Resource: &sdspb.SdsResultPayload_Resource{
			Type: resourceTypeRDSInstance,
			Name: resourceName,
		},
		ScanResults: []*sdspb.SdsResultPayload_ScanResult{
			{
				Duration:     int64(result.durationS * 1000),
				TableMatches: tableMatches,
				Location: &sdspb.SdsResultPayload_ScanLocation{
					// Deprecated flat field, still populated for backward
					// compatibility with the intake.
					ScanLocationType: &sdspb.SdsResultPayload_ScanLocation_Database{
						Database: result.dbName,
					},
					ScanLocation: &sdspb.SdsResultPayload_ScanLocation_RdsTable{
						RdsTable: &sdspb.SdsResultPayload_RdsTable{
							InstanceArn:  result.dbHost,
							DatabaseName: result.dbName,
							TableName:    result.tableName,
						},
					},
				},
			},
		},
	}

	out, err := payload.MarshalVT()
	if err != nil {
		return nil, fmt.Errorf("marshaling sds result for task %q: %w", result.taskID, err)
	}

	// Log the JSON-rendered payload (proto field names) so it is visible what is
	// actually sent on the wire; the protobuf bytes themselves are not readable.
	if js, jsErr := (protojson.MarshalOptions{UseProtoNames: true}).Marshal(payload); jsErr == nil {
		c.log.Infof("datasecurity: built %d-byte sds-result payload for task %q:\n%s", len(out), result.taskID, string(js))
	}

	return out, nil
}

// sendSDSResult forwards the already-built protobuf payload to the lean
// sds-result intake.
func (c *component) sendSDSResult(taskID string, rowCount int, payload []byte) error {
	forwarder, ok := c.epforwarder.Get()
	if !ok {
		return errors.New("event platform forwarder not initialized")
	}
	m := message.NewMessage(payload, nil, "", time.Now().UnixNano())
	if err := forwarder.SendEventPlatformEventBlocking(m, eventplatform.EventTypeSDSResult); err != nil {
		return fmt.Errorf("sending sds result event: %w", err)
	}
	c.log.Infof("datasecurity: sent sds-result for task %q (%d row(s))", taskID, rowCount)
	return nil
}

// scanColumns scans the whole column-oriented result set with a single ScanMap
// call and returns, for each (column, rule) pair that fired, one TableMatch
// counting how many rows (values) in that column matched the rule. The scanner
// reports each hit with a jsonpath such as "email[2]"; the leading path segment
// is the column name. Results are sorted by column then rule so the payload is
// deterministic regardless of map iteration order.
func scanColumns(scanner sds.Scanner, columns map[string][]interface{}) ([]*sdspb.SdsResultPayload_TableMatch, error) {
	// The scanner only matches string values for now, so keep only the string
	// values of each column and drop everything else.
	event := make(map[string]interface{}, len(columns))
	for name, values := range columns {
		strValues := make([]interface{}, 0, len(values))
		for _, v := range values {
			if s, ok := v.(string); ok {
				strValues = append(strValues, s)
			}
		}
		event[name] = strValues
	}

	hits, err := scanner.ScanMap(event)
	if err != nil {
		return nil, err
	}

	type matchKey struct {
		column string
		ruleID string
	}
	// rows[key] is the set of value paths that matched, so a value matched more
	// than once by the same rule still counts as a single row.
	rows := make(map[matchKey]map[string]struct{})
	for _, h := range hits {
		key := matchKey{column: columnNameFromPath(h.Path), ruleID: h.RuleID}
		if rows[key] == nil {
			rows[key] = make(map[string]struct{})
		}
		rows[key][h.Path] = struct{}{}
	}

	keys := make([]matchKey, 0, len(rows))
	for key := range rows {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].column != keys[j].column {
			return keys[i].column < keys[j].column
		}
		return keys[i].ruleID < keys[j].ruleID
	})

	tableMatches := make([]*sdspb.SdsResultPayload_TableMatch, 0, len(keys))
	for _, key := range keys {
		tableMatches = append(tableMatches, &sdspb.SdsResultPayload_TableMatch{
			RuleId:           key.ruleID,
			ColumnName:       key.column,
			CountMatchedRows: int64(len(rows[key])),
		})
	}
	return tableMatches, nil
}

// columnNameFromPath returns the leading segment of an SDS match jsonpath, i.e.
// the column name. For "email[2]" or "email.sub" it returns "email"; for a
// top-level scalar path "email" it returns the path unchanged.
func columnNameFromPath(path string) string {
	if i := strings.IndexAny(path, "[."); i >= 0 {
		return path[:i]
	}
	return path
}

// findPostgresInstance returns the first postgres instance known to
// autodiscovery that has opted into data security via data_security.enabled: true.
func (c *component) findPostgresInstance() (map[string]any, bool) {
	for _, cfg := range c.ac.GetUnresolvedConfigs() {
		if cfg.Name != postgresIntegrationName {
			continue
		}
		for _, instanceData := range cfg.Instances {
			var instance map[string]any
			if err := yaml.Unmarshal(instanceData, &instance); err != nil {
				continue
			}
			if instanceDataSecurityEnabled(instance) {
				return instance, true
			}
		}
	}
	return nil, false
}

// runPostgresQuery connects to postgres, runs the query and returns the result
// as column-oriented data (a column name -> list of row values map), along with
// the row count and the query duration in seconds.
func runPostgresQuery(host, port, dbname, user, password, query string) (map[string][]interface{}, int, float64, error) {
	dsn := fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable", user, password, host, port, dbname)

	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("connecting to postgres: %w", err)
	}
	defer pool.Close()

	start := time.Now()
	rows, err := pool.Query(ctx, query)
	if err != nil {
		return nil, 0, 0, err
	}
	defer rows.Close()

	fields := rows.FieldDescriptions()
	names := make([]string, len(fields))
	columns := make(map[string][]interface{}, len(fields))
	for i, f := range fields {
		names[i] = string(f.Name)
		columns[names[i]] = nil
	}

	rowCount := 0
	for rows.Next() {
		values, err := rows.Values()
		if err != nil {
			return nil, 0, 0, err
		}
		for i, name := range names {
			if i < len(values) {
				columns[name] = append(columns[name], values[i])
			}
		}
		rowCount++
	}
	if err := rows.Err(); err != nil {
		return nil, 0, 0, err
	}

	return columns, rowCount, time.Since(start).Seconds(), nil
}

// instanceDataSecurityEnabled reports whether a parsed instance has opted into
// data security via data_security.enabled: true.
func instanceDataSecurityEnabled(instance map[string]any) bool {
	ds, ok := instance[dataSecuritySection].(map[string]any)
	if !ok {
		return false
	}
	enabled, _ := ds["enabled"].(bool)
	return enabled
}

// stringField returns the first key present in the instance whose value is a
// string, or "" if none is found.
func stringField(instance map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := instance[k].(string); ok {
			return v
		}
	}
	return ""
}

// portString returns the postgres port as a string for the DSN, defaulting to
// 5432 when absent or unparseable.
func portString(instance map[string]any) string {
	switch v := instance["port"].(type) {
	case int:
		return strconv.Itoa(v)
	case int64:
		return strconv.FormatInt(v, 10)
	case float64:
		return strconv.Itoa(int(v))
	case string:
		if _, err := strconv.Atoi(v); err == nil {
			return v
		}
	}
	return "5432"
}
