// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2019-present Datadog, Inc.
// stringutils.h includes Python.h which must come before system headers (CPython requirement)
#include "stringutils.h"

#include <stdlib.h>

#include "rtloader_mem.h"
#include "rtloader_types.h"


PyObject * jloads = NULL;
PyObject * jdumps = NULL;

/**
 * returns a C (NULL terminated UTF-8) string from a python string.
 *
 * \param object  A Python string to be converted to C-string.
 *
 * \return A standard C string (NULL terminated character pointer)
 *  The returned pointer is allocated from the heap and must be
 * deallocated (free()ed) by the caller
 */
char *as_string(PyObject *object)
{
    if (object == NULL) {
        return NULL;
    }

    char *retval = NULL;

    PyObject *temp_bytes = NULL;

    if (PyBytes_Check(object)) {
        // We already have an encoded string, we suppose it has the correct encoding (UTF-8)
        temp_bytes = object;
        Py_INCREF(temp_bytes);
    } else if (PyUnicode_Check(object)) {
        // Encode the Unicode string that was given
        temp_bytes = PyUnicode_AsEncodedString(object, "UTF-8", "strict");
        if (temp_bytes == NULL) {
            // PyUnicode_AsEncodedString might raise an error if the codec raised an
            // exception
            PyErr_Clear();
            return NULL;
        }
    } else {
        return NULL;
    }

    retval = strdupe(PyBytes_AS_STRING(temp_bytes));
    Py_XDECREF(temp_bytes);

    return retval;
}

int init_stringutils(void) {
    PyObject *json = NULL;
    int ret = EXIT_FAILURE;

    char module_name[] = "json";
    json = PyImport_ImportModule(module_name);
    if (json == NULL) {
        goto done;
    }

    // get json.loads()
    char loads_name[] = "loads";
    jloads = PyObject_GetAttrString(json, loads_name);
    if (jloads == NULL) {
        goto done;
    }

    // get json.dumps()
    char dumps_name[] = "dumps";
    jdumps = PyObject_GetAttrString(json, dumps_name);
    if (jdumps == NULL) {
        goto done;
    }

    ret = EXIT_SUCCESS;

done:
    Py_XDECREF(json);
    return ret;
}

PyObject *from_json(const char *data) {
    PyObject *args = NULL;
    PyObject *retval = NULL;

    if (!data) {
        goto done;
    }
    if (jloads == NULL) {
        goto done;
    }

    args = Py_BuildValue("(s)", data);
    if (args == NULL) {
        goto done;
    }
    retval = PyObject_Call(jloads, args, NULL);

done:
    Py_XDECREF(args);
    return retval;
}

char *as_json(PyObject *object) {
    char *retval = NULL;
    PyObject *dumped = NULL;

    PyObject *args = Py_BuildValue("(O)", object);
    if (args == NULL) {
        goto done;
    }

    dumped = PyObject_Call(jdumps, args, NULL);
    if (dumped == NULL) {
        goto done;
    }
    retval = as_string(dumped);

done:
    //Py_XDECREF can accept (and ignore) NULL references
    Py_XDECREF(dumped);
    Py_XDECREF(args);
    return retval;
}
