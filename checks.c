#include "checks.h"

// global checks inventory
static PyObject *CHECKS;

void get_checks(char **checks) {
  PyObject *pName, *pModule, *pFunc, *pValue;

  Py_Initialize();

  pModule = PyImport_ImportModule("config");
  if (pModule != NULL) {
    pFunc = PyObject_GetAttrString(pModule, "load_check_directory");
    if (pFunc && PyCallable_Check(pFunc)) {
      pValue = PyObject_CallObject(pFunc, NULL);
      if (pValue != NULL) {
        CHECKS = PyDict_GetItemString(pValue, "initialized_checks");
        printf("Initialized checks: %ld\n", PyDict_Size(CHECKS));
        printf("Failed to init checks: %ld\n", PyDict_Size(PyDict_GetItemString(pValue, "init_failed_checks")));
        Py_DECREF(pValue);
      }
      Py_DECREF(pFunc);
    }
    Py_DECREF(pModule);
  }
  else {
    PyErr_Print();
    fprintf(stderr, "Failed to load\n");
  }

  Py_Finalize();
}

void run_check(char *name) {
  PyObject *key, *check, *instances;
  key = check = instances = NULL;

  key = PyString_FromString(name);
  if (!PyDict_Contains(CHECKS, key)) {
    fprintf(stderr, "Check %s not available\n", name);
    goto cleanup;
  }
  check = PyDict_GetItem(CHECKS, key);
  instances = PyList_New(1);
  PyList_Append(instances, PyString_FromString("instance1"));
  PyObject_CallMethod(check, "run", "[] z", instances, NULL);

  // what should we do with run results?

cleanup:
  if (key != NULL) Py_DECREF(key);
  if (check != NULL) Py_DECREF(check);
  if (instances != NULL) Py_DECREF(instances);
}
