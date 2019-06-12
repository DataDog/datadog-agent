// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build cpython

#include "stackstate_api.h"

PyObject* SubmitComponent(PyObject*, char*, PyObject*, char*, char*, PyObject*);
PyObject* SubmitRelation(PyObject*, char*, PyObject*, char*, char*, char*, PyObject*);
PyObject* SubmitStartSnapshot(PyObject*, char*, PyObject*);
PyObject* SubmitStopSnapshot(PyObject*, char*, PyObject*);

static PyObject *submit_component(PyObject *self, PyObject *args) {
    PyObject *check = NULL;
    char *check_id;
    PyObject *instance_key = NULL;
    char *component_id;
    char *component_type;
    PyObject *data = NULL;

    PyGILState_STATE gstate;
    gstate = PyGILState_Ensure();

    if (!PyArg_ParseTuple(args, "OsOssO", &check, &check_id, &instance_key, &component_id, &component_type, &data)) {
      PyGILState_Release(gstate);
      return NULL;
    }

    PyGILState_Release(gstate);
    return SubmitComponent(check, check_id, instance_key, component_id, component_type, data);
}

static PyObject *submit_relation(PyObject *self, PyObject *args) {
    PyObject *check = NULL;
    char *check_id;
    PyObject *instance_key = NULL;
    char *source_id;
    char *target_id;
    char *relation_type;
    PyObject *data = NULL;

    PyGILState_STATE gstate;
    gstate = PyGILState_Ensure();

    if (!PyArg_ParseTuple(args, "OsOsssO", &check, &check_id, &instance_key, &source_id, &target_id, &relation_type, &data)) {
      PyGILState_Release(gstate);
      return NULL;
    }

    PyGILState_Release(gstate);
    return SubmitRelation(check, check_id, instance_key, source_id, target_id, relation_type, data);
}

static PyObject *submit_start_snapshot(PyObject *self, PyObject *args) {
    PyObject *check = NULL;
    char *check_id;
    PyObject *instance_key = NULL;

    PyGILState_STATE gstate;
    gstate = PyGILState_Ensure();

    if (!PyArg_ParseTuple(args, "OsO", &check, &check_id, &instance_key)) {
      PyGILState_Release(gstate);
      return NULL;
    }

    PyGILState_Release(gstate);
    return SubmitStartSnapshot(check, check_id, instance_key);
}

static PyObject *submit_stop_snapshot(PyObject *self, PyObject *args) {
    PyObject *check = NULL;
    char *check_id;
    PyObject *instance_key = NULL;

    PyGILState_STATE gstate;
    gstate = PyGILState_Ensure();

    if (!PyArg_ParseTuple(args, "OsO", &check, &check_id, &instance_key)) {
      PyGILState_Release(gstate);
      return NULL;
    }

    PyGILState_Release(gstate);
    return SubmitStopSnapshot(check, check_id, instance_key);
}

static PyMethodDef StackStateMethods[] = {
  {"submit_component", (PyCFunction)submit_component, METH_VARARGS, "Submit a component to the stackstate api."},
  {"submit_relation", (PyCFunction)submit_relation, METH_VARARGS, "Submit a relation to the stackstate api."},
  {"submit_start_snapshot", (PyCFunction)submit_start_snapshot, METH_VARARGS, "Submit a snapshot start to the stackstate api."},
  {"submit_stop_snapshot", (PyCFunction)submit_stop_snapshot, METH_VARARGS, "Submit a snapshot stop to the stackstate api."},
  {NULL, NULL}  // guards
};

void initstackstate()
{
  PyGILState_STATE gstate;
  gstate = PyGILState_Ensure();

  PyObject *m = Py_InitModule("stackstate", StackStateMethods);

  PyGILState_Release(gstate);
}
