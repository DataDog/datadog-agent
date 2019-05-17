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

static PyMethodDef methods[] = {
    { "submit_metric", (PyCFunction)submit_metric, METH_VARARGS, "Submit metrics." },
    { "submit_service_check", (PyCFunction)submit_service_check, METH_VARARGS, "Submit service checks." },
    { "submit_event", (PyCFunction)submit_event, METH_VARARGS, "Submit events." },
    { NULL, NULL } // guards
};

static void add_constants(PyObject *m) {
    PyModule_AddIntConstant(m, "GAUGE", DATADOG_AGENT_SIX_GAUGE);
    PyModule_AddIntConstant(m, "RATE", DATADOG_AGENT_SIX_RATE);
    PyModule_AddIntConstant(m, "COUNT", DATADOG_AGENT_SIX_COUNT);
    PyModule_AddIntConstant(m, "MONOTONIC_COUNT", DATADOG_AGENT_SIX_MONOTONIC_COUNT);
    PyModule_AddIntConstant(m, "COUNTER", DATADOG_AGENT_SIX_COUNTER);
    PyModule_AddIntConstant(m, "HISTOGRAM", DATADOG_AGENT_SIX_HISTOGRAM);
    PyModule_AddIntConstant(m, "HISTORATE", DATADOG_AGENT_SIX_HISTORATE);
}

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

void _set_submit_metric_cb(cb_submit_metric_t cb) {
    cb_submit_metric = cb;
}

void _set_submit_service_check_cb(cb_submit_service_check_t cb) {
    cb_submit_service_check = cb;
}

void _set_submit_event_cb(cb_submit_event_t cb) {
    cb_submit_event = cb;
}

static char **py_tag_to_c(PyObject *py_tags) {
    char **tags = NULL;
    char *err = NULL;
    PyObject *py_tags_list = NULL; // new reference

    if (!PySequence_Check(py_tags)) {
        PyErr_SetString(PyExc_TypeError, "tags must be a sequence");
        return NULL;
    }

    int len = PySequence_Length(py_tags);
    if (len == 0) {
        if (!(tags = malloc(sizeof(*tags)))) {
            PyErr_SetString(PyExc_RuntimeError, "could not allocate memory for tags");
            return NULL;
        }
        tags[0] = NULL;
        return tags;
    }

    py_tags_list = PySequence_Fast(py_tags, err); // new reference
    if (py_tags_list == NULL) {
        goto done;
    }

    if (!(tags = malloc(sizeof(*tags) * (len+1)))) {
        PyErr_SetString(PyExc_RuntimeError, "could not allocate memory for tags");
        goto done;
    }
    int nb_valid_tag = 0;
    int i;
    for (i = 0; i < len; i++) {
        // `item` is borrowed, no need to decref
        PyObject *item = PySequence_Fast_GET_ITEM(py_tags_list, i);

        char *ctag = as_string(item);
        if (ctag == NULL) {
            continue;
        }
        tags[nb_valid_tag] = ctag;
        nb_valid_tag++;
    }
    tags[nb_valid_tag] = NULL;

done:
    free(err);
    Py_XDECREF(py_tags_list);
    return tags;
}

static void free_tags(char **tags) {
    int i;
    for (i = 0; tags[i] != NULL; i++) {
        free(tags[i]);
    }
    free(tags);
}

static PyObject *submit_metric(PyObject *self, PyObject *args) {
    if (cb_submit_metric == NULL) {
        Py_RETURN_NONE;
    }

    PyGILState_STATE gstate = PyGILState_Ensure();

    PyObject *check = NULL; // borrowed
    PyObject *py_tags = NULL; // borrowed
    char *name = NULL;
    char *hostname = NULL;
    char *check_id = NULL;
    char **tags = NULL;
    int mt;
    float value;

    // Python call: aggregator.submit_metric(self, check_id, aggregator.metric_type.GAUGE, name, value, tags, hostname)
    if (!PyArg_ParseTuple(args, "OsisfOs", &check, &check_id, &mt, &name, &value, &py_tags, &hostname)) {
        goto error;
    }

    if ((tags = py_tag_to_c(py_tags)) == NULL)
        goto error;

    cb_submit_metric(check_id, mt, name, value, tags, hostname);

    free_tags(tags);

    PyGILState_Release(gstate);
    Py_RETURN_NONE;

error:
    PyGILState_Release(gstate);
    return NULL;
}

static PyObject *submit_service_check(PyObject *self, PyObject *args) {
    if (cb_submit_service_check == NULL) {
        Py_RETURN_NONE;
    }

    // acquiring GIL to be able to raise exception
    PyGILState_STATE gstate = PyGILState_Ensure();

    PyObject *check = NULL; // borrowed
    PyObject *py_tags = NULL; // borrowed
    char *name = NULL;
    int status;
    char *hostname = NULL;
    char *message = NULL;
    char *check_id = NULL;
    char **tags = NULL;

    // aggregator.submit_service_check(self, check_id, name, status, tags, hostname, message)
    if (!PyArg_ParseTuple(args, "OssiOss", &check, &check_id, &name, &status, &py_tags, &hostname, &message)) {
        goto error;
    }

    if ((tags = py_tag_to_c(py_tags)) == NULL)
        goto error;

    cb_submit_service_check(check_id, name, status, tags, hostname, message);

    free_tags(tags);

    PyGILState_Release(gstate);
    Py_RETURN_NONE;

error:
    PyGILState_Release(gstate);
    return NULL;
}

static PyObject *submit_event(PyObject *self, PyObject *args) {
    if (cb_submit_event == NULL) {
        Py_RETURN_NONE;
    }

    PyGILState_STATE gstate = PyGILState_Ensure();

    PyObject *check = NULL; // borrowed
    PyObject *event_dict = NULL; // borrowed
    PyObject *py_tags = NULL; // borrowed
    char *check_id = NULL;
    event_t *ev = NULL;

    // aggregator.submit_event(self, check_id, event)
    if (!PyArg_ParseTuple(args, "OsO", &check, &check_id, &event_dict)) {
        PyGILState_Release(gstate);
        // returning NULL to raise error
        return NULL;
    }

    if (!PyDict_Check(event_dict)) {
        PyErr_SetString(PyExc_TypeError, "event must be a dict");
        PyGILState_Release(gstate);
        // returning NULL to raise error
        return NULL;
    }

    if (!(ev = (event_t *)malloc(sizeof(event_t)))) {
        PyErr_SetString(PyExc_RuntimeError, "could not allocate memory for event");
        PyGILState_Release(gstate);
        return NULL;
    }

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

    py_tags = PyDict_GetItemString(event_dict, "tags");
    ev->tags = py_tag_to_c(py_tags);
    if (ev->tags == NULL) {
        free(ev);
        PyGILState_Release(gstate);
        // we need to return NULL to raise the exception set by PyErr_SetString
        return NULL;
    }

    cb_submit_event(check_id, ev);

    free_tags(ev->tags);

    free(ev);
    PyGILState_Release(gstate);
    Py_RETURN_NONE;
}
