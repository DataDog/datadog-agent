package util

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/stretchr/testify/assert"
)

func TestFileWatcherMultipleFiles(t *testing.T) {
	// create two temporary files
	f1, _ := os.CreateTemp("", "file-watcher-test-")
	f2, _ := os.CreateTemp("", "file-watcher-test-")
	defer f1.Close()
	defer f2.Close()
	defer os.Remove(f1.Name())
	defer os.Remove(f2.Name())

	// get the absolute path for both files
	fp1, _ := filepath.Abs(f1.Name())
	fp2, _ := filepath.Abs(f2.Name())

	// initialize file contents
	os.WriteFile(fp1, []byte("This is file 1"), fs.ModeAppend)
	os.WriteFile(fp2, []byte("This is file 2"), fs.ModeAppend)

	// initialize file watchers
	fw1 := NewFileWatcher(fp1)
	fw2 := NewFileWatcher(fp2)

	ch1, err := fw1.Watch()
	assert.NoError(t, err)
	ch2, err := fw2.Watch()
	assert.NoError(t, err)

	fc1 := <-ch1
	assert.Equal(t, "This is file 1", string(fc1))
	fc2 := <-ch2
	assert.Equal(t, "This is file 2", string(fc2))

	os.WriteFile(fp1, []byte("Updated file 1"), fs.ModeAppend)
	os.WriteFile(fp2, []byte("Updated file 2"), fs.ModeAppend)

	fc1 = <-ch1
	assert.Equal(t, "Updated file 1", string(fc1))
	fc2 = <-ch2
	assert.Equal(t, "Updated file 2", string(fc2))
}

func TestFileWatcherDeletedFile(t *testing.T) {
	timeout := time.After(1 * time.Second)
	done := make(chan bool)
	go func() {
		f, _ := os.CreateTemp("", "file-watcher-delete-test-")
		defer f.Close()
		defer os.Remove(f.Name())

		fp, _ := filepath.Abs(f.Name())
		os.WriteFile(fp, []byte("Initial"), fs.ModeAppend)

		info, err := os.Stat(f.Name())
		if err != nil {
			panic(err)
		}
		m := info.Mode()

		fw := NewFileWatcher(fp)
		ch, err := fw.Watch()
		assert.NoError(t, err)

		fc := <-ch
		assert.Equal(t, "Initial", string(fc))

		// delete file and check that we are still receiving updates
		os.Remove(f.Name())
		os.WriteFile(fp, []byte("Updated"), fs.ModeAppend)
		err = os.Chmod(fp, m)
		assert.NoError(t, err)

		info, err = os.Stat(f.Name())
		if err != nil {
			panic(err)
		}
		m = info.Mode()
		log.Println(m)

		fc, ok := <-ch
		assert.True(t, ok, "expected channel to be open")
		assert.Equal(t, "Updated", string(fc), "expected to receive new file contents on channel")
		done <- true
	}()

	select {
	case <-timeout:
		t.Fatal("Timeout exceeded")
	case <-done:
	}
}
