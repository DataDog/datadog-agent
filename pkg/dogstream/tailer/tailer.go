package tailer

import (
	"io"
	"os"
	"unicode/utf8"

	log "github.com/cihub/seelog"
	"github.com/fsnotify/fsnotify"

	"github.com/DataDog/datadog-agent/pkg/dogstream"
)

// Tailer tails log files and forwards the lines to parsers
type Tailer struct {
	w            *watcher
	files        map[string]*os.File
	dispatchers  map[string]*dispatcher
	partialLines map[string]string
}

type dispatcher struct {
	parsers []dogstream.Parser
	lines   chan string
}

type watcher struct {
	*fsnotify.Watcher
	fileUpdates chan string
}

// NewTailer instantiates a new Tailer
func NewTailer() *Tailer {
	return &Tailer{
		w:            newWatcher(),
		files:        make(map[string]*os.File),
		dispatchers:  make(map[string]*dispatcher),
		partialLines: make(map[string]string),
	}
}

// AddFile adds a log file to the Tailer
func (t *Tailer) AddFile(filePath string, parsers []dogstream.Parser) error {
	_, ok := t.files[filePath]
	if !ok {
		newFile, err := t.openFile(filePath, true)
		if err != nil {
			return err
		}
		t.files[filePath] = newFile
		t.dispatchers[filePath] = newDispatcher(parsers)
		t.w.Add(filePath)
	}

	return nil
}

// Run starts the Tailer
func (t *Tailer) Run() {
	defer t.cleanUp()
	t.w.Run()
	for fileUpdated := range t.w.fileUpdates {
		file := t.files[fileUpdated]
		t.partialLines[fileUpdated] = t.read(file, t.partialLines[fileUpdated], t.dispatchers[fileUpdated].dispatchLine)
	}
}

// Stop stops the tailer and frees up all its resources
func (t *Tailer) Stop() {
	t.w.Stop()
}

func (t Tailer) openFile(filePath string, seekEnd bool) (*os.File, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	if seekEnd {
		file.Seek(0, 2)
	}
	return file, nil
}

func (t *Tailer) cleanUp() {
	for _, file := range t.files {
		file.Close()
	}

	for _, d := range t.dispatchers {
		d.Stop()
	}
}

func (t Tailer) read(file *os.File, previousPartial string, dispatchLine func(string)) string {
	partial := previousPartial

	buffer := make([]byte, 0, 4096)
	for {
		bytesRead, err := file.Read(buffer[:cap(buffer)])
		buffer = buffer[:bytesRead]

		if err != nil {
			if err != io.EOF {
				// EOF happens and is not an issue, any other error should be logged though
				log.Errorf("Error reading file: %s", err)
			}
			return partial
		}

		// Taken from google/mtail
		buffer = buffer[:bytesRead]

		for i, width := 0, 0; i < len(buffer) && i < bytesRead; i += width {
			var r rune
			r, width = utf8.DecodeRune(buffer[i:])
			switch {
			case r != '\n':
				partial += string(r)
			default:
				// dipatch line to parsers, blocks if not ready
				dispatchLine(partial)
				// reset partial string
				partial = ""
			}
		}
	}
}

func newDispatcher(parsers []dogstream.Parser) *dispatcher {
	d := &dispatcher{
		parsers: parsers,
		lines:   make(chan string),
	}

	go d.run()

	return d
}

func (d dispatcher) dispatchLine(line string) {
	d.lines <- line
}

func (d dispatcher) Stop() {
	close(d.lines)
}

func (d dispatcher) run() {
	for line := range d.lines {
		for _, parser := range d.parsers {
			parser.Parse("FIXME", line)
		}
	}
}

func newWatcher() *watcher {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		log.Infof("Can't instantiate fs watcher: %s", err)
	}
	return &watcher{
		w,
		make(chan string),
	}
}

func (w *watcher) Run() {
	// flush out the errors
	go func() {
		for {
			select {
			case err := <-w.Errors:
				if err != nil {
					log.Infof("error: %s", err)
				}
			}
		}
	}()

	go func() {
		for {
			select {
			case event := <-w.Events:
				if event.Op&fsnotify.Write == fsnotify.Write {
					w.fileUpdates <- event.Name
				}
			}
		}
	}()
}

func (w *watcher) Stop() {
	w.Close()
	close(w.fileUpdates)
}
