// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build cpython

#ifndef _PY_API_H
#define _PY_API_H

#include <Python.h>

typedef enum {
  MT_FIRST = 0,
  GAUGE = MT_FIRST,
  RATE,
  COUNT,
  MONOTONIC_COUNT,
  COUNTER,
  HISTOGRAM,
  HISTORATE,
  MT_LAST = HISTORATE
} MetricType;

void initaggregator();
PyObject* _none();
int _is_none(PyObject*);
const char* _object_type(PyObject *o);
int _PyDict_Check(PyObject*);
int _PyInt_Check(PyObject*);
int _PyString_Check(PyObject*);
PyObject* _PyObject_Repr(PyObject*);
PyObject* PySequence_Fast_Get_Item(PyObject*, Py_ssize_t);
Py_ssize_t PySequence_Fast_Get_Size(PyObject*);

#endif //_PY_API_H
