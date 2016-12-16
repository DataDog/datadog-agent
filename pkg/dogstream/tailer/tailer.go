package tailer

import (
	"log"
	"os"
	"unicode/utf8"

	"github.com/fsnotify/fsnotify"

	"github.com/DataDog/datadog-agent/pkg/dogstream"
)

// Tailer tails log files and forwards the lines to parsers
type Tailer struct {
	w           *watcher
	files       map[string]*os.File
	dispatchers map[string]*dispatcher
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
		w:           newWatcher(),
		files:       make(map[string]*os.File),
		dispatchers: make(map[string]*dispatcher),
	}
}

// AddFile adds a log file to the Tailer
func (t *Tailer) AddFile(filePath string, parsers []dogstream.Parser) error {
	_, ok := t.files[filePath]
	if !ok {
		newFile, err := t.openFile(filePath)
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
		t.read(file, t.dispatchers[fileUpdated].dispatchLine)
	}
}

// Stop stops the tailer and frees up all its resources
func (t *Tailer) Stop() {
	t.w.Stop()
}

func (t Tailer) openFile(filePath string) (*os.File, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	file.Seek(0, 2)
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

func (t Tailer) read(file *os.File, dispatchLine func(string)) {
	// FIXME: reads at most 4096 bytes
	buffer := make([]byte, 4096)
	bytesRead, err := file.Read(buffer)

	if err != nil {
		log.Println("Error reading file:", err)
	}

	// Taken from google/mtail
	buffer = buffer[:bytesRead]
	var line string

	for i, width := 0, 0; i < len(buffer) && i < bytesRead; i += width {
		var r rune
		r, width = utf8.DecodeRune(buffer[i:])
		switch {
		case r != '\n':
			line += string(r)
		default:
			// dipatch line to parsers, blocks if not ready
			dispatchLine(line)
			// reset line
			line = ""
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
		log.Println("Can't instantiate fs watcher:", err)
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
					log.Println("error:", err)
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
