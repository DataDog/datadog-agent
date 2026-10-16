// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build oracle

package oracle

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"
	"unicode/utf8"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/oracle/common"
)

// V$SQL (not V$SQLSTATS) retains per-child cursor origin and lifetime metadata.
// The row limit bounds client work; the deadline also bounds the Oracle scan.
const procedureCursorsQuery = `SELECT /* DD_PROCEDURES */ * FROM (
SELECT s.con_id, NVL(c.name, '') pdb_name, s.sql_id, s.child_number,
 RAWTOHEX(s.child_address) child_address, s.loads, s.invalidations,
 s.parsing_schema_name, s.program_id, s.program_line# program_line,
 s.command_type, DBMS_LOB.SUBSTR(s.sql_fulltext, 1000, 1) sql_text,
 DBMS_LOB.GETLENGTH(s.sql_fulltext) sql_text_length,
 s.executions, s.elapsed_time, s.cpu_time, s.buffer_gets, s.disk_reads
FROM v$sql s LEFT JOIN v$containers c ON s.con_id=c.con_id
WHERE s.last_active_time >= SYSDATE - :lookback/86400
 AND (%s)
ORDER BY s.elapsed_time DESC, s.con_id, s.sql_id, s.child_number
) WHERE ROWNUM <= :row_limit`

type procedureIdentity struct {
	Name      string `json:"procedure_name"`
	Signature string `json:"procedure_signature"`
	PDBName   string `json:"pdb_name"`
}

type observedProcedure struct {
	identity procedureIdentity
	seen     time.Time
}

type procedureCounters struct {
	Executions  float64 `db:"EXECUTIONS"`
	ElapsedTime float64 `db:"ELAPSED_TIME"`
	CPUTime     float64 `db:"CPU_TIME"`
	BufferGets  float64 `db:"BUFFER_GETS"`
	DiskReads   float64 `db:"DISK_READS"`
}

type procedureRow struct {
	procedureIdentity
	MetricsSource string   `json:"metrics_source"`
	Observed      int      `json:"observed"`
	Executions    *float64 `json:"executions,omitempty"`
	ElapsedTime   *float64 `json:"elapsed_time,omitempty"`
	CPUTime       *float64 `json:"cpu_time,omitempty"`
	BufferGets    *float64 `json:"buffer_gets,omitempty"`
	DiskReads     *float64 `json:"disk_reads,omitempty"`
}

type procedureQueryAssociation struct {
	procedureIdentity
	QuerySignature string `json:"query_signature"`
	ProgramLine    int64  `json:"program_line,omitempty"`
	Source         string `json:"association_source"`
}

type procedureMetricsPayload struct {
	Host                  string                      `json:"host"`
	DatabaseInstance      string                      `json:"database_instance"`
	Timestamp             float64                     `json:"timestamp"`
	MinCollectionInterval float64                     `json:"min_collection_interval"`
	Tags                  []string                    `json:"tags"`
	AgentVersion          string                      `json:"ddagentversion"`
	AgentHostname         string                      `json:"ddagenthostname"`
	Kind                  string                      `json:"kind"`
	OracleVersion         string                      `json:"oracle_version"`
	OracleRows            []procedureRow              `json:"oracle_rows"`
	Associations          []procedureQueryAssociation `json:"oracle_query_associations,omitempty"`
}

type procedureCursorKey struct {
	ConID        int    `db:"CON_ID"`
	SQLID        string `db:"SQL_ID"`
	ChildNumber  int    `db:"CHILD_NUMBER"`
	ChildAddress string `db:"CHILD_ADDRESS"`
}

type procedureCursor struct {
	procedureCursorKey
	procedureCounters
	PDBName       sql.NullString `db:"PDB_NAME"`
	Loads         int64          `db:"LOADS"`
	Invalidations int64          `db:"INVALIDATIONS"`
	ParsingSchema sql.NullString `db:"PARSING_SCHEMA_NAME"`
	ProgramID     int64          `db:"PROGRAM_ID"`
	ProgramLine   int64          `db:"PROGRAM_LINE"`
	CommandType   int            `db:"COMMAND_TYPE"`
	SQLText       string         `db:"SQL_TEXT"`
	SQLTextLength int            `db:"SQL_TEXT_LENGTH"`
}

type procedureObject struct {
	ObjectID int64  `db:"OBJECT_ID"`
	Owner    string `db:"OWNER"`
	Name     string `db:"OBJECT_NAME"`
	LastDDL  string `db:"LAST_DDL"`
}

func (o procedureObject) identity(pdb string) procedureIdentity {
	canonical, _ := json.Marshal([3]string{pdb, o.Owner, o.Name})
	return procedureIdentity{
		Name:      displayProcedureIdentifier(o.Owner) + "." + displayProcedureIdentifier(o.Name),
		Signature: common.GetQuerySignature(string(canonical)), PDBName: pdb,
	}
}

type procedureObjectKey struct {
	conID         int
	id            int64
	owner         string
	name          string
	parsingSchema string
}

// Resolution is cached for one collection only. Re-reading object ID and DDL
// time each collection prevents retaining a stale identity after DDL or ID reuse.
type procedureResolver struct {
	c       *Check
	ctx     context.Context
	objects map[procedureObjectKey]*procedureObject
}

func (r *procedureResolver) cursorOrigin(cursor procedureCursor) (*procedureObject, error) {
	// PROGRAM_ID can come from a different PDB, even after that PDB's cursor
	// disappears. Current all-container visibility cannot establish its origin.
	if r.c.multitenant {
		return nil, nil
	}
	return r.resolve(procedureObjectKey{conID: cursor.ConID, id: cursor.ProgramID})
}

func (r *procedureResolver) resolve(key procedureObjectKey) (*procedureObject, error) {
	if object, found := r.objects[key]; found {
		return object, nil
	}
	query := `SELECT /* DD_PROCEDURES */ object_id, owner, object_name,
 TO_CHAR(last_ddl_time,'YYYY-MM-DD HH24:MI:SS') last_ddl FROM `
	var binds []interface{}
	if r.c.multitenant && !r.c.connectedToPdb {
		query += "cdb_objects WHERE con_id=:con_id AND "
		binds = append(binds, key.conID)
	} else {
		query += "dba_objects WHERE "
	}
	if key.parsingSchema != "" {
		// PL/SQL resolves a two-part name as a local package (or public
		// synonym) before interpreting the first component as a schema.
		var shadows []int
		shadowBinds := append(append([]interface{}{}, binds...), key.owner, key.parsingSchema)
		shadowQuery := "SELECT 1 FROM "
		if r.c.multitenant && !r.c.connectedToPdb {
			shadowQuery += "cdb_objects WHERE con_id=:con_id AND "
		} else {
			shadowQuery += "dba_objects WHERE "
		}
		shadowQuery += "object_name=:prefix AND (owner=:parsing_schema OR owner='PUBLIC') AND ROWNUM<=1"
		if err := r.c.db.SelectContext(r.ctx, &shadows, shadowQuery, shadowBinds...); err != nil {
			return nil, err
		}
		if len(shadows) != 0 {
			r.objects[key] = nil
			return nil, nil
		}
	}
	query += "object_type='PROCEDURE' AND status='VALID' AND "
	if key.id != 0 {
		query += "object_id=:object_id"
		binds = append(binds, key.id)
	} else {
		// A text name cannot identify the application's edition. Exact object-ID
		// observations remain possible, but never infer counters across editions.
		query += "owner=:owner AND object_name=:object_name AND edition_name IS NULL"
		binds = append(binds, key.owner, key.name)
	}
	query += " AND ROWNUM<=2"
	var objects []procedureObject
	if err := r.c.db.SelectContext(r.ctx, &objects, query, binds...); err != nil {
		return nil, err
	}
	var object *procedureObject
	if len(objects) == 1 {
		object = &objects[0]
	}
	r.objects[key] = object
	return object, nil
}

type procedureCursorBaseline struct {
	counts        procedureCounters
	loads         int64
	invalidations int64
	objectID      int64
	lastDDL       string
	signature     string
}

func enrichProcedureActivity(resolver *procedureResolver, sample OracleActivityRowDB, row *OracleActivityRow) error {
	if sample.ConID > 0 && !sample.PdbName.Valid {
		return nil
	}
	// Subprogram 1 identifies the standalone entry, not a local nested routine.
	for _, entry := range []bool{true, false} {
		objectID, subprogramID := sample.PLSQLObjectID, sample.PLSQLSubprogramID
		if entry {
			objectID, subprogramID = sample.PLSQLEntryObjectID, sample.PLSQLEntrySubprogramID
		}
		if !objectID.Valid || objectID.Int64 <= 0 || !subprogramID.Valid || subprogramID.Int64 != 1 {
			continue
		}
		object, err := resolver.resolve(procedureObjectKey{conID: sample.ConID, id: objectID.Int64})
		if err != nil {
			return err
		}
		if object == nil {
			continue
		}
		identity := object.identity(resolver.c.getFullPDBName(sample.PdbName.String))
		resolver.c.observeActivityProcedure(identity)
		if entry {
			row.EntryProcedureName, row.EntryProcedureSignature = identity.Name, identity.Signature
		} else {
			row.CurrentProcedureName, row.CurrentProcedureSignature = identity.Name, identity.Signature
		}
	}
	return nil
}

func (c *Check) observeActivityProcedure(identity procedureIdentity) {
	if c.procedureActivityObserved == nil {
		c.procedureActivityObserved = make(map[string]observedProcedure)
	}
	now := c.clock.Now()
	for signature, observed := range c.procedureActivityObserved {
		if now.Sub(observed.seen) > time.Duration(c.procedureLookback())*time.Second {
			delete(c.procedureActivityObserved, signature)
		}
	}
	_, known := c.procedureActivityObserved[identity.Signature]
	if known || len(c.procedureActivityObserved) < c.config.StoredProcedures.DBRowsLimit {
		c.procedureActivityObserved[identity.Signature] = observedProcedure{identity: identity, seen: now}
	}
}

func (c *Check) procedureLookback() int64 {
	// Configuring a procedure interval below the scheduler interval must not
	// expire an activity observation before the next check can publish it.
	return max(c.config.StoredProcedures.CollectionInterval*2, int64(c.checkInterval*2), 60)
}

func (c *Check) observeProcedureQuery(row OracleActivityRow, pdb string) {
	if c.procedureActivityAssociations == nil {
		c.procedureActivityAssociations = make(map[procedureQueryAssociation]time.Time)
	}
	now := c.clock.Now()
	for association, seen := range c.procedureActivityAssociations {
		if now.Sub(seen) > time.Duration(c.procedureLookback())*time.Second {
			delete(c.procedureActivityAssociations, association)
		}
	}
	for _, association := range []procedureQueryAssociation{
		{procedureIdentity: procedureIdentity{Name: row.EntryProcedureName, Signature: row.EntryProcedureSignature, PDBName: c.getFullPDBName(pdb)}, QuerySignature: row.QuerySignature, Source: "activity_entry"},
		{procedureIdentity: procedureIdentity{Name: row.CurrentProcedureName, Signature: row.CurrentProcedureSignature, PDBName: c.getFullPDBName(pdb)}, QuerySignature: row.QuerySignature, Source: "activity_current"},
	} {
		if association.Signature == "" {
			continue
		}
		_, known := c.procedureActivityAssociations[association]
		if known || len(c.procedureActivityAssociations) < c.config.StoredProcedures.DBRowsLimit {
			c.procedureActivityAssociations[association] = now
		}
	}
}

func (c *Check) observeCurrentProcedureQuery(sample OracleActivityRowDB, row OracleActivityRow, completeSQLText bool) {
	if !c.config.StoredProcedures.Enabled || !completeSQLText || row.PreviousSQL || row.SQLID == "" ||
		row.QuerySignature == "" || sample.Status != "ACTIVE" {
		return
	}
	// An anonymous call wrapper is not a child SQL association to itself.
	switch sample.CommandName.String {
	case "SELECT", "INSERT", "UPDATE", "DELETE", "MERGE":
		c.observeProcedureQuery(row, sample.PdbName.String)
	}
}

func (current procedureCursorBaseline) delta(previous procedureCursorBaseline) (procedureCounters, bool) {
	if current.loads != previous.loads || current.invalidations != previous.invalidations || current.objectID != previous.objectID || current.lastDDL != previous.lastDDL || current.signature != previous.signature {
		return procedureCounters{}, false
	}
	d := procedureCounters{
		Executions:  current.counts.Executions - previous.counts.Executions,
		ElapsedTime: current.counts.ElapsedTime - previous.counts.ElapsedTime,
		CPUTime:     current.counts.CPUTime - previous.counts.CPUTime,
		BufferGets:  current.counts.BufferGets - previous.counts.BufferGets,
		DiskReads:   current.counts.DiskReads - previous.counts.DiskReads,
	}
	return d, d.Executions >= 0 && d.ElapsedTime >= 0 && d.CPUTime >= 0 && d.BufferGets >= 0 && d.DiskReads >= 0
}

func (r *procedureRow) addCounters(d procedureCounters) {
	if r.Executions == nil {
		r.Executions, r.ElapsedTime, r.CPUTime, r.BufferGets, r.DiskReads = new(float64), new(float64), new(float64), new(float64), new(float64)
	}
	*r.Executions += d.Executions
	*r.ElapsedTime += d.ElapsedTime
	*r.CPUTime += d.CPUTime
	*r.BufferGets += d.BufferGets
	*r.DiskReads += d.DiskReads
}

func (c *Check) collectStoredProcedures() error {
	if !isDbVersionGreaterOrEqualThan(c, minMultitenantVersion) {
		return errors.New("stored_procedures requires Oracle 12c or newer")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(c.config.StoredProcedures.MaxRunTime)*time.Second)
	defer cancel()
	var cursors []procedureCursor
	lookback := c.procedureLookback()
	filter := "s.command_type IN (47,170) OR s.program_id > 0"
	if c.multitenant {
		// Unusable origin cursors must not consume the direct-call row budget.
		filter = "s.command_type IN (47,170) AND s.program_id = 0"
	}
	if err := c.db.SelectContext(ctx, &cursors, fmt.Sprintf(procedureCursorsQuery, filter), lookback, c.config.StoredProcedures.DBRowsLimit); err != nil {
		return err
	}
	resolver := procedureResolver{c: c, ctx: ctx, objects: make(map[procedureObjectKey]*procedureObject)}
	rows := make(map[string]*procedureRow)
	for signature, observed := range c.procedureActivityObserved {
		if c.clock.Now().Sub(observed.seen) <= time.Duration(c.procedureLookback())*time.Second {
			rows[signature] = &procedureRow{procedureIdentity: observed.identity, MetricsSource: "observed", Observed: 1}
		}
	}
	associations := make(map[procedureQueryAssociation]struct{})
	for association, seen := range c.procedureActivityAssociations {
		if c.clock.Now().Sub(seen) <= time.Duration(c.procedureLookback())*time.Second {
			associations[association] = struct{}{}
		}
	}
	next := make(map[procedureCursorKey]procedureCursorBaseline)
	observe := func(object *procedureObject, pdb string) *procedureRow {
		identity := object.identity(c.getFullPDBName(pdb))
		row := rows[identity.Signature]
		if row == nil {
			row = &procedureRow{procedureIdentity: identity, MetricsSource: "observed", Observed: 1}
			rows[identity.Signature] = row
		}
		return row
	}
	for _, cursor := range cursors {
		if cursor.ConID > 0 && !cursor.PDBName.Valid {
			continue
		}
		if cursor.ProgramID > 0 {
			object, err := resolver.cursorOrigin(cursor)
			if err != nil {
				return err
			}
			if object != nil {
				row := observe(object, cursor.PDBName.String)
				// Truncated text cannot establish the same query identity as query metrics.
				if utf8.RuneCountInString(cursor.SQLText) == cursor.SQLTextLength {
					statement, err := c.GetObfuscatedStatement(c.LazyInitObfuscator(), cursor.SQLText)
					if err == nil && statement.QuerySignature != "" {
						associations[procedureQueryAssociation{procedureIdentity: row.procedureIdentity, QuerySignature: statement.QuerySignature, ProgramLine: cursor.ProgramLine, Source: "cursor_origin"}] = struct{}{}
					}
				}
			}
		}
		if !cursor.ParsingSchema.Valid || cursor.ProgramID != 0 || (cursor.CommandType != 47 && cursor.CommandType != 170) || utf8.RuneCountInString(cursor.SQLText) != cursor.SQLTextLength {
			continue
		}
		owner, name, qualified, ok := parseDirectProcedureCall(cursor.SQLText, cursor.ParsingSchema.String)
		if !ok {
			continue
		}
		key := procedureObjectKey{conID: cursor.ConID, owner: owner, name: name}
		if qualified {
			key.parsingSchema = cursor.ParsingSchema.String
		}
		object, err := resolver.resolve(key)
		if err != nil {
			return err
		}
		if object == nil {
			continue
		}
		row := observe(object, cursor.PDBName.String)
		row.MetricsSource = "direct_call"
		baseline := procedureCursorBaseline{counts: cursor.procedureCounters, loads: cursor.Loads, invalidations: cursor.Invalidations, objectID: object.ObjectID, lastDDL: object.LastDDL, signature: row.Signature}
		if previous, found := c.procedureCursors[cursor.procedureCursorKey]; found {
			if delta, valid := baseline.delta(previous); valid {
				row.addCounters(delta)
			}
		}
		next[cursor.procedureCursorKey] = baseline
	}
	payload := procedureMetricsPayload{
		Host: c.dbHostname, DatabaseInstance: c.dbInstanceIdentifier,
		Timestamp: float64(c.clock.Now().UnixMilli()), MinCollectionInterval: float64(c.config.StoredProcedures.CollectionInterval),
		Tags: c.tags, AgentVersion: c.agentVersion, AgentHostname: c.agentHostname, Kind: "procedure_metrics", OracleVersion: c.dbVersion,
		OracleRows: make([]procedureRow, 0, len(rows)),
	}
	for _, row := range rows {
		payload.OracleRows = append(payload.OracleRows, *row)
	}
	for association := range associations {
		payload.Associations = append(payload.Associations, association)
	}
	sort.Slice(payload.OracleRows, func(i, j int) bool { return payload.OracleRows[i].Signature < payload.OracleRows[j].Signature })
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	sender, err := c.GetSender()
	if err != nil {
		return err
	}
	sender.EventPlatformEvent(data, "dbm-metrics")
	// A failed/partial collection must not advance the baseline. Missing cursors
	// expire here; their eventual return establishes a fresh baseline.
	c.procedureCursors = next
	c.procedureActivityObserved = nil
	c.procedureActivityAssociations = nil
	return nil
}
