// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build cpython

#include "datadog_agent.h"

/*
    NOTE: some primitives like `PyArg_ParseTuple` are not available through the
    `go-python` library, so we can map Go functions directly within a `PyMethodDef`
    only if (for example) the function doesn't need to access function args.

    That's why functions like `log_message` and `get_config` exist.
*/

// Functions
PyObject* GetVersion(PyObject *self, PyObject *args);
PyObject* Headers(PyObject *self, PyObject *args);
PyObject* GetHostname(PyObject *self, PyObject *args);
PyObject* GetClusterName(PyObject *self, PyObject *args);
PyObject* LogMessage(char *message, int logLevel);
PyObject* GetConfig(char *key);
PyObject* GetSubprocessOutput(char **args, int argc, int raise);
PyObject* SetExternalTags(const char *hostname, const char *source_type, char **tags, int tags_s);

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

static PyObject *set_external_tags(PyObject *self, PyObject *args) {
    PyObject *input_list = NULL;
    PyGILState_STATE gstate = PyGILState_Ensure();

    // function expects only one positional arg containing a list
    if (!PyArg_ParseTuple(args, "O", &input_list)) {
        PyGILState_Release(gstate);
        return NULL;
    }

    // if not a list, set an error
    if (!PyList_Check(input_list)) {
        PyErr_SetString(PyExc_TypeError, "function arg must be a list");
        PyGILState_Release(gstate);
        return NULL;
    }

    // if the list is empty do nothing
    int input_len = PyList_Size(input_list);
    if (input_len == 0) {
        PyGILState_Release(gstate);
        Py_RETURN_NONE;
    }

    int i;
    for (i=0; i<input_len; i++) {
        PyObject *tuple = PyList_GetItem(input_list, i);

        // list must contain only tuples in form ('hostname', {'source_type': ['tag1', 'tag2']},)
        if (!PyTuple_Check(tuple)) {
            PyErr_SetString(PyExc_TypeError, "external host tags list must contain only tuples");
            PyGILState_Release(gstate);
            return NULL;
        }

        // first elem is the hostname
        const char *hostname = PyString_AsString(PyTuple_GetItem(tuple, 0));
        // second is a dictionary
        PyObject *dict = PyTuple_GetItem(tuple, 1);
        if (!PyDict_Check(dict)) {
            PyErr_SetString(PyExc_TypeError, "second elem of the host tags tuple must be a dict");
            PyGILState_Release(gstate);
            return NULL;
        }

        // dict contains only 1 key, if dict is empty don't do anything
        Py_ssize_t pos = 0;
        PyObject *key = NULL, *value = NULL;
        if (!PyDict_Next(dict, &pos, &key, &value)) {
            continue;
        }

        // key is the source type (e.g. 'vsphere') value is the list of tags
        const char *source_type = PyString_AsString(key);
        if (!PyList_Check(value)) {
            PyErr_SetString(PyExc_TypeError, "dict value must be a list of tags ");
            PyGILState_Release(gstate);
            return NULL;
        }

        // allocate an array of char* to store the tags we'll send to the Go function
        char **tags;
        int tags_len = PyList_Size(value);
        if(!(tags = (char **)malloc(sizeof(char *)*tags_len))) {
            PyErr_SetString(PyExc_MemoryError, "unable to allocate memory, bailing out");
            PyGILState_Release(gstate);
            return NULL;
        }

        // copy the list of tags into an array of char*
        int j, actual_size = 0;
        for (j=0; j<tags_len; j++) {
            PyObject *s = PyList_GetItem(value, j);
            if (s == NULL) {
                continue;
            }

            char *tag = PyString_AsString(s);
            if (tag == NULL) {
                continue;
            }

            int len = PyString_Size(s) + 1;
            tags[actual_size] = (char*)malloc(sizeof(char)*len);
            if (!tags[actual_size]) {
                // cleanup
                int k;
                for (k=0; k<actual_size; k++) {
                    free(tags[k]);
                }
                free(tags);
                // raise an exception
                PyErr_SetString(PyExc_MemoryError, "unable to allocate memory, bailing out");
                PyGILState_Release(gstate);
                return NULL;
            }
            strncpy(tags[actual_size], tag, len);
            actual_size++;
        }

        // finally, invoke the Go function
        SetExternalTags(hostname, source_type, tags, actual_size);

        // cleanup
        for (j=0; j<actual_size; j++) {
            free(tags[j]);
        }
        free(tags);
    }

    PyGILState_Release(gstate);
    Py_RETURN_NONE;
}

static PyMethodDef datadogAgentMethods[] = {
  {"get_version", GetVersion, METH_VARARGS, "Get the Agent version."},
  {"get_config", get_config, METH_VARARGS, "Get value from the agent configuration."},
  {"headers", Headers, METH_VARARGS | METH_KEYWORDS, "Get basic HTTP headers with the right UserAgent."},
  {"get_hostname", GetHostname, METH_VARARGS, "Get the agent hostname."},
  {"get_clustername", GetClusterName, METH_VARARGS, "Get the agent cluster name."},
  {"log", log_message, METH_VARARGS, "Log a message through the agent logger."},
  {"set_external_tags", set_external_tags, METH_VARARGS, "Send external host tags."},
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
                    "Please use the stackstate_checks.utils.subprocess_output.get_subprocess_output wrapper."},
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
