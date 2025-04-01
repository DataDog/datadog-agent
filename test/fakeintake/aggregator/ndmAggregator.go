package aggregator

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/networkdevice/metadata"
	"github.com/DataDog/datadog-agent/test/fakeintake/api"
)

// NDMPayload represents an NDM payload
type NDMPayload struct {
	collectedTime time.Time
	metadata.NetworkDevicesMetadata
}

func (p *NDMPayload) name() string {
	return fmt.Sprintf("%s:%s integration:%s", p.Namespace, p.Subnet, p.Integration)
}

// GetTags return the tags from a payload
func (p *NDMPayload) GetTags() []string {
	return []string{}
}

// GetCollectedTime return the time when the payload has been collected by the fakeintake server
func (p *NDMPayload) GetCollectedTime() time.Time {
	return p.collectedTime
}

// ParseNDMPayload parses an api.Payload into a list of NDMPayload
func ParseNDMPayload(payload api.Payload) (ndmPayloads []*NDMPayload, err error) {
	if len(payload.Data) == 0 || bytes.Equal(payload.Data, []byte("{}")) {
		return []*NDMPayload{}, nil
	}
	inflated, err := inflate(payload.Data, payload.Encoding)
	if err != nil {
		return nil, err
	}
	ndmPayloads = []*NDMPayload{}
	err = json.Unmarshal(inflated, &ndmPayloads)
	if err != nil {
		return nil, err
	}
	for _, n := range ndmPayloads {
		n.collectedTime = payload.Timestamp
	}
	return ndmPayloads, err
}

// NDMAggregator is an Aggregator for NDM devices payloads
type NDMAggregator struct {
	Aggregator[*NDMPayload]
}

// NewNDMAggregator return a new NDMAggregator
func NewNDMAggregator() NDMAggregator {
	return NDMAggregator{
		Aggregator: newAggregator(ParseNDMPayload),
	}
}
