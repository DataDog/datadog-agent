package types

import "github.com/DataDog/datadog-agent/pkg/appsec/api/http/v0_1_0/types"

type (
	RawJSONEventsChan       chan types.RawJSONEventSlice
	RawJSONEventsBatchSlice []types.RawJSONEventSlice
	RawJSONEventsBatchChan  chan RawJSONEventsBatchSlice
)
