package oracle

import (
	"encoding/json"
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/oracle/common"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/version"
)

type OracleActivityRow struct {
	SessionID int64  `db:"SID" json:"sid"`
	SqlID     string `db:"SQL_ID" json:"sql_id,omitempty"`
}

// ActivitySnapshot is a payload containing database activity samples. It is parsed from the intake payload.
// easyjson:json
type ActivitySnapshot struct {
	Metadata
	// Tags should be part of the common Metadata struct but because Activity payloads use a string array
	// and samples use a comma-delimited list of tags in a single string, both flavors have to be handled differently
	Tags               []string            `json:"ddtags,omitempty"`
	CollectionInterval float64             `json:"collection_interval,omitempty"`
	OracleActivityRows []OracleActivityRow `json:"oracle_activity,omitempty"`
}

// Metadata contains the metadata fields common to all events processed
// easyjson:json
type Metadata struct {
	Timestamp      float64 `json:"timestamp,omitempty"`
	Service        string  `json:"service,omitempty"`
	Host           string  `json:"host,omitempty"`
	Source         string  `json:"ddsource,omitempty"`
	DBMType        string  `json:"dbm_type,omitempty"`
	DDAgentVersion string  `json:"ddagentversion,omitempty"`
}

type MetricSender struct {
	sender           aggregator.Sender
	hostname         string
	submittedMetrics int
}

func (c *Check) SampleSession() error {
	sessionSamples := []OracleActivityRow{}
	err := c.db.Select(&sessionSamples, "SELECT sid, sql_id FROM v$session WHERE rownum < 5")
	if err != nil {
		log.Errorf("Session sampling ", err)
		return err
	}
	//log.Tracef("orasample %#v", sessionSamples)

	agentVersion, _ := version.Agent()

	payload := ActivitySnapshot{
		Metadata: Metadata{
			Timestamp:      float64(time.Now().UnixMilli()),
			Host:           c.hostname,
			DDAgentVersion: agentVersion.GetNumberAndPre(),
			Source:         common.IntegrationName,
			DBMType:        "activity"},
		CollectionInterval: 1,
		Tags:               []string{},
		OracleActivityRows: sessionSamples,
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		log.Errorf("Error marshalling device metadata: %s", err)
		return err
	}
	log.Tracef("JSON payload", string(payloadBytes))

	sender, err := c.GetSender()
	if err != nil {
		log.Tracef("GetSender SampleSession ", string(payloadBytes))
		return err
	}
	sender.EventPlatformEvent(string(payloadBytes), "dbm-activity")
	sender.Commit()
	return nil
}
