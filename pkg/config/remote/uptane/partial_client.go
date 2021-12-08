package uptane

import "github.com/DataDog/datadog-agent/pkg/proto/pbgo"

type PartialClient struct {
}

func NewPartialClient() *PartialClient {
	return &PartialClient{}
}

func (c *PartialClient) Verify(response *pbgo.ConfigResponse) error {
	return nil
}
