// +build cpython

#include "telemetry_api.h"

PyObject* SubmitTopologyEvent(PyObject*, char*, PyObject*);

static PyObject *submit_topology_event(PyObject *self, PyObject *args) {
    PyObject *check = NULL;
    PyObject *event = NULL;
    char *check_id;

    PyGILState_STATE gstate;
    gstate = PyGILState_Ensure();

    // aggregator.submit_topology_event(self, check_id, event)
    if (!PyArg_ParseTuple(args, "OsO", &check, &check_id, &event)) {
      PyGILState_Release(gstate);
      return NULL;
    }

    PyGILState_Release(gstate);
    return SubmitTopologyEvent(check, check_id, event);
}

static PyMethodDef TelemetryMethods[] = {
  {"submit_topology_event", (PyCFunction)submit_topology_event, METH_VARARGS, "Submit topology events to the aggregator."},
  {NULL, NULL}  // guards
};

void inittelemetry()
{
  PyGILState_STATE gstate;
  gstate = PyGILState_Ensure();

  PyObject *m = Py_InitModule("telemetry", TelemetryMethods);

  PyGILState_Release(gstate);
}
