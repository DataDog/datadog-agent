#include "datadog_agent.h"

PyObject* GetVersion();
PyObject* Headers();
PyObject* GetConfig(char *key);

static PyObject *get_config(PyObject *self, PyObject *args) {
    char *key;

    PyGILState_STATE gstate;
    gstate = PyGILState_Ensure();

    if (!PyArg_ParseTuple(args, "s", &key)) {
      PyGILState_Release(gstate);
      Py_RETURN_NONE;
    }

    PyGILState_Release(gstate);
    return GetConfig(key);
}

static PyMethodDef datadogAgentMethods[] = {
  {"get_version", (PyCFunction)GetVersion, METH_VARARGS, "Get the Agent version."},
  {"get_config", get_config, METH_VARARGS, "Get value from the agent configuration."},
  {"headers", (PyCFunction)Headers, METH_VARARGS, "Get basic HTTP headers with the right UserAgent."},
  {NULL, NULL}
};

/*
 * Util package emulate the features within 'util' from agent5. It is
 * deprecated in favor of 'datadog_agent' package.
 */
static PyMethodDef utilMethods[] = {
  {"headers", (PyCFunction)Headers, METH_VARARGS, "Get basic HTTP headers with the right UserAgent."},
  {NULL, NULL}
};

void initdatadogagent()
{
  PyGILState_STATE gstate;
  gstate = PyGILState_Ensure();

  PyObject *da = Py_InitModule("datadog_agent", datadogAgentMethods);
  PyObject *util = Py_InitModule("util", utilMethods);

  PyGILState_Release(gstate);
}
