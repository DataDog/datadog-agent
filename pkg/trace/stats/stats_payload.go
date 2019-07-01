package stats

import (
	"compress/gzip"
	"encoding/json"
	"io"
)

// Payload represents the payload to be flushed to the stats endpoint
type Payload struct {
	HostName string   `json:"hostname"`
	Env      string   `json:"env"`
	Stats    []Bucket `json:"stats"`
}

// EncodePayload encodes the payload as Gzipped JSON into w.
func EncodePayload(w io.Writer, payload *Payload) error {
	gz, err := gzip.NewWriterLevel(w, gzip.BestSpeed)
	if err != nil {
		return err
	}
	defer gz.Close()
	return json.NewEncoder(gz).Encode(payload)
}
