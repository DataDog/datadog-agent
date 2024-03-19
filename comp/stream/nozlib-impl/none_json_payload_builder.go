package streamimpl

import (
	"errors"

	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/transaction"
	"github.com/DataDog/datadog-agent/comp/stream"
	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
)

type noneJSONPayloadBuilder struct{}

func (n *noneJSONPayloadBuilder) BuildWithOnErrItemTooBigPolicy(
	m marshaler.IterableStreamJSONMarshaler,
	policy stream.OnErrItemTooBigPolicy) (transaction.BytesPayloads, error) {
	return nil, errors.New("not implemented")
}
