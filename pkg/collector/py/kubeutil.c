// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build cpython,kubelet

#include "kubeutil.h"

// Functions
PyObject* GetKubeletConnectionInfo();

static PyMethodDef kubeutilMethods[] = {
  {"get_connection_info", GetKubeletConnectionInfo, METH_NOARGS, "Get kubelet connection information."},
  {NULL, NULL, 0, NULL}  // guards
};

static struct PyModuleDef kubeutilDef = {
  PyModuleDef_HEAD_INIT,
  "kubeutil",        /* m_name */
  "kubeutil module", /* m_doc */
  -1,                /* m_size */
  kubeutilMethods,   /* m_methods */
};

PyMODINIT_FUNC PyInit_kubeutil()
{
  return PyModule_Create(&kubeutilDef);
}

void register_kubeutil_module()
{
  PyImport_AppendInittab("kubeutil", PyInit_kubeutil);
}
