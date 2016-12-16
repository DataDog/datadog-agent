package tailer

import (
	"os"
	"path"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/dogstream"
)

func check(e error) {
	if e != nil {
		panic(e)
	}
}

type FakeParser struct {
	m     sync.Mutex
	lines []string
}

func newFakeParser() *FakeParser {
	return &FakeParser{
		lines: []string{},
	}
}

func (f *FakeParser) Parse(logFile, line string) error {
	f.m.Lock()
	f.lines = append(f.lines, line)
	f.m.Unlock()
	return nil
}

func testFilePath(filename string) string {
	_, currentFilePath, _, _ := runtime.Caller(1)
	return path.Join(path.Dir(currentFilePath), "test", filename)
}

func TestRead(t *testing.T) {
	testContent := []byte("hello\ngo\n")
	file, _ := os.Create(testFilePath("foo"))
	defer file.Close()
	file.Seek(0, 2)
	_, err := file.Write(testContent)
	check(err)

	tailer := NewTailer()
	readLines := []string{}
	dispatch := func(line string) {
		readLines = append(readLines, line)
	}
	tailerFile, _ := os.Open(testFilePath("foo"))
	defer tailerFile.Close()
	tailer.read(tailerFile, dispatch)

	assert.Len(t, readLines, 2)
}

func TestTailerOneFile(t *testing.T) {
	file, _ := os.Create(testFilePath("foo"))
	defer file.Close()
	file.Seek(0, 2)
	garbageContent := []byte("garbage\nignore\n")
	_, err := file.Write(garbageContent)
	check(err)

	tailer := NewTailer()
	parsers := []dogstream.Parser{
		newFakeParser(),
		newFakeParser(),
	}

	err = tailer.AddFile(testFilePath("foo"), parsers)
	check(err)
	go tailer.Run()
	defer tailer.Stop()

	tailedContent := []byte("hello\ngo\n")
	_, err = file.Write(tailedContent)
	check(err)

	tailedContent = []byte("more\ntext\n")
	_, err = file.Write(tailedContent)
	check(err)

	time.Sleep(time.Millisecond)

	for _, parser := range parsers {
		fakeParser := parser.(*FakeParser)
		if assert.Len(t, fakeParser.lines, 4) {
			assert.Equal(t, "hello", fakeParser.lines[0])
			assert.Equal(t, "go", fakeParser.lines[1])
			assert.Equal(t, "more", fakeParser.lines[2])
			assert.Equal(t, "text", fakeParser.lines[3])
		}
	}
}

func TestTailerMultipleFiles(t *testing.T) {
	fileFoo, _ := os.Create(testFilePath("foo"))
	defer fileFoo.Close()
	fileFoo.Seek(0, 2)

	fileBar, _ := os.Create(testFilePath("bar"))
	defer fileBar.Close()
	fileBar.Seek(0, 2)

	tailer := NewTailer()
	parserFoo := newFakeParser()
	parserCommon := newFakeParser()
	parserBar := newFakeParser()
	parsersFoo := []dogstream.Parser{
		parserCommon,
		parserFoo,
	}
	parsersBar := []dogstream.Parser{
		parserCommon,
		parserBar,
	}

	err := tailer.AddFile(testFilePath("foo"), parsersFoo)
	check(err)
	err = tailer.AddFile(testFilePath("bar"), parsersBar)
	check(err)
	go tailer.Run()
	defer tailer.Stop()

	tailedContent := []byte("hello\ngo\n")
	_, err = fileFoo.Write(tailedContent)
	check(err)

	tailedContent = []byte("more\ntext\n")
	_, err = fileFoo.Write(tailedContent)
	check(err)

	tailedContent = []byte("bar\ncontent\n")
	_, err = fileBar.Write(tailedContent)
	check(err)

	tailedContent = []byte("still\nbar here\n")
	_, err = fileBar.Write(tailedContent)
	check(err)

	time.Sleep(time.Millisecond)

	if assert.Len(t, parserFoo.lines, 4) {
		assert.Equal(t, "hello", parserFoo.lines[0])
		assert.Equal(t, "go", parserFoo.lines[1])
		assert.Equal(t, "more", parserFoo.lines[2])
		assert.Equal(t, "text", parserFoo.lines[3])
	}

	if assert.Len(t, parserBar.lines, 4) {
		assert.Equal(t, "bar", parserBar.lines[0])
		assert.Equal(t, "content", parserBar.lines[1])
		assert.Equal(t, "still", parserBar.lines[2])
		assert.Equal(t, "bar here", parserBar.lines[3])
	}

	if assert.Len(t, parserCommon.lines, 8) {
		strings := []string{"hello", "go", "more", "text", "bar", "content", "still", "bar here"}
		for _, str := range strings {
			assert.Contains(t, parserCommon.lines, str)
		}
	}
}
