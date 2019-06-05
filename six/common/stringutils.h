// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog
// (https://www.datadoghq.com/).
// Copyright 2019 Datadog, Inc.
#ifndef DATADOG_AGENT_SIX_STRINGUTILS_H
#define DATADOG_AGENT_SIX_STRINGUTILS_H

/*! \file stringutils.h
    \brief Six String Utilities header file.

    The prototypes here defined provide a set of helper functions to convert between
    strings in their C representation and respective python objects. This includes native
    strings but also YAML representations of python objects - allowing for serialization.
*/
/*! \fn char *as_string(PyObject * object)
    \brief Returns a Python object representation for the supplied YAML C-string.
    \param object The YAML C-string representation of the object we wish to deserialize.
    \return char * representation of the supplied string. In case of error NULL is returned.

    The returned C-string is allocated by this function and should subsequently be freed by
    the caller. This function should not set errors on the python interpreter.
*/
/*! \fn PyObject *from_yaml(const char * object)
    \brief Returns a Python object representation for the supplied YAML C-string.
    \param object The YAML C-string representation of the object we wish to deserialize.
    \return PyObject * pointer to the python object representation of the supplied yaml C
    string. In case of error, NULL will be returned.

    The returned Python object is a new reference and should subsequently be DECREF'd when
    no longer used, wanted by the caller.
*/
/*! \fn char *as_yaml(PyObject * object)
    \brief Returns a C string YAML representation for the supplied Python object.
    \param object The python object whose YAML representation we want.
    \return char * pointer to the C-string representation for the supplied Python object.
    In case of error, NULL will be returned.

    The returned C-string YAML representation is allocated by the function and should
    be subsequently freed by the caller.
*/
/*! \def PyStringFromCString(x)
    \brief A macro that returns a Python string from C string x (char *).

    This macro is implemented differently in the Python2 and Python3 variants.
    For Python2 this macro wraps and calls PyString_FromString(x).
    For Python3 this macro wraps and calls PyUnicode_FromString(x).
*/

#ifdef __cplusplus
extern "C" {
#endif

#include <Python.h>

char *as_string(PyObject *);
PyObject *from_yaml(const char *);
char *as_yaml(PyObject *);

#ifdef DATADOG_AGENT_THREE
#    define PyStringFromCString(x) PyUnicode_FromString(x)
#elif defined(DATADOG_AGENT_TWO)
#    define PyStringFromCString(x) PyString_FromString(x)
#endif

#ifdef __cplusplus
}
#endif

#endif
