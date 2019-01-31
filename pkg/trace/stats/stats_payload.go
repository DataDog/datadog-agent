package stats

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
)

// Payload represents the payload to be flushed to the stats endpoint
type Payload struct {
	HostName string   `json:"hostname"`
	Env      string   `json:"env"`
	Stats    []Bucket `json:"stats"`
}

// EncodePayload encodes the stats payload as json/gzip.
func EncodePayload(payload *Payload) ([]byte, error) {
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
