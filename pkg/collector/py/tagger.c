// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build cpython

#include "tagger.h"

// Functions
PyObject* GetTags(char *id, int highCard);
PyObject* Tag(char *id, TaggerCardinality card);

// _must_ be in the same order as the TaggerCardinality enum
char* TaggerCardinalityNames[] = {
  "LOW",
  "ORCHESTRATOR",
  "HIGH"
};

static PyObject *tag(PyObject *self, PyObject *args) {
    char *entity;
    int  card;

    PyGILState_STATE gstate;
    gstate = PyGILState_Ensure();

    if (!PyArg_ParseTuple(args, "si", &entity, &card)) {
      PyGILState_Release(gstate);
      return NULL;
    }

    PyGILState_Release(gstate);
    return Tag(entity, card);
}

static PyObject *get_tags(PyObject *self, PyObject *args) {
    char *entity;
    int  high_card;

    PyGILState_STATE gstate;
    gstate = PyGILState_Ensure();

    if (!PyArg_ParseTuple(args, "si", &entity, &high_card)) {
      PyGILState_Release(gstate);
      return NULL;
    }

    PyGILState_Release(gstate);
    return GetTags(entity, high_card);
}

static PyMethodDef taggerMethods[] = {
  {"tag", tag, METH_VARARGS, "Get tags for an entity."},
  {"get_tags", get_tags, METH_VARARGS, "(Deprecated) Get tags for an entity."},
  {NULL, NULL}
};

void inittagger()
{
  PyGILState_STATE gstate;
  gstate = PyGILState_Ensure();

  PyObject *tagger = Py_InitModule("tagger", taggerMethods);

  int i;
  for (i=TC_FIRST; i<=TC_LAST; i++) {
    PyModule_AddIntConstant(tagger, TaggerCardinalityNames[i], i);
  }

  PyGILState_Release(gstate);
}
