package functiontools

import (
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strings"
)

// fileContent represents file data to be sent to the callback URL.
type fileContent struct {
	Data        string
	Path        string
	ContentType string
}

func getFile(path string, pattern string) (fileContent, error) {
	if path == "" {
		return fileContent{}, fmt.Errorf("parameter 'file_path' is required")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return fileContent{}, err
	}

	contentType := http.DetectContentType(data)
	if contentType == "application/octet-stream" && len(data) == 0 {
		contentType = "text/plain"
	}

	content := string(data)
	if pattern != "" {
		compiled, err := regexp.Compile(pattern)
		if err != nil {
			return fileContent{}, fmt.Errorf("parameter 'regex' is not a valid regular expression: %w", err)
		}

		lines := strings.Split(content, "\n")
		matches := make([]string, 0, len(lines))
		for _, line := range lines {
			if compiled.MatchString(line) {
				matches = append(matches, line)
			}
		}
		content = strings.Join(matches, "\n")
	}

	return fileContent{
		Data:        content,
		Path:        path,
		ContentType: contentType,
	}, nil
}
