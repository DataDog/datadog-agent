// +build linux windows darwin

package v5

import (
	"encoding/json"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/metadata/gohai"
	"github.com/stretchr/testify/require"
)

func TestGohaiPayloadMarshalling(t *testing.T) {
	gp := gohai.GetPayload()
	payload := GohaiPayload{MarshalledGohaiPayload{*gp}}
	marshalled, err := json.Marshal(payload)
	require.Nil(t, err)

	var gohaiPayload GohaiPayload
	err = json.Unmarshal(marshalled, &gohaiPayload)
	require.Nil(t, err)
}
