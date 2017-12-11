#include "datadog_agent.h"

// Functions
PyObject* GetVersion(PyObject *self, PyObject *args);
PyObject* Headers(PyObject *self, PyObject *args);
PyObject* GetHostname(PyObject *self, PyObject *args);
PyObject* LogMessage(char *message, int logLevel);
PyObject* GetConfig(char *key);
PyObject* GetSubprocessOutput(char **args, int argc, int raise);

// Exceptions
PyObject* SubprocessOutputEmptyError;

static PyObject *get_config(PyObject *self, PyObject *args) {
    char *key;

    PyGILState_STATE gstate;
    gstate = PyGILState_Ensure();

    if (!PyArg_ParseTuple(args, "s", &key)) {
      PyGILState_Release(gstate);
      return NULL;
    }

    PyGILState_Release(gstate);
    return GetConfig(key);
}

static PyObject *log_message(PyObject *self, PyObject *args) {
    char *message;
    int  log_level;

    PyGILState_STATE gstate;
    gstate = PyGILState_Ensure();

    // datadog_agent.log(message, log_level)
    if (!PyArg_ParseTuple(args, "si", &message, &log_level)) {
      PyGILState_Release(gstate);
      return NULL;
    }

    PyGILState_Release(gstate);
    return LogMessage(message, log_level);
}

static PyObject *get_subprocess_output(PyObject *self, PyObject *args) {
    PyObject *cmd_args, *cmd_raise_on_empty;
    int raise = 1, i=0;
    int subprocess_args_sz;
    char ** subprocess_args, * subprocess_arg;
    PyObject *py_result;

    PyGILState_STATE gstate = PyGILState_Ensure();

    cmd_raise_on_empty = NULL;
    if (!PyArg_ParseTuple(args, "O|O:get_subprocess_output", &cmd_args, &cmd_raise_on_empty)) {
        PyGILState_Release(gstate);
        return NULL;
    }

    if (!PyList_Check(cmd_args)) {
        PyErr_SetString(PyExc_TypeError, "command args not a list");
        PyGILState_Release(gstate);
        return NULL;
    }

    if (cmd_raise_on_empty != NULL && !PyBool_Check(cmd_raise_on_empty)) {
        PyErr_SetString(PyExc_TypeError, "bad raise_on_empty_argument - should be bool");
        PyGILState_Release(gstate);
        return NULL;
    }

    if (cmd_raise_on_empty != NULL) {
        raise = (int)(cmd_raise_on_empty == Py_True);
    }

    subprocess_args_sz = PyList_Size(cmd_args);
    if(!(subprocess_args = (char **)malloc(sizeof(char *)*subprocess_args_sz))) {
        PyErr_SetString(PyExc_MemoryError, "unable to allocate memory, bailing out");
        PyGILState_Release(gstate);
        return NULL;
    }

    for (i = 0; i < subprocess_args_sz; i++) {
        subprocess_arg = PyString_AsString(PyList_GetItem(cmd_args, i));
        if (subprocess_arg == NULL) {
            PyErr_SetString(PyExc_Exception, "unable to parse arguments to cgo/go-land");
            free(subprocess_args);
            PyGILState_Release(gstate);
            return NULL;
        }
        subprocess_args[i] = subprocess_arg;
    }

    PyGILState_Release(gstate);
    py_result = GetSubprocessOutput(subprocess_args, subprocess_args_sz, raise);
    free(subprocess_args);

    return py_result;
}

static PyMethodDef datadogAgentMethods[] = {
  {"get_version", GetVersion, METH_VARARGS, "Get the Agent version."},
  {"get_config", get_config, METH_VARARGS, "Get value from the agent configuration."},
  {"headers", Headers, METH_VARARGS | METH_KEYWORDS, "Get basic HTTP headers with the right UserAgent."},
  {"get_hostname", GetHostname, METH_VARARGS, "Get the agent hostname."},
  {"log", log_message, METH_VARARGS, "Log a message through the agent logger."},
  {NULL, NULL}
};

/*
 * Util package emulate the features within 'util' from agent5. It is
 * deprecated in favor of 'datadog_agent' package.
 */
static PyMethodDef utilMethods[] = {
  {"headers", (PyCFunction)Headers, METH_VARARGS | METH_KEYWORDS, "Get basic HTTP headers with the right UserAgent."},
  {NULL, NULL}
};

/*
 * _Util package is a private module for utility bindings
 */
static PyMethodDef _utilMethods[] = {
  {"get_subprocess_output", (PyCFunction)get_subprocess_output,
      METH_VARARGS, "Run subprocess and return its output. "
                    "This is a private method and should not be called directly. "
                    "Please use the utils.subprocess_output.get_subprocess_output wrapper."},
  {NULL, NULL}
};

void initdatadogagent()
{
  PyGILState_STATE gstate;
  gstate = PyGILState_Ensure();

  PyObject *da = Py_InitModule("datadog_agent", datadogAgentMethods);
  PyObject *util = Py_InitModule("util", utilMethods);
  PyObject *_util = Py_InitModule("_util", _utilMethods);

  SubprocessOutputEmptyError = PyErr_NewException("_util.SubprocessOutputEmptyError", NULL, NULL);
  Py_INCREF(SubprocessOutputEmptyError);
  PyModule_AddObject(_util, "SubprocessOutputEmptyError", SubprocessOutputEmptyError);

  PyGILState_Release(gstate);
}
