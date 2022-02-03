package decoder

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const contentLenLimit = 100

func lineBreakerChans() (chan *Input, chan *DecodedInput) {
	return make(chan *Input, 10), make(chan *DecodedInput, 10)
}

func TestLineBreakActor(t *testing.T) {
	test := func(chunks [][]byte) func(*testing.T) {
		return func(t *testing.T) {
			inputChan, outputChan := lineBreakerChans()
			go func() {
				for _, chunk := range chunks {
					inputChan <- &Input{content: chunk}
				}
			}()
			lb := NewLineBreaker(inputChan, outputChan, &NewLineMatcher{}, contentLenLimit)
			lb.Start()
			require.Equal(t, "line1", string((<-outputChan).content))
			require.Equal(t, "line2", string((<-outputChan).content))
			require.Equal(t, "line3", string((<-outputChan).content))
			require.Equal(t, "line4", string((<-outputChan).content))

			close(inputChan)

			// once the input channel closes, the output channel closes as well
			_, ok := <-outputChan
			require.Equal(t, false, ok)
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
	inputChan, outputChan := lineBreakerChans()
	lb := NewLineBreaker(inputChan, outputChan, &NewLineMatcher{}, contentLenLimit)

	var line *DecodedInput

	// one line in one raw should be sent
	lb.breakIncomingData([]byte("helloworld\n"))
	line = <-outputChan
	assert.Equal(t, "helloworld", string(line.content))
	assert.Equal(t, len("helloworld\n"), line.rawDataLen)
	assert.Equal(t, "", lb.lineBuffer.String())

	// multiple lines in one raw should be sent
	lb.breakIncomingData([]byte("helloworld\nhowayou\ngoodandyou"))
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
	lb.breakIncomingData([]byte("helloworld\nthisisa"))
	line = <-outputChan
	l += line.rawDataLen
	assert.Equal(t, "helloworld", string(line.content))
	assert.Equal(t, "thisisa", lb.lineBuffer.String())
	lb.breakIncomingData([]byte("longinput\nindeed"))
	line = <-outputChan
	l += line.rawDataLen
	assert.Equal(t, "thisisalonginput", string(line.content))
	assert.Equal(t, "indeed", lb.lineBuffer.String())
	assert.Equal(t, len("helloworld\nthisisalonginput\n"), l)
	lb.lineBuffer.Reset()
	lb.rawDataLen = 0

	// one line in multiple rows should be sent
	lb.breakIncomingData([]byte("hello world"))
	lb.breakIncomingData([]byte("!\n"))
	line = <-outputChan
	assert.Equal(t, "hello world!", string(line.content))
	assert.Equal(t, len("hello world!\n"), line.rawDataLen)

	// excessively long line in one row should be sent by chunks
	lb.breakIncomingData([]byte(strings.Repeat("a", contentLenLimit+10) + "\n"))
	line = <-outputChan
	assert.Equal(t, contentLenLimit, len(line.content))
	assert.Equal(t, contentLenLimit, line.rawDataLen)
	line = <-outputChan
	assert.Equal(t, strings.Repeat("a", 10), string(line.content))
	assert.Equal(t, 11, line.rawDataLen)

	// excessively long line in multiple rows should be sent by chunks
	lb.breakIncomingData([]byte(strings.Repeat("a", contentLenLimit-5)))
	lb.breakIncomingData([]byte(strings.Repeat("a", 15) + "\n"))
	line = <-outputChan
	assert.Equal(t, contentLenLimit, len(line.content))
	assert.Equal(t, contentLenLimit, line.rawDataLen)
	line = <-outputChan
	assert.Equal(t, strings.Repeat("a", 10), string(line.content))
	assert.Equal(t, 11, line.rawDataLen)

	// empty lines should be sent
	lb.breakIncomingData([]byte("\n"))
	line = <-outputChan
	assert.Equal(t, "", string(line.content))
	assert.Equal(t, "", lb.lineBuffer.String())
	assert.Equal(t, 1, line.rawDataLen)

	// empty message should not change anything
	lb.breakIncomingData([]byte(""))
	assert.Equal(t, "", lb.lineBuffer.String())
	assert.Equal(t, 0, lb.rawDataLen)
}

func TestLineBreakIncomingDataWithCustomSequence(t *testing.T) {
	inputChan, outputChan := lineBreakerChans()
	lb := NewLineBreaker(inputChan, outputChan, NewBytesSequenceMatcher([]byte("SEPARATOR"), 1), contentLenLimit)

	var line *DecodedInput

	// one line in one raw should be sent
	lb.breakIncomingData([]byte("helloworldSEPARATOR"))
	line = <-outputChan
	assert.Equal(t, "helloworld", string(line.content))
	assert.Equal(t, "", lb.lineBuffer.String())

	// multiple lines in one raw should be sent
	lb.breakIncomingData([]byte("helloworldSEPARATORhowayouSEPARATORgoodandyou"))
	line = <-outputChan
	assert.Equal(t, "helloworld", string(line.content))
	line = <-outputChan
	assert.Equal(t, "howayou", string(line.content))
	assert.Equal(t, "goodandyou", lb.lineBuffer.String())
	lb.lineBuffer.Reset()

	// Line separartor may be cut by sending party
	lb.breakIncomingData([]byte("helloworldSEPAR"))
	lb.breakIncomingData([]byte("ATORhowayouSEPARATO"))
	lb.breakIncomingData([]byte("Rgoodandyou"))
	line = <-outputChan
	assert.Equal(t, "helloworld", string(line.content))
	line = <-outputChan
	assert.Equal(t, "howayou", string(line.content))
	assert.Equal(t, "goodandyou", lb.lineBuffer.String())
	lb.lineBuffer.Reset()

	// empty lines should be sent
	lb.breakIncomingData([]byte("SEPARATOR"))
	line = <-outputChan
	assert.Equal(t, "", string(line.content))
	assert.Equal(t, "", lb.lineBuffer.String())

	// empty message should not change anything
	lb.breakIncomingData([]byte(""))
	assert.Equal(t, "", lb.lineBuffer.String())
}

func TestLineBreakIncomingDataWithSingleByteCustomSequence(t *testing.T) {
	inputChan, outputChan := lineBreakerChans()
	lb := NewLineBreaker(inputChan, outputChan, NewBytesSequenceMatcher([]byte("&"), 1), contentLenLimit)
	var line *DecodedInput

	// one line in one raw should be sent
	lb.breakIncomingData([]byte("helloworld&"))
	line = <-outputChan
	assert.Equal(t, "helloworld", string(line.content))
	assert.Equal(t, "", lb.lineBuffer.String())

	// multiple blank lines
	n := 10
	lb.breakIncomingData([]byte(strings.Repeat("&", n)))
	for i := 0; i < n; i++ {
		line = <-outputChan
		assert.Equal(t, "", string(line.content))
	}
	assert.Equal(t, "", lb.lineBuffer.String())
	lb.lineBuffer.Reset()

	// Mix empty & non-empty lines
	lb.breakIncomingData([]byte("helloworld&&"))
	lb.breakIncomingData([]byte("&howayou&"))
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
	lb.breakIncomingData([]byte(""))
	assert.Equal(t, "", lb.lineBuffer.String())
}

func TestLinBreakerInputNotDockerHeader(t *testing.T) {
	inputChan, outputChan := lineBreakerChans()
	lb := NewLineBreaker(inputChan, outputChan, &NewLineMatcher{}, 100)
	lb.Start()
	defer close(inputChan)

	input := []byte("hello")
	input = append(input, []byte{1, 0, 0, 0, 0, 10, 0, 0}...) // docker header
	input = append(input, []byte("2018-06-14T18:27:03.246999277Z app logs\n")...)
	inputChan <- NewInput(input)

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
