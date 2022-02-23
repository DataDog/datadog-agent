package decoder

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const contentLenLimit = 100

func lineBreakerChans() (func(*DecodedInput), chan *DecodedInput) {
	ch := make(chan *DecodedInput, 10)
	return func(di *DecodedInput) { ch <- di }, ch
}

func TestLineBreaking(t *testing.T) {
	test := func(chunks [][]byte) func(*testing.T) {
		return func(t *testing.T) {
			outputFn, outputChan := lineBreakerChans()
			lb := NewLineBreaker(outputFn, &NewLineMatcher{}, contentLenLimit)
			for _, chunk := range chunks {
				lb.process(chunk)
			}
			require.Equal(t, "line1", string((<-outputChan).content))
			require.Equal(t, "line2", string((<-outputChan).content))
			require.Equal(t, "line3", string((<-outputChan).content))
			require.Equal(t, "line4", string((<-outputChan).content))
		}
	}

	t.Run("with one chunk", test([][]byte{
		[]byte("line1\nline2\nline3\nline4\n"),
	}))

	t.Run("with chunk per line", test([][]byte{
		[]byte("line1\n"),
		[]byte("line2\n"),
		[]byte("line3\n"),
		[]byte("line4\n"),
	}))

	var bytes [][]byte
	for _, b := range []byte("line1\nline2\nline3\nline4\n") {
		bytes = append(bytes, []byte{b})
	}
	t.Run("with chunk per byte", test(bytes))
}

func TestLineBreakIncomingData(t *testing.T) {
	outputFn, outputChan := lineBreakerChans()
	lb := NewLineBreaker(outputFn, &NewLineMatcher{}, contentLenLimit)

	var line *DecodedInput

	// one line in one raw should be sent
	lb.process([]byte("helloworld\n"))
	line = <-outputChan
	assert.Equal(t, "helloworld", string(line.content))
	assert.Equal(t, len("helloworld\n"), line.rawDataLen)
	assert.Equal(t, "", lb.lineBuffer.String())

	// multiple lines in one raw should be sent
	lb.process([]byte("helloworld\nhowayou\ngoodandyou"))
	l := 0
	line = <-outputChan
	l += line.rawDataLen
	assert.Equal(t, "helloworld", string(line.content))
	line = <-outputChan
	l += line.rawDataLen
	assert.Equal(t, "howayou", string(line.content))
	assert.Equal(t, "goodandyou", lb.lineBuffer.String())
	assert.Equal(t, len("helloworld\nhowayou\n"), l)
	lb.lineBuffer.Reset()
	lb.rawDataLen, l = 0, 0

	// multiple lines in multiple rows should be sent
	lb.process([]byte("helloworld\nthisisa"))
	line = <-outputChan
	l += line.rawDataLen
	assert.Equal(t, "helloworld", string(line.content))
	assert.Equal(t, "thisisa", lb.lineBuffer.String())
	lb.process([]byte("longinput\nindeed"))
	line = <-outputChan
	l += line.rawDataLen
	assert.Equal(t, "thisisalonginput", string(line.content))
	assert.Equal(t, "indeed", lb.lineBuffer.String())
	assert.Equal(t, len("helloworld\nthisisalonginput\n"), l)
	lb.lineBuffer.Reset()
	lb.rawDataLen = 0

	// one line in multiple rows should be sent
	lb.process([]byte("hello world"))
	lb.process([]byte("!\n"))
	line = <-outputChan
	assert.Equal(t, "hello world!", string(line.content))
	assert.Equal(t, len("hello world!\n"), line.rawDataLen)

	// excessively long line in one row should be sent by chunks
	lb.process([]byte(strings.Repeat("a", contentLenLimit+10) + "\n"))
	line = <-outputChan
	assert.Equal(t, contentLenLimit, len(line.content))
	assert.Equal(t, contentLenLimit, line.rawDataLen)
	line = <-outputChan
	assert.Equal(t, strings.Repeat("a", 10), string(line.content))
	assert.Equal(t, 11, line.rawDataLen)

	// excessively long line in multiple rows should be sent by chunks
	lb.process([]byte(strings.Repeat("a", contentLenLimit-5)))
	lb.process([]byte(strings.Repeat("a", 15) + "\n"))
	line = <-outputChan
	assert.Equal(t, contentLenLimit, len(line.content))
	assert.Equal(t, contentLenLimit, line.rawDataLen)
	line = <-outputChan
	assert.Equal(t, strings.Repeat("a", 10), string(line.content))
	assert.Equal(t, 11, line.rawDataLen)

	// empty lines should be sent
	lb.process([]byte("\n"))
	line = <-outputChan
	assert.Equal(t, "", string(line.content))
	assert.Equal(t, "", lb.lineBuffer.String())
	assert.Equal(t, 1, line.rawDataLen)

	// empty message should not change anything
	lb.process([]byte(""))
	assert.Equal(t, "", lb.lineBuffer.String())
	assert.Equal(t, 0, lb.rawDataLen)
}

func TestLineBreakIncomingDataWithCustomSequence(t *testing.T) {
	outputFn, outputChan := lineBreakerChans()
	lb := NewLineBreaker(outputFn, NewBytesSequenceMatcher([]byte("SEPARATOR"), 1), contentLenLimit)

	var line *DecodedInput

	// one line in one raw should be sent
	lb.process([]byte("helloworldSEPARATOR"))
	line = <-outputChan
	assert.Equal(t, "helloworld", string(line.content))
	assert.Equal(t, "", lb.lineBuffer.String())

	// multiple lines in one raw should be sent
	lb.process([]byte("helloworldSEPARATORhowayouSEPARATORgoodandyou"))
	line = <-outputChan
	assert.Equal(t, "helloworld", string(line.content))
	line = <-outputChan
	assert.Equal(t, "howayou", string(line.content))
	assert.Equal(t, "goodandyou", lb.lineBuffer.String())
	lb.lineBuffer.Reset()

	// Line separartor may be cut by sending party
	lb.process([]byte("helloworldSEPAR"))
	lb.process([]byte("ATORhowayouSEPARATO"))
	lb.process([]byte("Rgoodandyou"))
	line = <-outputChan
	assert.Equal(t, "helloworld", string(line.content))
	line = <-outputChan
	assert.Equal(t, "howayou", string(line.content))
	assert.Equal(t, "goodandyou", lb.lineBuffer.String())
	lb.lineBuffer.Reset()

	// empty lines should be sent
	lb.process([]byte("SEPARATOR"))
	line = <-outputChan
	assert.Equal(t, "", string(line.content))
	assert.Equal(t, "", lb.lineBuffer.String())

	// empty message should not change anything
	lb.process([]byte(""))
	assert.Equal(t, "", lb.lineBuffer.String())
}

func TestLineBreakIncomingDataWithSingleByteCustomSequence(t *testing.T) {
	outputFn, outputChan := lineBreakerChans()
	lb := NewLineBreaker(outputFn, NewBytesSequenceMatcher([]byte("&"), 1), contentLenLimit)
	var line *DecodedInput

	// one line in one raw should be sent
	lb.process([]byte("helloworld&"))
	line = <-outputChan
	assert.Equal(t, "helloworld", string(line.content))
	assert.Equal(t, "", lb.lineBuffer.String())

	// multiple blank lines
	n := 10
	lb.process([]byte(strings.Repeat("&", n)))
	for i := 0; i < n; i++ {
		line = <-outputChan
		assert.Equal(t, "", string(line.content))
	}
	assert.Equal(t, "", lb.lineBuffer.String())
	lb.lineBuffer.Reset()

	// Mix empty & non-empty lines
	lb.process([]byte("helloworld&&"))
	lb.process([]byte("&howayou&"))
	line = <-outputChan
	assert.Equal(t, "helloworld", string(line.content))
	line = <-outputChan
	assert.Equal(t, "", string(line.content))
	line = <-outputChan
	assert.Equal(t, "", string(line.content))
	line = <-outputChan
	assert.Equal(t, "howayou", string(line.content))
	assert.Equal(t, "", lb.lineBuffer.String())
	lb.lineBuffer.Reset()

	// empty message should not change anything
	lb.process([]byte(""))
	assert.Equal(t, "", lb.lineBuffer.String())
}

func TestLinBreakerInputNotDockerHeader(t *testing.T) {
	outputFn, outputChan := lineBreakerChans()
	lb := NewLineBreaker(outputFn, &NewLineMatcher{}, 100)

	input := []byte("hello")
	input = append(input, []byte{1, 0, 0, 0, 0, 10, 0, 0}...) // docker header
	input = append(input, []byte("2018-06-14T18:27:03.246999277Z app logs\n")...)
	lb.process(input)

	var output *DecodedInput
	output = <-outputChan
	expected1 := append([]byte("hello"), []byte{1, 0, 0, 0, 0}...)
	assert.Equal(t, expected1, output.content)
	assert.Equal(t, len(expected1)+1, output.rawDataLen)

	output = <-outputChan
	expected2 := append([]byte{0, 0}, []byte("2018-06-14T18:27:03.246999277Z app logs")...)
	assert.Equal(t, expected2, output.content)
	assert.Equal(t, len(expected2)+1, output.rawDataLen)
}
