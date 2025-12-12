package functiontools

import (
	"os"
	"path/filepath"
	"strings"
)

func listFiles(directory string, extension string) ([]string, error) {
	var results []string

	// Normalize extension: add the dot if not provided
	if !strings.HasPrefix(extension, ".") {
		extension = "." + extension
	}

	err := filepath.Walk(directory, func(path string, info os.FileInfo, err error) error {
		// If an error occurs while walking, propagate it
		if err != nil {
			return err
		}

		// Skip directories
		if info.IsDir() {
			return nil
		}

		// Check if the file ends with the desired extension
		if strings.HasSuffix(info.Name(), extension) {
			results = append(results, path)
		}

		return nil
	})

	return results, err
}
