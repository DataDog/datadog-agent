#include "datadog_agent.h"

PyObject* GetVersion();
PyObject* Headers();

static PyMethodDef datadogAgentMethods[] = {
  {"get_version", (PyCFunction)GetVersion, METH_VARARGS, "Get the Agent version."},
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
