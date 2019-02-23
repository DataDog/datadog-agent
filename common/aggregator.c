// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2019 Datadog, Inc.
#include "aggregator.h"

#include <assert.h>
#include <sixstrings.h>

#define MODULE_NAME "aggregator"

// these must be set by the Agent
static cb_submit_metric_t cb_submit_metric = NULL;

// forward declarations
static PyObject *submit_metric(PyObject *self, PyObject *args);
static PyObject *submit_service_check(PyObject *self, PyObject *args);
static PyObject *submit_event(PyObject *self, PyObject *args);
void add_constants(PyObject *);

// module object storage (Python2)
static PyObject *module;

static PyMethodDef methods[] = {
    { "submit_metric", (PyCFunction)submit_metric, METH_VARARGS, "Submit metrics to the aggregator." },
    { "submit_service_check", (PyCFunction)submit_service_check, METH_VARARGS,
      "Submit service checks to the aggregator." },
    { "submit_event", (PyCFunction)submit_event, METH_VARARGS, "Submit events to the aggregator." },
    { NULL, NULL } // guards
};

#ifdef DATADOG_AGENT_THREE
static struct PyModuleDef module_def = { PyModuleDef_HEAD_INIT, MODULE_NAME, NULL, -1, methods };

PyMODINIT_FUNC PyInit_aggregator(void) {
    PyObject *m = PyModule_Create(&module_def);
    add_constants(m);
    return m;
}
#endif

#ifdef DATADOG_AGENT_TWO
void Py2_init_aggregator() {
    module = Py_InitModule(MODULE_NAME, methods);
    add_constants(module);
}
#endif

void add_constants(PyObject *m) {
    PyModule_AddIntConstant(m, "GAUGE", DATADOG_AGENT_SIX_GAUGE);
    PyModule_AddIntConstant(m, "COUNT", DATADOG_AGENT_SIX_COUNT);
    PyModule_AddIntConstant(m, "MONOTONIC_COUNT", DATADOG_AGENT_SIX_MONOTONIC_COUNT);
    PyModule_AddIntConstant(m, "COUNTER", DATADOG_AGENT_SIX_COUNTER);
    PyModule_AddIntConstant(m, "HISTOGRAM", DATADOG_AGENT_SIX_HISTOGRAM);
    PyModule_AddIntConstant(m, "HISTORATE", DATADOG_AGENT_SIX_HISTORATE);
}

void _set_submit_metric_cb(cb_submit_metric_t cb) {
    cb_submit_metric = cb;
}

static PyObject *submit_metric(PyObject *self, PyObject *args) {
    // callback must be set
    assert(cb_submit_metric != NULL);

    PyObject *check = NULL;
    PyObject *py_tags = NULL;
    PyObject *py_tags_list = NULL;
    char *err = NULL;
    char *name = NULL;
    char *hostname = NULL;
    char *check_id = NULL;
    char **tags = NULL;
    int mt;
    float value;

    PyGILState_STATE gstate;
    gstate = PyGILState_Ensure();

    // Python call: aggregator.submit_metric(self, check_id, aggregator.metric_type.GAUGE, name, value, tags, hostname)
    if (!PyArg_ParseTuple(args, "OsisfOs", &check, &check_id, &mt, &name, &value, &py_tags, &hostname)) {
        goto done;
    }

    // convert tags to an array of char*
    Py_ssize_t len = 0;
    if (py_tags != NULL && PySequence_Check(py_tags)) {
        len = PySequence_Length(py_tags);
        if (len) {
            py_tags_list = PySequence_Fast(py_tags, err);
            if (py_tags_list == NULL) {
                goto done;
            }

            tags = malloc(len * sizeof(char *));

            for (int i = 0; i < len; i++) {
                PyObject *item = PySequence_Fast_GET_ITEM(py_tags_list, i); // `item` is borrowed, no need to decref
                char *str = as_string(item);
                // skip if not a string
                if (str == NULL) {
                    continue;
                }
                tags[i] = str;
            }
        }
    }

    cb_submit_metric(check_id, mt, name, value, tags, len, hostname);

done:
    if (err != NULL) {
        free(err);
    }
    Py_XDECREF(check);
    Py_XDECREF(py_tags);
    Py_XDECREF(py_tags_list);
    PyGILState_Release(gstate);

    return Py_None;
}

static PyObject *submit_service_check(PyObject *self, PyObject *args) {
    /*FIXME*/
    return NULL;
}
static PyObject *submit_event(PyObject *self, PyObject *args) {
    /*FIXME*/
    return NULL;
}
