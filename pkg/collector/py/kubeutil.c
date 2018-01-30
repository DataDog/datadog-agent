// +build cpython,kubelet

#include "kubeutil.h"

// Functions
PyObject* GetKubeletConnectionInfo();

static PyMethodDef kubeutilMethods[] = {
  {"get_connection_info", GetKubeletConnectionInfo, METH_NOARGS, "Get kubelet connection information."},
  {NULL, NULL}
};

void initkubeutil()
{
  PyGILState_STATE gstate;
  gstate = PyGILState_Ensure();

  PyObject *da = Py_InitModule("kubeutil", kubeutilMethods);

  PyGILState_Release(gstate);
}
