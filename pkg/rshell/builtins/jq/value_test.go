// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package jq

import (
	"context"
	"io"
	"math"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOrderedObjectDuplicateKeepsPositionAndLastValue(t *testing.T) {
	v, err := parseSingleJSON(context.Background(), `{"b":1,"a":2,"b":3}`)
	require.NoError(t, err)

	got, err := encodeValue(v, false, MaxOutputBytes)
	require.NoError(t, err)
	assert.Equal(t, `{"b":3,"a":2}`, got)
}

func TestJSONEncodingDoesNotEscapeHTML(t *testing.T) {
	v, err := stringValue("<tag>&")
	require.NoError(t, err)
	got, err := encodeValue(v, false, MaxOutputBytes)
	require.NoError(t, err)
	assert.Equal(t, `"<tag>&"`, got)
}

func TestNumberBoundsAndUnderflow(t *testing.T) {
	underflow, err := parseNumber("1e-400")
	require.NoError(t, err)
	assert.Equal(t, float64(0), underflow.num.float)

	overflow, err := parseNumber("1e400")
	require.NoError(t, err)
	assert.Equal(t, math.MaxFloat64, overflow.num.float)

	// The bound prevents repeated squaring from exhausting memory.
	_, err = parseNumber(strings.Repeat("9", 78))
	assert.ErrorIs(t, err, errIntegerRange)
}

func TestContainerDepthIsConsistent(t *testing.T) {
	text := strings.Repeat("[", MaxNestingDepth) + "0" + strings.Repeat("]", MaxNestingDepth)
	v, err := parseSingleJSON(context.Background(), text)
	require.NoError(t, err)
	assert.Equal(t, MaxNestingDepth, v.depth)

	_, err = arrayValue([]value{v})
	assert.ErrorIs(t, err, errValueDepth)

	tooDeep := "[" + text + "]"
	_, err = parseSingleJSON(context.Background(), tooDeep)
	assert.ErrorIs(t, err, errValueDepth)
}

func TestSurrogateValidation(t *testing.T) {
	_, err := parseSingleJSON(context.Background(), `"\ud800"`)
	assert.ErrorIs(t, err, errInvalidSurrogate)

	v, err := parseSingleJSON(context.Background(), `"\ud83d\ude00"`)
	require.NoError(t, err)
	assert.Equal(t, "😀", v.str)

	v, err = parseSingleJSON(context.Background(), "\"\xff\"")
	require.NoError(t, err)
	assert.Equal(t, "�", v.str)
}

func TestUnterminatedStringDrainPreservesCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	reader := io.MultiReader(
		strings.NewReader(`{"x":"\qbad`),
		&cancelingReader{reader: strings.NewReader(`"`), cancel: cancel},
	)

	_, err := newJSONValueDecoder(ctx, reader).next()
	assert.ErrorIs(t, err, context.Canceled)
}

func TestProvisionalObjectKeyWhitespaceDrainIsBuffered(t *testing.T) {
	reader := &countingReader{reader: strings.NewReader("{ 0\n" + strings.Repeat(" ", 1<<20))}

	_, err := newJSONValueDecoder(context.Background(), reader).next()
	assert.Error(t, err)
	assert.Less(t, reader.reads, 100)
}

func TestProvisionalObjectKeyDrainPushesBackTrailingBytes(t *testing.T) {
	pad := strings.Repeat(" ", 8<<10)
	for _, input := range []string{
		`{ 0` + "\n" + pad + `abcdef`,
		`{ 0` + "\n" + pad + `"abc" trailing`,
		`{ 0` + "\n" + pad + `abc]]]`,
	} {
		t.Run(strings.TrimSpace(input[len(input)-12:]), func(t *testing.T) {
			decoder := newJSONValueDecoder(context.Background(), strings.NewReader(input))

			_, err := decoder.next()
			assert.Error(t, err)
			assert.NotContains(t, err.Error(), "invalid JSON")
			assert.LessOrEqual(t, decoder.stream.base+int64(len(decoder.stream.delivered)), int64(len(input)))
		})
	}
}

func TestStreamReplayBufferDoesNotRetainWholeStream(t *testing.T) {
	const values = 20000
	decoder := newJSONValueDecoder(context.Background(), strings.NewReader(strings.Repeat(`{"a":1}`+"\n", values)))

	widest := 0
	for range values {
		if _, err := decoder.next(); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if got := len(decoder.stream.delivered); got > widest {
			widest = got
		}
	}
	assert.Less(t, widest, 2*inputReadChunk)
}

func TestStreamReplayCompactionIsAmortized(t *testing.T) {
	const size = 1 << 20
	data := make([]byte, size)
	allocations := testing.AllocsPerRun(1, func() {
		stream := jsonStreamReader{delivered: data}
		for offset := int64(inputReadChunk); offset < size; offset += inputReadChunk {
			stream.discardBefore(offset)
		}
	})
	assert.Less(t, allocations, 10.0)
}

func TestStreamReplayRebaseKeepsUnterminatedTokenDrain(t *testing.T) {
	decoder := newJSONValueDecoder(context.Background(), strings.NewReader(`{"a":1} "unterminated`))

	_, err := decoder.next()
	require.NoError(t, err)

	_, err = decoder.next()
	assert.Error(t, err)
	assert.NotErrorIs(t, err, io.EOF)
	assert.NotContains(t, err.Error(), "invalid JSON decoder offset")
	assert.NotZero(t, decoder.stream.base)
}

type cancelingReader struct {
	reader io.Reader
	cancel context.CancelFunc
}

func (r *cancelingReader) Read(p []byte) (int, error) {
	r.cancel()
	return r.reader.Read(p)
}

type countingReader struct {
	reader io.Reader
	reads  int
}

func (r *countingReader) Read(p []byte) (int, error) {
	r.reads++
	return r.reader.Read(p)
}

func TestFloatValueClampsOverflowAndRejectsNaN(t *testing.T) {
	positive, err := floatValue(math.Inf(1))
	assert.NoError(t, err)
	assert.Equal(t, "1.7976931348623157e+308", positive.num.text())

	negative, err := floatValue(math.Inf(-1))
	assert.NoError(t, err)
	assert.Equal(t, "-1.7976931348623157e+308", negative.num.text())

	_, err = floatValue(math.NaN())
	assert.ErrorContains(t, err, "not finite")
}

func TestResultAccumulatorChecksAggregateBeforeAppend(t *testing.T) {
	large, err := stringValue(strings.Repeat("x", MaxValueBytes-2))
	require.NoError(t, err)
	results := newResultAccumulator(3)
	require.NoError(t, results.add(large))
	require.NoError(t, results.add(large))
	assert.ErrorIs(t, results.add(large), errResultLimit)
	assert.Len(t, results.values, 2)
}
