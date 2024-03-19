package stream

import (
	"errors"

	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/transaction"
	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
)

var (
	// ErrPayloadFull is returned when the payload buffer is full
	ErrPayloadFull = errors.New("reached maximum payload size")

	// ErrItemTooBig is returned when a item alone exceeds maximum payload size
	ErrItemTooBig = errors.New("item alone exceeds maximum payload size")
)

// OnErrItemTooBigPolicy defines the behavior when OnErrItemTooBig occurs.
type OnErrItemTooBigPolicy int

const (
	// DropItemOnErrItemTooBig skips the error and continues when ErrItemTooBig is encountered
	DropItemOnErrItemTooBig OnErrItemTooBigPolicy = iota

	// FailOnErrItemTooBig returns the error and stop when ErrItemTooBig is encountered
	FailOnErrItemTooBig
)

type JSONPayloadBuilder interface {
	BuildWithOnErrItemTooBigPolicy(
		m marshaler.IterableStreamJSONMarshaler,
		policy OnErrItemTooBigPolicy) (transaction.BytesPayloads, error)
}
