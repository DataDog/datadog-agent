// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build cpython

#include "containers.h"

// Functions
int IsContainerExcluded(char *name, char *image);

static PyObject *is_excluded(PyObject *self, PyObject *args) {
    char *name;
    char *image;

    PyGILState_STATE gstate;
    gstate = PyGILState_Ensure();

    // containers.is_excluded(name, image)
    if (!PyArg_ParseTuple(args, "ss", &name, &image)) {
      PyGILState_Release(gstate);
      return NULL;
    }

    PyGILState_Release(gstate);
    if (IsContainerExcluded(name, image) == 1) {
        Py_INCREF(Py_True);
        return Py_True;
    } else {
        Py_INCREF(Py_False);
        return Py_False;
    }
}

static PyMethodDef containersMethods[] = {
  {"is_excluded", is_excluded, METH_VARARGS, "Filter a container per name and image"},
  {NULL, NULL, 0, NULL}  // guards
};

static struct PyModuleDef containersDef = {
  PyModuleDef_HEAD_INIT,
  "containers",        /* m_name */
  "containers module", /* m_doc */
  -1,                  /* m_size */
  containersMethods,   /* m_methods */
};

PyMODINIT_FUNC PyInit_containers()
{
  return PyModule_Create(&containersDef);
}

void register_containers_module()
{
  PyImport_AppendInittab("containers", PyInit_containers);
}
