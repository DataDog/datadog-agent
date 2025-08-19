// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2019-present Datadog, Inc.
#ifndef DATADOG_AGENT_RTLOADER_UTIL__H
#define DATADOG_AGENT_RTLOADER_UTIL__H
#include <Python.h>
#include <rtloader_types.h>

/*! \file _util.h
    \brief RtLoader _util builtin header file.

    The prototypes here defined provide functions to initialize the python _util
    builtin module, and set its relevant callbacks for the rtloader caller.
*/
/*! \fn PyMODINIT_FUNC PyInit__util(void)
    \brief Initializes the _util builtin python module.

    The _util python builtin is created and registered here as per the module_def
    PyMethodDef definition in `_util.c` with the corresponding C-implemented python
    methods. A fresh reference to the module is created here. This function is
    python3 only.
*/
/*! \fn void _set_get_subprocess_output_cb(cb_get_subprocess_output_t)
    \brief Sets a callback to be used by rtloader to run subprocess commands and collect their
    output.
    \param object A function pointer with cb_get_subprocess_output_t prototype to the callback
    function.

    The callback is expected to be provided by the rtloader caller - in go-context: CGO.
*/

#define _DOT "."
#define _UTIL_MODULE_NAME "_util"
#define _SUBPROCESS_OUTPUT_ERROR_NAME "SubprocessOutputEmptyError"
#define _SUBPROCESS_OUTPUT_ERROR_NS_NAME _UTIL_MODULE_NAME _DOT _SUBPROCESS_OUTPUT_ERROR_NAME

// The keyword-only arguments separator ($) for PyArg_ParseTupleAndKeywords()
// has been introduced in Python 3.3
// https://docs.python.org/3/c-api/arg.html#other-objects
#define PY_ARG_PARSE_TUPLE_KEYWORD_ONLY "$"

#ifdef __cplusplus
extern "C" {
#endif

PyMODINIT_FUNC PyInit__util(void);

void _set_get_subprocess_output_cb(cb_get_subprocess_output_t);
#ifdef __cplusplus
}
#endif

#endif
