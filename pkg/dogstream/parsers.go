package dogstream

import (
	"fmt"

	python "github.com/sbinet/go-python"
)

// Parser is the parser interface
type Parser interface {
	Parse(logFile, line string) error
}

// PythonParse is for pure python parsers
type PythonParse struct {
	callable *python.PyObject
}

// Parse gets a line from the logs and does something with it
func (p *PythonParse) Parse(logfile, line string) error {
	// Lock the GIL and release it at the end of the run
	_gstate := python.PyGILState_Ensure()
	defer func() {
		python.PyGILState_Release(_gstate)
	}()

	// args
	args := python.PyTuple_New(2)
	kwargs := python.PyDict_New()
	python.PyTuple_SetItem(args, 0, python.PyString_FromString(logfile))
	python.PyTuple_SetItem(args, 1, python.PyString_FromString(logfile))

	result := p.callable.Call(args, kwargs)
	if result == nil {
		return fmt.Errorf("Error invoking the parser")
	}

	return nil
}
