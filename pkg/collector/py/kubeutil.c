// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

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

  PyObject *ku = Py_InitModule("kubeutil", kubeutilMethods);

  PyGILState_Release(gstate);
}
