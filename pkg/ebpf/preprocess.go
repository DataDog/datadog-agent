package ebpf

import (
	"bufio"
	"bytes"
	"io"
	"regexp"

	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
)

var (
	// CIncludePattern is the regex for #include headers of C files
	CIncludePattern = `^\s*#\s*include\s+"(.*)"$`
)

// PreprocessFile pre-processes the `#include` of embedded headers.
// It will only replace top-level includes for files that exist
// and does not evaluate the content of included files for #include directives.
func PreprocessFile(bpfDir, fileName string) (*bytes.Buffer, error) {
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
