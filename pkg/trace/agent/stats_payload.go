package agent

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
)

// StatsPayload represents the payload to be flushed to the stats endpoint
type StatsPayload struct {
	HostName string        `json:"hostname"`
	Env      string        `json:"env"`
	Stats    []StatsBucket `json:"stats"`
}

// EncodeStatsPayload encodes the stats payload as json/gzip.
func EncodeStatsPayload(payload *StatsPayload) ([]byte, error) {
	var b bytes.Buffer
	var err error

	gz, err := gzip.NewWriterLevel(&b, gzip.BestSpeed)
	if err != nil {
		return nil, err
	}
	err = json.NewEncoder(gz).Encode(payload)
	gz.Close()

	return b.Bytes(), err
}
