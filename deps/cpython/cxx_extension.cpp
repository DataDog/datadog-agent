#define PY_SSIZE_T_CLEAN
#include <Python.h>

#include <stdexcept>
#include <string>

static PyObject *greet(PyObject *, PyObject *args) {
    const char *name;
    if (!PyArg_ParseTuple(args, "s", &name)) {
        return nullptr;
    }
    try {
        throw std::runtime_error(std::string("hello ") + name);
    } catch (const std::exception &e) {
        return PyUnicode_FromString(e.what());
    }
}

static PyMethodDef methods[] = {
    {"greet", greet, METH_VARARGS, nullptr},
    {nullptr, nullptr, 0, nullptr},
};

static PyModuleDef module = {
    PyModuleDef_HEAD_INIT,
    "cxx_extension",
    nullptr,
    -1,
    methods,
};

PyMODINIT_FUNC PyInit_cxx_extension(void) {
    return PyModule_Create(&module);
}
