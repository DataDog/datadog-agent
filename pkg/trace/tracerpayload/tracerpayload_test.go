package tracerpayload

import (
	_ "embed"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vmihailenco/msgpack/v4"
)

//go:embed testdata/10traces210spans.msgp.v0.5
var rawPayload []byte

func TestUnmarshalPayload(t *testing.T) {
	var payload MsgpPayload

	err := msgpack.Unmarshal(rawPayload, &payload)
	assert.NoError(t, err)

	ts := MakeTraces(&payload)
	fmt.Printf("%v\n", ts)
	assert.Fail(t, "")
}

// TODO: tests are cool
