// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2019-present Datadog, Inc.
#ifndef DATADOG_AGENT_RTLOADER_LOG_H
#define DATADOG_AGENT_RTLOADER_LOG_H

/*! \file log.h
    \brief RtLoader log builtin header file.

    The prototypes here provide functions to logs messages from rtloader to the
    agent logger.
*/

#include "rtloader_types.h"

#ifdef __cplusplus
extern "C" {
#endif

/*! \fn void _set_log_cb(cb_log_t)
    \brief Sets a callback to be used by rtloader to logs messages to the agent
    logger.
    \param object A function pointer to the callback function.

    The callback is expected to be provided by the rtloader caller - in go-context: CGO.
*/
void _set_log_cb(cb_log_t);

/*! \fn void agent_log( log_level_t, const char *)
    \brief Logs the message to the agent loggers.
    \param log_level_t The log level to use to log the message.
    \param const char* A pointer to the message.
*/
void agent_log(log_level_t, char *);

#ifdef __cplusplus
}
#endif

#endif
