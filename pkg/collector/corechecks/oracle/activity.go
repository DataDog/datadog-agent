// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build oracle

package oracle

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/oracle/common"
	"github.com/DataDog/datadog-agent/pkg/obfuscate"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Converts sql types to Go native types
func (c *Check) getSQLRow(SQLID sql.NullString, forceMatchingSignature *string, SQLPlanHashValue *uint64, SQLExecStart sql.NullString) (OracleSQLRow, error) {
	SQLRow := OracleSQLRow{}
	if SQLID.Valid {
		SQLRow.SQLID = SQLID.String
	} else {
		SQLRow.SQLID = ""
		return SQLRow, nil
	}
	if forceMatchingSignature != nil {
		forceMatchingSignatureUint64, err := strconv.ParseUint(*forceMatchingSignature, 10, 64)
		if err != nil {
			return SQLRow, fmt.Errorf("failed converting force_matching_signature to uint64 %w", err)
		}
		SQLRow.ForceMatchingSignature = forceMatchingSignatureUint64
	} else {
		SQLRow.ForceMatchingSignature = 0
	}
	if SQLPlanHashValue != nil {
		SQLRow.SQLPlanHashValue = *SQLPlanHashValue
	}
	if SQLExecStart.Valid {
		SQLRow.SQLExecStart = SQLExecStart.String
	}
	return SQLRow, nil
}

func sendPayload(c *Check, sessionRows []OracleActivityRow, timestamp float64) error {
	var collectionInterval float64
	if c.config.QuerySamples.ActiveSessionHistory {
		collectionInterval = 1
	} else {
		collectionInterval = float64(c.config.MinCollectionInterval)
	}
	var ts float64
	if timestamp > 0 {
		ts = timestamp
	} else {
		ts = float64(c.clock.Now().UnixMilli())
	}
	log.Debugf("%s STIMESTAMP FETCHED %f", c.logPrompt, timestamp)
	log.Debugf("%s STIMESTAMP UNIX    %f", c.logPrompt, float64(c.clock.Now().UnixMilli()))
	payload := ActivitySnapshot{
		Metadata: Metadata{
			//Timestamp:      float64(c.clock.Now().UnixMilli()),
			Timestamp:      ts,
			Host:           c.dbHostname,
			Source:         common.IntegrationName,
			DBMType:        "activity",
			DDAgentVersion: c.agentVersion,
		},
		CollectionInterval: collectionInterval,
		Tags:               c.tags,
		OracleActivityRows: sessionRows,
	}

	c.lastOracleActivityRows = make([]OracleActivityRow, len(sessionRows))
	copy(c.lastOracleActivityRows, sessionRows)

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		log.Errorf("%s Error marshalling activity payload: %s", c.logPrompt, err)
		return err
	}

	log.Debugf("%s Activity payload %s", c.logPrompt, strings.ReplaceAll(string(payloadBytes), "@", "XX"))

	sender, err := c.GetSender()
	if err != nil {
		log.Errorf("%s GetSender SampleSession %s", c.logPrompt, string(payloadBytes))
		return err
	}
	sender.EventPlatformEvent(payloadBytes, "dbm-activity")
	sendMetric(c, count, "dd.oracle.activity.samples_count", float64(len(sessionRows)), append(c.tags, fmt.Sprintf("sql_substring_length:%d", c.sqlSubstringLength)))

	return nil
}

//nolint:revive // TODO(DBM) Fix revive linter
func (c *Check) SampleSession() error {
	activeSessionHistory := c.config.QuerySamples.ActiveSessionHistory
	if activeSessionHistory && c.lastSampleID == 0 {
		err := getWrapper(c, &c.lastSampleID, "SELECT /* DD */ NVL(MAX(sample_id),0) FROM v$active_session_history")
		if err != nil {
			return err
		}
		if c.lastSampleID == 0 {
			log.Infof("%s no active session history samples found", c.logPrompt)
			return nil
		}
	}
	start := time.Now()
	copy(c.lastOracleActivityRows, []OracleActivityRow{})

	var sessionRows []OracleActivityRow
	sessionSamples := []OracleActivityRowDB{}
	var activityQuery string
	maxSQLTextLength := c.sqlSubstringLength

	if activeSessionHistory {
		activityQuery = activityQueryActiveSessionHistory
	} else if c.hostingType == selfManaged && !c.config.QuerySamples.ForceDirectQuery {
		if isDbVersionGreaterOrEqualThan(c, minMultitenantVersion) {
			activityQuery = activityQueryOnView12
		} else {
			activityQuery = activityQueryOnView11
		}
	} else {
		activityQuery = activityQueryDirect
	}

	if !c.config.QuerySamples.IncludeAllSessions && !activeSessionHistory {
		activityQuery = fmt.Sprintf("%s %s", activityQuery, ` AND (
	NOT (state = 'WAITING' AND wait_class = 'Idle')
	OR state = 'WAITING' AND event = 'fbar timer' AND type = 'USER'
)
AND status = 'ACTIVE'`)
	}

	var err error
	if activeSessionHistory {
		err = selectWrapper(c, &sessionSamples, activityQuery, maxSQLTextLength, c.lastSampleID)
	} else {
		err = selectWrapper(c, &sessionSamples, activityQuery, maxSQLTextLength, maxSQLTextLength)
	}

	if err != nil {
		if strings.Contains(err.Error(), "ORA-06502") {
			if c.sqlSubstringLength > 1000 {
				c.sqlSubstringLength = max(c.sqlSubstringLength-500, 1000)
				sendMetricWithDefaultTags(c, count, "dd.oracle.activity.decrease_sql_substring_length", float64(c.sqlSubstringLength))
				return nil
			}
		}
		return fmt.Errorf("failed to collect session sampling activity: %w \n%s", err, activityQuery)
	}

	o := obfuscate.NewObfuscator(obfuscate.Config{SQL: c.config.ObfuscatorOptions})
	defer o.Stop()
	var payloadSent bool
	var lastNow string
	for _, sample := range sessionSamples {
		var sessionRow OracleActivityRow

		if activeSessionHistory && sample.SampleID > c.lastSampleID {
			c.lastSampleID = sample.SampleID
		}

		sessionRow.Now = sample.Now
		if lastNow != sessionRow.Now && lastNow != "" {
			err = sendPayload(c, sessionRows, sample.UtcMs)
			if err != nil {
				log.Errorf("%s error sending payload %s", c.logPrompt, err)
			}
			payloadSent = true
		}
		lastNow = sessionRow.Now

		sessionRow.SessionID = sample.SessionID
		sessionRow.SessionSerial = sample.SessionSerial
		if sample.User.Valid {
			sessionRow.User = sample.User.String
		}
		sessionRow.Status = sample.Status
		if sample.OsUser.Valid {
			sessionRow.OsUser = sample.OsUser.String
		}
		if sample.Process.Valid {
			sessionRow.Process = sample.Process.String
		}
		if sample.Client.Valid {
			sessionRow.Client = sample.Client.String
		}
		if sample.Port.Valid {
			sessionRow.Port = strconv.FormatInt(int64(sample.Port.Int64), 10)
		}

		program := ""
		if sample.Program.Valid {
			sessionRow.Program = sample.Program.String
			program = sample.Program.String
		}

		sessionType := ""
		if sample.Type.Valid {
			sessionRow.Type = sample.Type.String
			sessionType = sample.Type.String
		}

		commandName := ""
		if sample.CommandName.Valid {
			commandName = sample.CommandName.String
		}
		sessionRow.CommandName = commandName
		previousSQL := false
		sqlCurrentSQL, err := c.getSQLRow(sample.SQLID, sample.ForceMatchingSignature, sample.SQLPlanHashValue, sample.SQLExecStart)
		if err != nil {
			log.Errorf("%s error getting SQL row %s", c.logPrompt, err)
		}

		var sqlPrevSQL OracleSQLRow
		if sqlCurrentSQL.SQLID != "" {
			sessionRow.OracleSQLRow = sqlCurrentSQL
		} else {
			sqlPrevSQL, err = c.getSQLRow(sample.PrevSQLID, sample.PrevForceMatchingSignature, sample.PrevSQLPlanHashValue, sample.PrevSQLExecStart)
			if err != nil {
				log.Errorf("%s error getting SQL row %s", c.logPrompt, err)
			}
			if sqlPrevSQL.SQLID != "" {
				sessionRow.OracleSQLRow = sqlPrevSQL
				previousSQL = true
			}
		}
		sessionRow.PreviousSQL = previousSQL

		if sample.Module.Valid {
			sessionRow.Module = sample.Module.String
		}
		if sample.Action.Valid {
			sessionRow.Action = sample.Action.String
		}
		if sample.ClientInfo.Valid {
			sessionRow.ClientInfo = sample.ClientInfo.String
		}
		if sample.LogonTime.Valid {
			sessionRow.LogonTime = sample.LogonTime.String
		}
		if sample.ClientIdentifier.Valid {
			sessionRow.ClientIdentifier = sample.ClientIdentifier.String
		}
		if sample.BlockingInstance != nil {
			sessionRow.BlockingInstance = *sample.BlockingInstance
		}
		if sample.BlockingSession != nil {
			sessionRow.BlockingSession = *sample.BlockingSession
		}
		if sample.FinalBlockingInstance != nil {
			sessionRow.FinalBlockingInstance = *sample.FinalBlockingInstance
		}
		if sample.FinalBlockingSession != nil {
			sessionRow.FinalBlockingSession = *sample.FinalBlockingSession
		}
		if sample.WaitEvent.Valid {
			sessionRow.WaitEvent = sample.WaitEvent.String
		}
		if sample.WaitEventClass.Valid {
			sessionRow.WaitEventClass = sample.WaitEventClass.String
		}
		if sample.WaitTimeMicro != nil {
			sessionRow.WaitTimeMicro = *sample.WaitTimeMicro
		}
		sessionRow.OpFlags = sample.OpFlags

		statement := ""
		obfuscate := true
		var hasRealSQLText bool
		if sample.Statement.Valid && sample.Statement.String != "" && !previousSQL {
			// If we captured the statement, we are assigning the value
			statement = sample.Statement.String
			hasRealSQLText = true
		} else if previousSQL && sample.PrevSQLFullText.Valid && sample.PrevSQLFullText.String != "" {
			statement = sample.PrevSQLFullText.String
			hasRealSQLText = true
		} else if (sample.OpFlags & 8) == 8 {
			statement = "LOG ON/LOG OFF"
			obfuscate = false
		} else if commandName != "" {
			statement = commandName
		} else if sessionType == "BACKGROUND" {
			statement = program
			obfuscate = false
		} else if sample.Module.Valid && sample.Module.String == "DBMS_SCHEDULER" {
			statement = sample.Module.String
			obfuscate = false
		} else {
			log.Debugf("activity sql text empty for %#v \n", sample)
		}

		if hasRealSQLText {
			/*
			 * If the statement length is maxSQLTextLength characters, we are assuming that the statement was truncated,
			 * so we are trying to fetch it complete. The full statement is stored in a LOB, so we are calling
			 * getFullSQLText which doesn't leak PGA memory
			 */
			if len(statement) == maxSQLTextLength {
				var fetchedStatement string
				err = getFullSQLText(c, &fetchedStatement, "sql_id", sessionRow.SQLID)
				if err != nil {
					log.Warnf("%s failed to fetch full sql text for the current sql_id: %s", c.logPrompt, err)
				}
				if fetchedStatement != "" {
					statement = fetchedStatement
				}
			}
		} else {
			if (sample.OpFlags & 128) == 128 {
				statement = fmt.Sprintf("%s IN HARD PARSE", statement)
			} else if (sample.OpFlags & 16) == 16 {
				statement = fmt.Sprintf("%s IN PARSE", statement)
			}
			if (sample.OpFlags & 65536) == 65536 {
				statement = fmt.Sprintf("%s IN CURSOR CLOSING", statement)
			}
		}

		if statement != "" && obfuscate {
			obfuscatedStatement, err := c.GetObfuscatedStatement(o, statement)
			sessionRow.Statement = obfuscatedStatement.Statement
			if err == nil {
				sessionRow.Commands = obfuscatedStatement.Commands
				sessionRow.Tables = obfuscatedStatement.Tables
				sessionRow.Comments = obfuscatedStatement.Comments
				sessionRow.QuerySignature = obfuscatedStatement.QuerySignature
			}
		} else {
			sessionRow.Statement = statement
			sessionRow.QuerySignature = common.GetQuerySignature(statement)
		}

		if sample.PdbName.Valid {
			sessionRow.PdbName = sample.PdbName.String
		}
		if sessionRow.PdbName == "" {
			if c.multitenant {
				sessionRow.PdbName = "CDB$ROOT"
			} else {
				sessionRow.PdbName = c.cdbName
			}
		}
		sessionRow.CdbName = c.cdbName
		sessionRows = append(sessionRows, sessionRow)
	}
	if !payloadSent {
		err = sendPayload(c, sessionRows, 0)
		if err != nil {
			log.Errorf("%s error sending payload %s", c.logPrompt, err)
		}
	}
	sender, err := c.GetSender()
	if err != nil {
		log.Errorf("%s GetSender SampleSession", c.logPrompt)
		return err
	}
	sendMetricWithDefaultTags(c, gauge, "dd.oracle.activity.time_ms", float64(time.Since(start).Milliseconds()))
	TlmOracleActivityLatency.Observe(float64(time.Since(start).Milliseconds()))
	TlmOracleActivitySamplesCount.Set(float64(len(sessionRows)))

	sender.Commit()

	return nil
}
