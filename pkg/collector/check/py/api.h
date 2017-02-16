#include <Python.h>

typedef enum {
  MT_FIRST = 0,
  GAUGE = MT_FIRST,
  RATE,
  COUNT,
  MONOTONIC_COUNT,
  HISTOGRAM,
  MT_LAST = HISTOGRAM
} MetricType;

void initaggregator();
PyObject* _none();
PyObject* PySequence_Fast_Get_Item(PyObject*, Py_ssize_t);
Py_ssize_t PySequence_Fast_Get_Size(PyObject*);
