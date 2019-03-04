// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2019 Datadog, Inc.
#include "datadog_agent.h"

#include <assert.h>
#include <sixstrings.h>

#define MODULE_NAME "datadog_agent"

// these must be set by the Agent
static cb_get_version_t cb_get_version = NULL;
static cb_get_config_t cb_get_config = NULL;
static cb_headers_t cb_headers = NULL;
static cb_get_hostname_t cb_get_hostname = NULL;
static cb_get_clustername_t cb_get_clustername = NULL;
static cb_log_t cb_log = NULL;

// forward declarations
static PyObject *get_version(PyObject *self, PyObject *args);
static PyObject *get_config(PyObject *self, PyObject *args);
static PyObject *headers(PyObject *self, PyObject *args, PyObject *kwargs);
static PyObject *get_hostname(PyObject *self, PyObject *args);
static PyObject *get_clustername(PyObject *self, PyObject *args);
static PyObject *log_message(PyObject *self, PyObject *args);

static PyMethodDef methods[] = {
    { "get_version", get_version, METH_NOARGS, "Get Agent version." },
    { "get_config", get_config, METH_VARARGS, "Get an Agent config item." },
    { "headers", (PyCFunction)headers, METH_VARARGS | METH_KEYWORDS, "Get standard set of HTTP headers." },
    { "get_hostname", get_hostname, METH_NOARGS, "Get the hostname." },
    { "get_clustername", get_clustername, METH_NOARGS, "Get the cluster name." },
    { "log", log_message, METH_VARARGS, "Log a message through the agent logger." },
    { NULL, NULL } // guards
};

#ifdef DATADOG_AGENT_THREE
static struct PyModuleDef module_def = { PyModuleDef_HEAD_INIT, MODULE_NAME, NULL, -1, methods };

PyMODINIT_FUNC PyInit_datadog_agent(void) {
    return PyModule_Create(&module_def);
}
#endif

#ifdef DATADOG_AGENT_TWO
// in Python2 keep the object alive for the program lifetime
static PyObject *module;

void Py2_init_datadog_agent() {
    module = Py_InitModule(MODULE_NAME, methods);
}
#endif

void _set_get_version_cb(cb_get_version_t cb) {
    cb_get_version = cb;
}

void _set_get_config_cb(cb_get_config_t cb) {
    cb_get_config = cb;
}

void _set_headers_cb(cb_headers_t cb) {
    cb_headers = cb;
}

void _set_get_hostname_cb(cb_get_hostname_t cb) {
    cb_get_hostname = cb;
}

void _set_get_clustername_cb(cb_get_clustername_t cb) {
    cb_get_clustername = cb;
}

void _set_log_cb(cb_log_t cb) {
    cb_log = cb;
}

PyObject *get_version(PyObject *self, PyObject *args) {
    // callback must be set
    assert(cb_get_version != NULL);

    char *v;
    cb_get_version(&v);

    if (v != NULL) {
        PyObject *retval = PyUnicode_FromString(v);
        free(v);
        return retval;
    }
    Py_RETURN_NONE;
}

/**
 * Before Six the Agent used reflection to inspect the contents of a configuration
 * value and the CPython API to perform conversion to a Python equivalent. Such
 * a conversion wouldn't be possible in a Python-agnostic way so we use JSON to
 * pass the data from Go to Python. The configuration value is loaded in the Agent,
 * marshalled into JSON and passed as a `char*` to Six, where the string is
 * decoded back to Python and passed to the caller. JSON usage is transparent to
 * the caller, who would receive a Python object as returned from `json.loads`.
 */
PyObject *get_config(PyObject *self, PyObject *args) {
    // callback must be set
    assert(cb_get_config != NULL);

    char *key;
    if (!PyArg_ParseTuple(args, "s", &key)) {
        Py_RETURN_NONE;
    }

    char *data;
    cb_get_config(key, &data);

    // new ref
    PyObject *value = from_json(data);
    if (value == NULL) {
        Py_RETURN_NONE;
    }
    return value;
}

/**
 * datadog_agent.headers() isn't used by any official integration provided by
 * Datdog but custom checks might still rely on that.
 * Currently the contents of the returned string are the same but defined in two
 * different places:
 *
 *  1. github.com/DataDog/integrations-core/datadog_checks_base/datadog_checks/base/utils/headers.py
 *  2. github.com/DataDog/datadog-agent/pkg/util/common.go
 */
PyObject *headers(PyObject *self, PyObject *args, PyObject *kwargs) {
    // callback must be set but be resilient for the Python caller
    if (cb_headers == NULL) {
        Py_RETURN_NONE;
    }

    char *data;
    cb_headers(&data);

    // new ref
    PyObject *headers_dict = from_json(data);
    if (headers_dict == NULL || !PyDict_Check(headers_dict)) {
        Py_RETURN_NONE;
    }

    // `args` contains `agentConfig` but we don't need it
    // `kwargs` might contain the `http_host` key, let's grab it
    if (kwargs != NULL) {
        char key[] = "http_host";
        // borrowed
        PyObject *pyHTTPHost = PyDict_GetItemString(kwargs, key);
        if (pyHTTPHost != NULL) {
            PyDict_SetItemString(headers_dict, "Host", pyHTTPHost);
        }
    }

    return headers_dict;
}

PyObject *get_hostname(PyObject *self, PyObject *args) {
    // callback must be set
    if (cb_get_hostname == NULL) {
        Py_RETURN_NONE;
    }

    char *v;
    cb_get_hostname(&v);

    if (v != NULL) {
        PyObject *retval = PyUnicode_FromString(v);
        free(v);
        return retval;
    }
    Py_RETURN_NONE;
}

PyObject *get_clustername(PyObject *self, PyObject *args) {
    // callback must be set
    if (cb_get_clustername == NULL) {
        Py_RETURN_NONE;
    }

    char *v;
    cb_get_clustername(&v);

    if (v != NULL) {
        PyObject *retval = PyUnicode_FromString(v);
        free(v);
        return retval;
    }
    Py_RETURN_NONE;
}

static PyObject *log_message(PyObject *self, PyObject *args) {
    // callback must be set
    if (cb_log == NULL) {
        Py_RETURN_NONE;
    }

    char *message;
    int log_level;

    // datadog_agent.log(message, log_level)
    if (!PyArg_ParseTuple(args, "si", &message, &log_level)) {
        Py_RETURN_NONE;
    }

    cb_log(message, log_level);
    Py_RETURN_NONE;
}
