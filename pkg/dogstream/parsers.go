package dogstream

import (
	"fmt"

	python "github.com/sbinet/go-python"
)

type DogstreamParser interface {
	Parse(logFile, line string) error
}

type PythonParse struct {
	callable *python.PyObject
}

func (p *PythonParse) Parse(logfile, line string) error {
	// args
	args := python.PyTuple_New(2)
	kwargs := python.PyDict_New()
	python.PyTuple_SetItem(args, 0, python.PyString_FromString(logfile))
	python.PyTuple_SetItem(args, 1, python.PyString_FromString(logfile))

	result := p.callable.Call(args, kwargs)
	if result != nil {
		return fmt.Errorf("Error invoking the parser")
	}

	return nil
}
