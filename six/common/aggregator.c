// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2019 Datadog, Inc.
#include "aggregator.h"

#include <sixstrings.h>

// these must be set by the Agent
static cb_submit_metric_t cb_submit_metric = NULL;
static cb_submit_service_check_t cb_submit_service_check = NULL;
static cb_submit_event_t cb_submit_event = NULL;

// forward declarations
static PyObject *submit_metric(PyObject *self, PyObject *args);
static PyObject *submit_service_check(PyObject *self, PyObject *args);
static PyObject *submit_event(PyObject *self, PyObject *args);
void add_constants(PyObject *);

static PyMethodDef methods[] = {
    { "submit_metric", (PyCFunction)submit_metric, METH_VARARGS, "Submit metrics." },
    { "submit_service_check", (PyCFunction)submit_service_check, METH_VARARGS, "Submit service checks." },
    { "submit_event", (PyCFunction)submit_event, METH_VARARGS, "Submit events." },
    { NULL, NULL } // guards
};

#ifdef DATADOG_AGENT_THREE
static struct PyModuleDef module_def = { PyModuleDef_HEAD_INIT, AGGREGATOR_MODULE_NAME, NULL, -1, methods };

PyMODINIT_FUNC PyInit_aggregator(void) {
    PyObject *m = PyModule_Create(&module_def);
    add_constants(m);
    return m;
}
#endif

#ifdef DATADOG_AGENT_TWO
// module object storage
static PyObject *module;

void Py2_init_aggregator() {
    module = Py_InitModule(AGGREGATOR_MODULE_NAME, methods);
    add_constants(module);
}
#endif

void add_constants(PyObject *m) {
    PyModule_AddIntConstant(m, "GAUGE", DATADOG_AGENT_SIX_GAUGE);
    PyModule_AddIntConstant(m, "RATE", DATADOG_AGENT_SIX_RATE);
    PyModule_AddIntConstant(m, "COUNT", DATADOG_AGENT_SIX_COUNT);
    PyModule_AddIntConstant(m, "MONOTONIC_COUNT", DATADOG_AGENT_SIX_MONOTONIC_COUNT);
    PyModule_AddIntConstant(m, "COUNTER", DATADOG_AGENT_SIX_COUNTER);
    PyModule_AddIntConstant(m, "HISTOGRAM", DATADOG_AGENT_SIX_HISTOGRAM);
    PyModule_AddIntConstant(m, "HISTORATE", DATADOG_AGENT_SIX_HISTORATE);
}

void _set_submit_metric_cb(cb_submit_metric_t cb) {
    cb_submit_metric = cb;
}

void _set_submit_service_check_cb(cb_submit_service_check_t cb) {
    cb_submit_service_check = cb;
}

void _set_submit_event_cb(cb_submit_event_t cb) {
    cb_submit_event = cb;
}

static PyObject *submit_metric(PyObject *self, PyObject *args) {
    if (cb_submit_metric == NULL) {
        Py_RETURN_NONE;
    }

    PyObject *check = NULL; // borrowed
    PyObject *py_tags = NULL; // borrowed
    PyObject *py_tags_list = NULL; // new reference
    char *err = NULL;
    char *name = NULL;
    char *hostname = NULL;
    char *check_id = NULL;
    char **tags = NULL;
    int mt;
    float value;

    // Python call: aggregator.submit_metric(self, check_id, aggregator.metric_type.GAUGE, name, value, tags, hostname)
    if (!PyArg_ParseTuple(args, "OsisfOs", &check, &check_id, &mt, &name, &value, &py_tags, &hostname)) {
        goto done;
    }

    int len = PySequence_Length(py_tags);
    if (len) {
        py_tags_list = PySequence_Fast(py_tags, err); // new reference
        if (py_tags_list == NULL) {
            goto done;
        }

        tags = malloc(len * sizeof(char *));
        int i;
        for (i = 0; i < len; i++) {
            // `item` is borrowed, no need to decref
            PyObject *item = PySequence_Fast_GET_ITEM(py_tags_list, i);
            tags[i] = as_string(item);
        }
    }

    cb_submit_metric(check_id, mt, name, value, tags, len, hostname);

done:
    free(err);
    Py_XDECREF(py_tags_list);
    Py_RETURN_NONE;
}

static PyObject *submit_service_check(PyObject *self, PyObject *args) {
    if (cb_submit_service_check == NULL) {
        Py_RETURN_NONE;
    }

    PyObject *check = NULL; // borrowed
    PyObject *py_tags = NULL; // borrowed
    PyObject *py_tags_list = NULL; // new reference
    char *err = NULL;
    char *name = NULL;
    int status;
    char *hostname = NULL;
    char *message = NULL;
    char *check_id = NULL;
    char **tags = NULL;

    // aggregator.submit_service_check(self, check_id, name, status, tags, hostname, message)
    if (!PyArg_ParseTuple(args, "OssiOss", &check, &check_id, &name, &status, &py_tags, &hostname, &message)) {
        goto done;
    }

    int len = PySequence_Length(py_tags);
    if (len) {
        py_tags_list = PySequence_Fast(py_tags, err); // new reference
        if (py_tags_list == NULL) {
            goto done;
        }

        tags = malloc(len * sizeof(char *));
        int i;
        for (i = 0; i < len; i++) {
            // `item` is borrowed, no need to decref
            PyObject *item = PySequence_Fast_GET_ITEM(py_tags_list, i);
            tags[i] = as_string(item);
        }
    }

    cb_submit_service_check(check_id, name, status, tags, len, hostname, message);

done:
    free(err);
    Py_XDECREF(py_tags_list);
    Py_RETURN_NONE;
}

static PyObject *submit_event(PyObject *self, PyObject *args) {
    if (cb_submit_event == NULL) {
        Py_RETURN_NONE;
    }

    PyObject *check = NULL; // borrowed
    PyObject *event_dict = NULL; // borrowed
    PyObject *py_tags = NULL; // borrowed
    PyObject *py_tags_list = NULL; // new reference
    char *check_id = NULL;
    char *err = NULL;
    event_t *ev = NULL;

    // aggregator.submit_event(self, check_id, event)
    if (!PyArg_ParseTuple(args, "OsO", &check, &check_id, &event_dict)) {
        goto done;
    }

    if (!PyDict_Check(event_dict)) {
        goto done;
    }

    ev = (event_t *)malloc(sizeof(event_t));
    // notice: PyDict_GetItemString returns a borrowed ref
    ev->title = as_string(PyDict_GetItemString(event_dict, "msg_title"));
    ev->text = as_string(PyDict_GetItemString(event_dict, "msg_text"));
    ev->ts = PyLong_AsLong(PyDict_GetItemString(event_dict, "timestamp"));
    ev->priority = as_string(PyDict_GetItemString(event_dict, "priority"));
    ev->host = as_string(PyDict_GetItemString(event_dict, "host"));
    ev->alert_type = as_string(PyDict_GetItemString(event_dict, "alert_type"));
    ev->aggregation_key = as_string(PyDict_GetItemString(event_dict, "aggregation_key"));
    ev->source_type_name = as_string(PyDict_GetItemString(event_dict, "source_type_name"));
    ev->event_type = as_string(PyDict_GetItemString(event_dict, "event_type"));

    int len = 0;
    py_tags = PyDict_GetItemString(event_dict, "tags");
    if (py_tags != NULL && py_tags != Py_None) {
        len = PySequence_Length(py_tags);
    }

    if (len) {
        ev->tags_num = len;
        py_tags_list = PySequence_Fast(py_tags, err); // new reference
        if (py_tags_list == NULL) {
            goto done;
        }

        ev->tags = malloc(len * sizeof(char *));
        int i;
        for (i = 0; i < len; i++) {
            // `item` is borrowed, no need to decref
            PyObject *item = PySequence_Fast_GET_ITEM(py_tags_list, i);
            ev->tags[i] = as_string(item);
        }
    }

    cb_submit_event(check_id, ev);

done:
    free(err);
    Py_XDECREF(py_tags_list);
    Py_RETURN_NONE;
}
