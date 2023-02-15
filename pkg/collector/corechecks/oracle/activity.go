package oracle

import (
	"encoding/json"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type Session struct {
	Sid    int64  `db:"SID" json:"sid"`
	Sql_id string `db:"SQL_ID" json:"sql_id,omitempty"`
}

type ActivityPayload struct {
	Host               string    `json:"host"`
	DDAgentVersion     string    `json:"ddagentversion"`
	DDSource           string    `json:"ddsource"`
	DBMType            string    `json:"dbm_type"`
	CollectionInterval int       `json:"collection_interval"`
	DDTags             []string  `json:"ddtags"`
	Timestamp          int64     `json:"timestamp"`
	OracleActivity     []Session `json:"oracle_activity"`
}

func (c *Check) SampleSession() error {
	sessionSamples := []Session{}
	err := c.db.Select(&sessionSamples, "SELECT sid, sql_id FROM v$session WHERE rownum < 5")
	if err != nil {
		log.Errorf("Session sampling ", err)
		return err
	}
	//log.Tracef("orasample %#v", sessionSamples)

	payload := ActivityPayload{
		Host:               "a",
		DDAgentVersion:     "1",
		DDSource:           "oracle",
		DBMType:            "activity",
		CollectionInterval: 1,
		DDTags:             []string{"Espresso", "Educative", "Shots"},
		Timestamp:          1,
		OracleActivity:     sessionSamples,
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		log.Errorf("Error marshalling device metadata: %s", err)
		return err
	}
	log.Tracef("JSON payload", string(payloadBytes))

	return nil
}
