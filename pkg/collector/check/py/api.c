#include "api.h"

PyObject* SubmitMetric(PyObject*, MetricType, char*, float, PyObject*);
PyObject* SubmitServiceCheck(PyObject*, char*, int, PyObject*, char*);

char* MetricTypeNames[] = {
  "GAUGE",
  "RATE",
  "COUNT",
  "MONOTONIC_COUNT",
  "HISTOGRAM"
};

static PyObject *submit_metric(PyObject *self, PyObject *args) {
    PyObject *check = NULL;
    int mt;
    char *name;
    float value;
    PyObject *tags = NULL;

    PyGILState_STATE gstate;
    gstate = PyGILState_Ensure();

    // aggregator.submit_metric(self, aggregator.metric_type.GAUGE, name, value, tags)
    if (!PyArg_ParseTuple(args, "OisfO", &check, &mt, &name, &value, &tags)) {
      PyGILState_Release(gstate);
      Py_RETURN_NONE;
    }

    PyGILState_Release(gstate);
    return SubmitMetric(check, mt, name, value, tags);
}

static PyObject *submit_service_check(PyObject *self, PyObject *args) {
    PyObject *check = NULL;
    char *name;
    int status;
    PyObject *tags = NULL;
    char *message = NULL;

    PyGILState_STATE gstate;
    gstate = PyGILState_Ensure();

    // aggregator.submit_service_check(self, name, status, tags, message)
    if (!PyArg_ParseTuple(args, "OsiOs", &check, &name, &status, &tags, &message)) {
      PyGILState_Release(gstate);
      Py_RETURN_NONE;
    }

    PyGILState_Release(gstate);
    return SubmitServiceCheck(check, name, status, tags, message);
}

static PyMethodDef AggMethods[] = {
  {"submit_metric", (PyCFunction)submit_metric, METH_VARARGS, "Submit metrics to the aggregator."},
  {"submit_service_check", (PyCFunction)submit_service_check, METH_VARARGS, "Submit service checks to the aggregator."},
  {NULL, NULL}  // guards
};

PyObject* _none() {
	Py_RETURN_NONE;
}

void initaggregator()
{
  PyGILState_STATE gstate;
  gstate = PyGILState_Ensure();

  PyObject *m = Py_InitModule("aggregator", AggMethods);

  int i;
  for (i=MT_FIRST; i<=MT_LAST; i++) {
    PyModule_AddIntConstant(m, MetricTypeNames[i], i);
  }

  PyGILState_Release(gstate);
}

PyObject* PySequence_Fast_Get_Item(PyObject *o, Py_ssize_t i)
{
  return PySequence_Fast_GET_ITEM(o, i);
}

Py_ssize_t PySequence_Fast_Get_Size(PyObject *o)
{
  return PySequence_Fast_GET_SIZE(o);
}
