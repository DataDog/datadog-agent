package util

import (
	"os"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type FileWatcher struct {
	filePath string
}

func NewFileWatcher(filePath string) *FileWatcher {
	return &FileWatcher{filePath: filePath}
}

func (fw *FileWatcher) readFile() ([]byte, error) {
	content, err := os.ReadFile(fw.filePath)
	if err != nil {
		return nil, err
	}
	return content, nil
}

// Watch watches the target file for changes and returns a channel that will receive
// the file's content whenever it changes.
// The initial implementation used fsnotify, but this was losing update events when running
// e2e tests - this simpler implementation behaves as expected, even if it's less efficient.
// Since this is meant to be used only for testing and development, it's fine to keep this
// implementation.
func (fw *FileWatcher) Watch() (<-chan []byte, error) {
	updateChan := make(chan []byte)
	prevContent := []byte{}
	ticker := time.NewTicker(100 * time.Millisecond)
	go func() {
		defer close(updateChan)
		for range ticker.C {
			content, err := fw.readFile()
			if err != nil {
				log.Printf("Error reading file %s: %s", fw.filePath, err)
				return
			}
			if len(content) > 0 && string(content) != string(prevContent) {
				prevContent = content
				updateChan <- content
			}
		}
	}()

	return updateChan, nil
}
