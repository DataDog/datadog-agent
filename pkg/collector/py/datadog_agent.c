#include "datadog_agent.h"

PyObject* GetVersion(PyObject *self, PyObject *args);
PyObject* Headers(PyObject *self, PyObject *args);
PyObject* GetHostname(PyObject *self, PyObject *args);
PyObject* LogMessage(char *message, int logLevel);
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

static PyObject *log_message(PyObject *self, PyObject *args) {
    char *message;
    int  log_level;

    PyGILState_STATE gstate;
    gstate = PyGILState_Ensure();

    // datadog_agent.log(message, log_level)
    if (!PyArg_ParseTuple(args, "si", &message, &log_level)) {
      PyGILState_Release(gstate);
      Py_RETURN_NONE;
    }

    PyGILState_Release(gstate);
    return LogMessage(message, log_level);
}

static PyMethodDef datadogAgentMethods[] = {
  {"get_version", GetVersion, METH_VARARGS, "Get the Agent version."},
  {"get_config", get_config, METH_VARARGS, "Get value from the agent configuration."},
  {"headers", Headers, METH_VARARGS, "Get basic HTTP headers with the right UserAgent."},
  {"get_hostname", GetHostname, METH_VARARGS, "Get the agent hostname."},
  {"log", log_message, METH_VARARGS, "Log a message through the agent logger."},
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
