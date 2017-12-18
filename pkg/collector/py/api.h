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
int _PyDict_Check(PyObject*);
int _PyInt_Check(PyObject*);
int _PyString_Check(PyObject*);
PyObject* PySequence_Fast_Get_Item(PyObject*, Py_ssize_t);
Py_ssize_t PySequence_Fast_Get_Size(PyObject*);

#endif //_PY_API_H
