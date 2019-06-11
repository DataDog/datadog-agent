// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog
// (https://www.datadoghq.com/).
// Copyright 2019 Datadog, Inc.

#include "cgo_free.h"
#include "stringutils.h"
#include <tagger.h>

// these must be set by the Agent
static cb_tags_t cb_tags = NULL;

int parseArgs(PyObject *args, char **id, int *cardinality)
{
    PyGILState_STATE gstate = PyGILState_Ensure();

    if (!PyArg_ParseTuple(args, "si", id, cardinality)) {
        PyGILState_Release(gstate);
        return 0;
    }
    PyGILState_Release(gstate);
    return 1;
}

/*! \fn PyObject *buildTagList(char **tags)
    \brief builds a python string (tag) list from a C-string array.
    \param tags A char** C-string array.
    \return a PyObject * string (tag) list.

    This function assumes the provided C-string array has been allocated with cgo
    and frees it with the cgo_free callback for safety. The parameter passed should
    not be used after calling this function. The list and string python references
    are created by this function, no futher considerations are necessary.
*/
PyObject *buildTagsList(char **tags)
{
    if (tags == NULL) {
        // Py_RETURN_NONE macro increases the refcount on Py_None
        Py_RETURN_NONE;
    }

    PyObject *res = PyList_New(0);
    int i;
    for (i = 0; tags[i]; i++) {
        PyObject *pyTag = PyStringFromCString(tags[i]);
        cgo_free(tags[i]);

        // PyList_Append does not steal references so DECREF necessary for pyTag
        PyList_Append(res, pyTag);
        // PyList_Append (unlike `PyList_SetItem`) increments the refcount on pyTag
        // so we decrease it once appended
        Py_XDECREF(pyTag);
    }
    cgo_free(tags);
    return res;
}

/*! \fn PyObject *tag(PyObject *self, PyObject *args)
    \brief builds a tag list as per the id and cardinality passed as method
    arguments.
    \param self A PyObject* pointer to the tagger module.
    \param args A PyObject* pointer to the tag method, typically expected to
    contain the id and the cardinality..
    \return a PyObject * pointer to the python tag list.

    The method will return a tag list as long as the cardinality provided is
    one of LOW, ORCHESTRATOR, OR HIGH. This function calls the cgo-bound cb_tags
    callback, please read more about the internals of the registered callback.
*/
PyObject *tag(PyObject *self, PyObject *args)
{
    if (cb_tags == NULL) {
        // Py_RETURN_NONE macro increases the refcount on Py_None
        Py_RETURN_NONE;
    }

    char *id;
    int cardinality;
    if (!parseArgs(args, &id, &cardinality)) {
        return NULL;
    }

    if (cardinality != DATADOG_AGENT_SIX_TAGGER_LOW &&
            cardinality != DATADOG_AGENT_SIX_TAGGER_ORCHESTRATOR &&
            cardinality != DATADOG_AGENT_SIX_TAGGER_HIGH) {
        PyGILState_STATE gstate = PyGILState_Ensure();

        // The refcount for the error type: PyExc_TypeError need not be incremented
        PyErr_SetString(PyExc_TypeError, "Invalid cardinality");
        PyGILState_Release(gstate);
        return NULL;
    }

    return buildTagsList(cb_tags(id, cardinality));
}

/*! \fn PyObject *get_tag(PyObject *self, PyObject *args)
    \brief builds a tag list as per the id and cardinality passed as method.
    \param self A PyObject* pointer to the tagger module.
    \param args A PyObject* pointer to the tag method, typically expected to
    contain the id and the cardinality.
    \return a PyObject * pointer to the python tag list.

    DESCRIPTION TODO
*/
PyObject *get_tags(PyObject *self, PyObject *args)
{
    if (cb_tags == NULL) {
        // Py_RETURN_NONE macro increases the refcount on Py_None
        Py_RETURN_NONE;
    }

    char *id;
    int highCard;
    if (!parseArgs(args, &id, &highCard)) {
        return NULL;
    }

    int cardinality;
    if (highCard > 0) {
        cardinality = DATADOG_AGENT_SIX_TAGGER_HIGH;
    } else {
        cardinality = DATADOG_AGENT_SIX_TAGGER_LOW;
    }

    return buildTagsList(cb_tags(id, cardinality));
}

void _set_tags_cb(cb_tags_t cb)
{
    cb_tags = cb;
}

static PyMethodDef methods[] = {
    { "tag", (PyCFunction)tag, METH_VARARGS, "Get tags for an entity." },
    { "get_tags", (PyCFunction)get_tags, METH_VARARGS, "(Deprecated) Get tags for an entity." },
    { NULL, NULL } // guards
};

/*! \fn void add_constants(PyObject *m)
    \brief Registers constants with the module passed in as parameter.
    \param m A PyObject *m pointer to the relevant module we wish to register the
    constants in.

    LOW, ORCHESTRATOR and HIGH constants are registered with the module passed in.
    No reference considerations are necessary.
*/
static void add_constants(PyObject *m)
{
    PyModule_AddIntConstant(m, "LOW", DATADOG_AGENT_SIX_TAGGER_LOW);
    PyModule_AddIntConstant(m, "ORCHESTRATOR", DATADOG_AGENT_SIX_TAGGER_ORCHESTRATOR);
    PyModule_AddIntConstant(m, "HIGH", DATADOG_AGENT_SIX_TAGGER_HIGH);
}

#ifdef DATADOG_AGENT_THREE
static struct PyModuleDef module_def = { PyModuleDef_HEAD_INIT, TAGGER_MODULE_NAME, NULL, -1, methods };

PyMODINIT_FUNC PyInit_tagger(void)
{
    PyObject *module = PyModule_Create(&module_def);
    add_constants(module);
    return module;
}
#elif defined(DATADOG_AGENT_TWO)
// in Python2 keep the object alive for the program lifetime
static PyObject *module;

void Py2_init_tagger()
{
    module = Py_InitModule(TAGGER_MODULE_NAME, methods);
    add_constants(module);
}
#endif
