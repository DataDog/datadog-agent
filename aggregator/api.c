#include "api.h"

PyObject* SubmitData(PyObject*, MetricType, char*, float, PyObject*);

char* MetricTypeNames[] = {
  "GAUGE",
  "RATE",
  "HISTOGRAM"
};

static PyObject *submit_data(PyObject *self, PyObject *args) {
    PyObject *check = NULL;
    int mt;
    char *name;
    float value;
    PyObject *tags = NULL;

    PyGILState_STATE gstate;
    gstate = PyGILState_Ensure();

    // aggregator.submit_data(self, aggregator.metric_type.GAUGE, name, value, tags)
    if (!PyArg_ParseTuple(args, "OisfO", &check, &mt, &name, &value, &tags)) {
      PyGILState_Release(gstate);
      Py_RETURN_NONE;
    }

    PyGILState_Release(gstate);
    return SubmitData(check, mt, name, value, tags);
}

static PyMethodDef AggMethods[] = {
  {"submit_data", (PyCFunction)submit_data, METH_VARARGS, "Submit metrics to the aggregator."},
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

  for (int i=MT_FIRST; i<=MT_LAST; i++) {
    PyModule_AddIntConstant(m, MetricTypeNames[i], i);
  }

  PyGILState_Release(gstate);
}

PyObject* PySequence_Fast_Get_Item(PyObject *o, Py_ssize_t i)
{
  return PySequence_Fast_GET_ITEM(o, i);
}
