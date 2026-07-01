// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog
// (https://www.datadoghq.com/).
// Copyright 2019-present Datadog, Inc.
#ifndef DATADOG_AGENT_RTLOADER_STRINGUTILS_H
#define DATADOG_AGENT_RTLOADER_STRINGUTILS_H

/*! \file stringutils.h
    \brief RtLoader String Utilities header file.

    The prototypes here defined provide a set of helper functions to convert between
    strings in their C representation and respective python objects. This includes native
    strings but also JSON representations of python objects - allowing for serialization.
    Please make sure to properly initialize stringutils before using.
*/
/*! \fn int init_stringutils(void)
    \brief Initializes stringutils; grabbing and caching the python json.loads and json.dumps objects.
    \return int The success of the operation `EXIT_SUCCESS` (0) or `EXIT_FAILURE` (1).
    \sa as_json, from_json

    The function must be called before using the json helper functions in the module.
    Typically this is expected to be done during initialization. The routine grabs the
    json.loads and json.dumps method references and caches them so `as_json` and `from_json`
    will not need to grab new references and will be able to call them directly.
*/
/*! \fn char *as_string(PyObject * object)
    \brief Returns a C string representation of the supplied Python string object.
    \param object The Python string object to convert.
    \return char * representation of the supplied string. In case of error NULL is returned.

    The returned C-string is allocated by this function and should subsequently be freed by
    the caller. This function should not set errors on the python interpreter.
*/
/*! \fn PyObject *from_json(const char * data)
    \brief Returns a Python object representation for the supplied JSON C-string.
    \param data The JSON C-string representation of the object we wish to deserialize.
    \return PyObject * pointer to the python object representation of the supplied JSON C
    string. In case of error, NULL will be returned.

    The returned Python object is a new reference and should subsequently be DECREF'd when
    no longer used, wanted by the caller.
*/
/*! \fn char *as_json(PyObject * object)
    \brief Returns a C string JSON representation for the supplied Python object.
    \param object The python object whose JSON representation we want.
    \return char * pointer to the C-string representation for the supplied Python object.
    In case of error, NULL will be returned.

    The returned C-string JSON representation is allocated by the function and should
    be subsequently freed by the caller.
*/

#ifdef __cplusplus
extern "C" {
#endif

#include <Python.h>

int init_stringutils(void);
char *as_string(PyObject *);
PyObject *from_json(const char *);
char *as_json(PyObject *);

#ifdef __cplusplus
}
#endif

#endif
