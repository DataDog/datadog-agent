package ebpf

import (
	"bufio"
	"bytes"
	"io"
	"regexp"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
	"github.com/pkg/errors"
)

var (
	// ErrNotImplemented will be returned on non-linux environments like Windows and Mac OSX
	ErrNotImplemented = errors.New("BPF-based system probe not implemented on non-linux systems")

	// CIncludePattern is the regex for #include headers of C files
	CIncludePattern = `^\s*#\s*include\s+"(.*)"$`
)

// snakeToCapInitialCamel converts a snake case to Camel case with capital initial
func snakeToCapInitialCamel(s string) string {
	n := ""
	capNext := true
	for _, v := range s {
		if v >= 'A' && v <= 'Z' {
			n += string(v)
		}
		if v >= 'a' && v <= 'z' {
			if capNext {
				n += strings.ToUpper(string(v))
			} else {
				n += string(v)
			}
		}
		if v == '_' {
			capNext = true
		} else {
			capNext = false
		}
	}
	return n
}

// processHeaders processes the `#include` of embedded headers.
func processHeaders(bpfDir, fileName string) (*bytes.Buffer, error) {
	sourceReader, err := bytecode.GetReader(bpfDir, fileName)
	if err != nil {
		return nil, err
	}

	// Note that embedded headers including other embedded headers is not managed because
	// this would also require to properly handle inclusion guards.
	includeRegexp := regexp.MustCompile(CIncludePattern)
	source := new(bytes.Buffer)
	scanner := bufio.NewScanner(sourceReader)
	for scanner.Scan() {
		match := includeRegexp.FindSubmatch(scanner.Bytes())
		if len(match) == 2 {
			header, err := bytecode.GetReader(bpfDir, string(match[1]))
			if err == nil {
				if _, err := io.Copy(source, header); err != nil {
					return source, err
				}
				continue
			}
		}
		source.Write(scanner.Bytes())
		source.WriteByte('\n')
	}
	return source, nil
}
