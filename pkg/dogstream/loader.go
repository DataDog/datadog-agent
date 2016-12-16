package dogstream

import (
	"errors"

	log "github.com/cihub/seelog"
	python "github.com/sbinet/go-python"
)

// Load searches and imports a Python module given the parser name
func Load(parserName string) (Parser, error) {
	// Lock the GIL and release it at the end of the run
	_gstate := python.PyGILState_Ensure()
	defer func() {
		python.PyGILState_Release(_gstate)
	}()

	// import python module containing the parser
	parserModule := python.PyImport_ImportModuleNoBlock(parserName)
	if parserModule == nil || !python.PyModule_Check(parserModule) {
		// we don't expect a traceback here so we use the error msg in `pvalue`
		_, pvalue, _ := python.PyErr_Fetch()
		msg := python.PyString_AsString(pvalue)
		log.Warn(msg)
		return nil, errors.New(msg)
	}

	// search for a `parse` function
	// TODO: this can be way more sophisticated, searching
	// for any callable with any name, etc...
	dir := parserModule.PyObject_Dir()
	var callable *python.PyObject
	for i := 0; i < python.PyList_GET_SIZE(dir); i++ {
		symbolName := python.PyString_AsString(python.PyList_GET_ITEM(dir, i))
		obj := parserModule.GetAttrString(symbolName)

		if symbolName == "parse" && obj.HasAttrString("__call__") == 1 {
			callable = obj
			break
		}
	}

	parserInstance := &PythonParse{
		callable: callable,
	}

	return parserInstance, nil
}
