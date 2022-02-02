package decoder

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type MockLineParser struct {
	inputChan chan *DecodedInput
}

func NewMockLineParser() *MockLineParser {
	return &MockLineParser{
		inputChan: make(chan *DecodedInput, 10),
	}
}

func (p *MockLineParser) Handle(input *DecodedInput) {
	p.inputChan <- input
}

func (p *MockLineParser) Start() {

}

func (p *MockLineParser) Stop() {
	close(p.inputChan)
}

const contentLenLimit = 100

func TestLineBreakActor(t *testing.T) {
	test := func(chunks [][]byte) func(*testing.T) {
		return func(t *testing.T) {
			inputChan := make(chan *Input, 5)
			go func() {
				for _, chunk := range chunks {
					inputChan <- &Input{content: chunk}
				}
			}()
			p := NewMockLineParser()
			lb := NewLineBreaker(inputChan, &NewLineMatcher{}, p, contentLenLimit)
			lb.Start()
			require.Equal(t, "line1", string((<-p.inputChan).content))
			require.Equal(t, "line2", string((<-p.inputChan).content))
			require.Equal(t, "line3", string((<-p.inputChan).content))
			require.Equal(t, "line4", string((<-p.inputChan).content))
			lb.Stop()
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
	lp := NewMockLineParser()
	lb := NewLineBreaker(nil, &NewLineMatcher{}, lp, contentLenLimit)

	var line *DecodedInput

	// one line in one raw should be sent
	lb.breakIncomingData([]byte("helloworld\n"))
	line = <-lp.inputChan
	assert.Equal(t, "helloworld", string(line.content))
	assert.Equal(t, len("helloworld\n"), line.rawDataLen)
	assert.Equal(t, "", lb.lineBuffer.String())

	// multiple lines in one raw should be sent
	lb.breakIncomingData([]byte("helloworld\nhowayou\ngoodandyou"))
	l := 0
	line = <-lp.inputChan
	l += line.rawDataLen
	assert.Equal(t, "helloworld", string(line.content))
	line = <-lp.inputChan
	l += line.rawDataLen
	assert.Equal(t, "howayou", string(line.content))
	assert.Equal(t, "goodandyou", lb.lineBuffer.String())
	assert.Equal(t, len("helloworld\nhowayou\n"), l)
	lb.lineBuffer.Reset()
	lb.rawDataLen, l = 0, 0

	// multiple lines in multiple rows should be sent
	lb.breakIncomingData([]byte("helloworld\nthisisa"))
	line = <-lp.inputChan
	l += line.rawDataLen
	assert.Equal(t, "helloworld", string(line.content))
	assert.Equal(t, "thisisa", lb.lineBuffer.String())
	lb.breakIncomingData([]byte("longinput\nindeed"))
	line = <-lp.inputChan
	l += line.rawDataLen
	assert.Equal(t, "thisisalonginput", string(line.content))
	assert.Equal(t, "indeed", lb.lineBuffer.String())
	assert.Equal(t, len("helloworld\nthisisalonginput\n"), l)
	lb.lineBuffer.Reset()
	lb.rawDataLen = 0

	// one line in multiple rows should be sent
	lb.breakIncomingData([]byte("hello world"))
	lb.breakIncomingData([]byte("!\n"))
	line = <-lp.inputChan
	assert.Equal(t, "hello world!", string(line.content))
	assert.Equal(t, len("hello world!\n"), line.rawDataLen)

	// excessively long line in one row should be sent by chunks
	lb.breakIncomingData([]byte(strings.Repeat("a", contentLenLimit+10) + "\n"))
	line = <-lp.inputChan
	assert.Equal(t, contentLenLimit, len(line.content))
	assert.Equal(t, contentLenLimit, line.rawDataLen)
	line = <-lp.inputChan
	assert.Equal(t, strings.Repeat("a", 10), string(line.content))
	assert.Equal(t, 11, line.rawDataLen)

	// excessively long line in multiple rows should be sent by chunks
	lb.breakIncomingData([]byte(strings.Repeat("a", contentLenLimit-5)))
	lb.breakIncomingData([]byte(strings.Repeat("a", 15) + "\n"))
	line = <-lp.inputChan
	assert.Equal(t, contentLenLimit, len(line.content))
	assert.Equal(t, contentLenLimit, line.rawDataLen)
	line = <-lp.inputChan
	assert.Equal(t, strings.Repeat("a", 10), string(line.content))
	assert.Equal(t, 11, line.rawDataLen)

	// empty lines should be sent
	lb.breakIncomingData([]byte("\n"))
	line = <-lp.inputChan
	assert.Equal(t, "", string(line.content))
	assert.Equal(t, "", lb.lineBuffer.String())
	assert.Equal(t, 1, line.rawDataLen)

	// empty message should not change anything
	lb.breakIncomingData([]byte(""))
	assert.Equal(t, "", lb.lineBuffer.String())
	assert.Equal(t, 0, lb.rawDataLen)
}

func TestLineBreakIncomingDataWithCustomSequence(t *testing.T) {
	lp := NewMockLineParser()
	lb := NewLineBreaker(nil, NewBytesSequenceMatcher([]byte("SEPARATOR"), 1), lp, contentLenLimit)

	var line *DecodedInput

	// one line in one raw should be sent
	lb.breakIncomingData([]byte("helloworldSEPARATOR"))
	line = <-lp.inputChan
	assert.Equal(t, "helloworld", string(line.content))
	assert.Equal(t, "", lb.lineBuffer.String())

	// multiple lines in one raw should be sent
	lb.breakIncomingData([]byte("helloworldSEPARATORhowayouSEPARATORgoodandyou"))
	line = <-lp.inputChan
	assert.Equal(t, "helloworld", string(line.content))
	line = <-lp.inputChan
	assert.Equal(t, "howayou", string(line.content))
	assert.Equal(t, "goodandyou", lb.lineBuffer.String())
	lb.lineBuffer.Reset()

	// Line separartor may be cut by sending party
	lb.breakIncomingData([]byte("helloworldSEPAR"))
	lb.breakIncomingData([]byte("ATORhowayouSEPARATO"))
	lb.breakIncomingData([]byte("Rgoodandyou"))
	line = <-lp.inputChan
	assert.Equal(t, "helloworld", string(line.content))
	line = <-lp.inputChan
	assert.Equal(t, "howayou", string(line.content))
	assert.Equal(t, "goodandyou", lb.lineBuffer.String())
	lb.lineBuffer.Reset()

	// empty lines should be sent
	lb.breakIncomingData([]byte("SEPARATOR"))
	line = <-lp.inputChan
	assert.Equal(t, "", string(line.content))
	assert.Equal(t, "", lb.lineBuffer.String())

	// empty message should not change anything
	lb.breakIncomingData([]byte(""))
	assert.Equal(t, "", lb.lineBuffer.String())
}

func TestLineBreakIncomingDataWithSingleByteCustomSequence(t *testing.T) {
	lp := NewMockLineParser()
	lb := NewLineBreaker(nil, NewBytesSequenceMatcher([]byte("&"), 1), lp, contentLenLimit)
	var line *DecodedInput

	// one line in one raw should be sent
	lb.breakIncomingData([]byte("helloworld&"))
	line = <-lp.inputChan
	assert.Equal(t, "helloworld", string(line.content))
	assert.Equal(t, "", lb.lineBuffer.String())

	// multiple blank lines
	n := 10
	lb.breakIncomingData([]byte(strings.Repeat("&", n)))
	for i := 0; i < n; i++ {
		line = <-lp.inputChan
		assert.Equal(t, "", string(line.content))
	}
	assert.Equal(t, "", lb.lineBuffer.String())
	lb.lineBuffer.Reset()

	// Mix empty & non-empty lines
	lb.breakIncomingData([]byte("helloworld&&"))
	lb.breakIncomingData([]byte("&howayou&"))
	line = <-lp.inputChan
	assert.Equal(t, "helloworld", string(line.content))
	line = <-lp.inputChan
	assert.Equal(t, "", string(line.content))
	line = <-lp.inputChan
	assert.Equal(t, "", string(line.content))
	line = <-lp.inputChan
	assert.Equal(t, "howayou", string(line.content))
	assert.Equal(t, "", lb.lineBuffer.String())
	lb.lineBuffer.Reset()

	// empty message should not change anything
	lb.breakIncomingData([]byte(""))
	assert.Equal(t, "", lb.lineBuffer.String())
}

func TestLinBreakerInputNotDockerHeader(t *testing.T) {
	inputChan := make(chan *Input)
	lp := NewMockLineParser()
	lb := NewLineBreaker(inputChan, &NewLineMatcher{}, lp, 100)
	lb.Start()

	input := []byte("hello")
	input = append(input, []byte{1, 0, 0, 0, 0, 10, 0, 0}...) // docker header
	input = append(input, []byte("2018-06-14T18:27:03.246999277Z app logs\n")...)
	inputChan <- NewInput(input)

	var output *DecodedInput
	output = <-lp.inputChan
	expected1 := append([]byte("hello"), []byte{1, 0, 0, 0, 0}...)
	assert.Equal(t, expected1, output.content)
	assert.Equal(t, len(expected1)+1, output.rawDataLen)

	output = <-lp.inputChan
	expected2 := append([]byte{0, 0}, []byte("2018-06-14T18:27:03.246999277Z app logs")...)
	assert.Equal(t, expected2, output.content)
	assert.Equal(t, len(expected2)+1, output.rawDataLen)
	lb.Stop()
}
