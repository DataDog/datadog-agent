// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2019 Datadog, Inc.
#include "datadog_agent.h"
#include "cgo_free.h"

#include <stringutils.h>

// these must be set by the Agent
static cb_get_version_t cb_get_version = NULL;
static cb_get_config_t cb_get_config = NULL;
static cb_headers_t cb_headers = NULL;
static cb_get_hostname_t cb_get_hostname = NULL;
static cb_get_clustername_t cb_get_clustername = NULL;
static cb_log_t cb_log = NULL;
static cb_set_external_tags_t cb_set_external_tags = NULL;

// forward declarations
static PyObject *get_version(PyObject *self, PyObject *args);
static PyObject *get_config(PyObject *self, PyObject *args);
static PyObject *headers(PyObject *self, PyObject *args, PyObject *kwargs);
static PyObject *get_hostname(PyObject *self, PyObject *args);
static PyObject *get_clustername(PyObject *self, PyObject *args);
static PyObject *log_message(PyObject *self, PyObject *args);
static PyObject *set_external_tags(PyObject *self, PyObject *args);

static PyMethodDef methods[] = {
    { "get_version", get_version, METH_NOARGS, "Get Agent version." },
    { "get_config", get_config, METH_VARARGS, "Get an Agent config item." },
    { "headers", (PyCFunction)headers, METH_VARARGS | METH_KEYWORDS, "Get standard set of HTTP headers." },
    { "get_hostname", get_hostname, METH_NOARGS, "Get the hostname." },
    { "get_clustername", get_clustername, METH_NOARGS, "Get the cluster name." },
    { "log", log_message, METH_VARARGS, "Log a message through the agent logger." },
    { "set_external_tags", set_external_tags, METH_VARARGS, "Send external host tags." },
    { NULL, NULL } // guards
};

#ifdef DATADOG_AGENT_THREE
static struct PyModuleDef module_def = { PyModuleDef_HEAD_INIT, DATADOG_AGENT_MODULE_NAME, NULL, -1, methods };

PyMODINIT_FUNC PyInit_datadog_agent(void)
{
    return PyModule_Create(&module_def);
}
#endif

#ifdef DATADOG_AGENT_TWO
// in Python2 keep the object alive for the program lifetime
static PyObject *module;

void Py2_init_datadog_agent()
{
    module = Py_InitModule(DATADOG_AGENT_MODULE_NAME, methods);
}
#endif

void _set_get_version_cb(cb_get_version_t cb)
{
    cb_get_version = cb;
}

void _set_get_config_cb(cb_get_config_t cb)
{
    cb_get_config = cb;
}

void _set_headers_cb(cb_headers_t cb)
{
    cb_headers = cb;
}

void _set_get_hostname_cb(cb_get_hostname_t cb)
{
    cb_get_hostname = cb;
}

void _set_get_clustername_cb(cb_get_clustername_t cb)
{
    cb_get_clustername = cb;
}

void _set_log_cb(cb_log_t cb)
{
    cb_log = cb;
}

void _set_set_external_tags_cb(cb_set_external_tags_t cb)
{
    cb_set_external_tags = cb;
}

PyObject *get_version(PyObject *self, PyObject *args)
{
    if (cb_get_version == NULL) {
        Py_RETURN_NONE;
    }

    char *v;
    cb_get_version(&v);

    if (v != NULL) {
        PyObject *retval = PyStringFromCString(v);
        cgo_free(v);
        return retval;
    }
    Py_RETURN_NONE;
}

/**
 * Before Six the Agent used reflection to inspect the contents of a configuration
 * value and the CPython API to perform conversion to a Python equivalent. Such
 * a conversion wouldn't be possible in a Python-agnostic way so we use YAML to
 * pass the data from Go to Python. The configuration value is loaded in the Agent,
 * marshalled into YAML and passed as a `char*` to Six, where the string is
 * decoded back to Python and passed to the caller. YAML usage is transparent to
 * the caller, who would receive a Python object as returned from `yaml.safe_load`.
 * YAML is used instead of JSON since the `json.load` return unicode for
 * string, for python2, which would be a breaking change from the previous
 * version of the agent.
 */
PyObject *get_config(PyObject *self, PyObject *args)
{
    // callback must be set
    if (cb_get_config == NULL) {
        Py_RETURN_NONE;
    }

    char *key;
    if (!PyArg_ParseTuple(args, "s", &key)) {
        return NULL;
    }

    char *data = NULL;
    cb_get_config(key, &data);

    // new ref
    PyObject *value = from_yaml(data);
    cgo_free(data);
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
PyObject *headers(PyObject *self, PyObject *args, PyObject *kwargs)
{
    // callback must be set but be resilient for the Python caller
    if (cb_headers == NULL) {
        Py_RETURN_NONE;
    }

    char *data = NULL;
    cb_headers(&data);

    // new ref
    PyObject *headers_dict = from_yaml(data);
    cgo_free(data);
    if (headers_dict == NULL || !PyDict_Check(headers_dict)) {
        // if headers_dict is not a dict we don't need to old a ref to it
        Py_XDECREF(headers_dict);
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

// provide a non-static entry point for the `headers` method; headers is duplicated
// in the `util` module; allow it to be called directly

PyObject *_public_headers(PyObject *self, PyObject *args, PyObject *kwargs)
{
    return headers(self, args, kwargs);
}

PyObject *get_hostname(PyObject *self, PyObject *args)
{
    // callback must be set
    if (cb_get_hostname == NULL) {
        Py_RETURN_NONE;
    }

    char *v;
    cb_get_hostname(&v);

    if (v != NULL) {
        PyObject *retval = PyStringFromCString(v);
        cgo_free(v);
        return retval;
    }
    Py_RETURN_NONE;
}

PyObject *get_clustername(PyObject *self, PyObject *args)
{
    // callback must be set
    if (cb_get_clustername == NULL) {
        Py_RETURN_NONE;
    }

    char *v;
    cb_get_clustername(&v);

    if (v != NULL) {
        PyObject *retval = PyStringFromCString(v);
        cgo_free(v);
        return retval;
    }
    Py_RETURN_NONE;
}

static PyObject *log_message(PyObject *self, PyObject *args)
{
    // callback must be set
    if (cb_log == NULL) {
        Py_RETURN_NONE;
    }

    char *message;
    int log_level;

    // datadog_agent.log(message, log_level)
    if (!PyArg_ParseTuple(args, "si", &message, &log_level)) {
        return NULL;
    }

    cb_log(message, log_level);
    Py_RETURN_NONE;
}

// set_external_tags receive the following data:
// [('hostname', {'source_type': ['tag1', 'tag2']})]
static PyObject *set_external_tags(PyObject *self, PyObject *args)
{
    PyObject *input_list = NULL;
    PyGILState_STATE gstate = PyGILState_Ensure();

    // function expects only one positional arg containing a list
    if (!PyArg_ParseTuple(args, "O", &input_list)) {
        PyGILState_Release(gstate);
        return NULL;
    }

    // if not a list, set an error
    if (!PyList_Check(input_list)) {
        PyErr_SetString(PyExc_TypeError, "tags must be a list");
        PyGILState_Release(gstate);
        return NULL;
    }

    int error = 0;
    char *hostname = NULL;
    char *source_type = NULL;
    int input_len = PyList_Size(input_list);
    int i;
    for (i = 0; i < input_len; i++) {
        PyObject *tuple = PyList_GetItem(input_list, i);

        // list must contain only tuples in form ('hostname', {'source_type': ['tag1', 'tag2']},)
        if (!PyTuple_Check(tuple)) {
            PyErr_SetString(PyExc_TypeError, "external host tags list must contain only tuples");
            goto error;
        }

        // first elem is the hostname
        hostname = as_string(PyTuple_GetItem(tuple, 0));
        if (hostname == NULL) {
            PyErr_SetString(PyExc_TypeError, "hostname is not a valid string");
            goto error;
        }

        // second is a dictionary
        PyObject *dict = PyTuple_GetItem(tuple, 1);
        if (!PyDict_Check(dict)) {
            PyErr_SetString(PyExc_TypeError, "second elem of the host tags tuple must be a dict");
            goto error;
        }

        // dict contains only 1 key, if dict is empty don't do anything
        Py_ssize_t pos = 0;
        PyObject *key = NULL, *value = NULL;
        if (!PyDict_Next(dict, &pos, &key, &value)) {
            continue;
        }

        // key is the source type (e.g. 'vsphere') value is the list of tags
        source_type = as_string(key);
        if (source_type == NULL) {
            PyErr_SetString(PyExc_TypeError, "source_type is not a valid string");
            goto error;
        }

        if (!PyList_Check(value)) {
            PyErr_SetString(PyExc_TypeError, "dict value must be a list of tags");
            goto error;
        }

        // allocate an array of char* to store the tags we'll send to the Go function
        char **tags;
        int tags_len = PyList_Size(value);
        if (!(tags = (char **)malloc(sizeof(*tags) * tags_len + 1))) {
            PyErr_SetString(PyExc_MemoryError, "unable to allocate memory, bailing out");
            goto error;
        }
        tags[tags_len] = NULL;

        // copy the list of tags into an array of char*
        int j, actual_size = 0;
        for (j = 0; j < tags_len; j++) {
            PyObject *s = PyList_GetItem(value, j);
            if (s == NULL) {
                continue;
            }

            char *tag = as_string(s);
            // cleanup and return error
            if (tag == NULL) {
                int k;
                for (k = 0; k < actual_size; k++) {
                    free(tags[k]);
                }
                free(tags);
                // raise an exception
                PyErr_SetString(PyExc_MemoryError, "unable to allocate memory, bailing out");
                goto error;
            }
            tags[actual_size] = tag;
            actual_size++;
        }

        // finally, invoke the Go function
        cb_set_external_tags(hostname, source_type, tags);

        // cleanup
        for (j = 0; j < actual_size; j++) {
            free(tags[j]);
        }
        free(tags);
    }

done:
    if (hostname)
        free(hostname);
    if (source_type)
        free(source_type);
    PyGILState_Release(gstate);

    // we need to return NULL to raise the exception set by PyErr_SetString
    if (error)
        return NULL;
    Py_RETURN_NONE;

error:
    error = 1;
    goto done;
}
