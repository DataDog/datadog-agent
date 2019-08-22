// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

#ifndef DD_EVENT_H
#define DD_EVENT_H

#include <windows.h>
#include <winerror.h>
#include "winevt.h"
#include <conio.h>
#include <stdio.h>
#include <stdlib.h>

typedef struct RichEvent_t {
    LPWSTR message;
    LPWSTR task;
    LPWSTR opcode;
    LPWSTR level;
} RichEvent ;

ULONGLONG startEventSubscribe(char *channel, char* query, ULONGLONG  ullBookmark, int flags, PVOID ctx);
RichEvent* EnrichEvent(ULONGLONG ullEvent);

/// our version of winerror.h doesn't have these... when we get an up-to-date compiler,
// should be able to remove.

///////////////////////////////////////////////////
//                                               //
//           Start of WinEvt Error codes         //
//                                               //
//                 15000 to 15079                //
///////////////////////////////////////////////////

//
// MessageId: ERROR_EVT_INVALID_CHANNEL_PATH
//
// MessageText:
//
// The specified channel path is invalid.
//
#define ERROR_EVT_INVALID_CHANNEL_PATH   15000L

//
// MessageId: ERROR_EVT_INVALID_QUERY
//
// MessageText:
//
// The specified query is invalid.
//
#define ERROR_EVT_INVALID_QUERY          15001L

//
// MessageId: ERROR_EVT_PUBLISHER_METADATA_NOT_FOUND
//
// MessageText:
//
// The publisher metadata cannot be found in the resource.
//
#define ERROR_EVT_PUBLISHER_METADATA_NOT_FOUND 15002L

//
// MessageId: ERROR_EVT_EVENT_TEMPLATE_NOT_FOUND
//
// MessageText:
//
// The template for an event definition cannot be found in the resource (error = %1).
//
#define ERROR_EVT_EVENT_TEMPLATE_NOT_FOUND 15003L

//
// MessageId: ERROR_EVT_INVALID_PUBLISHER_NAME
//
// MessageText:
//
// The specified publisher name is invalid.
//
#define ERROR_EVT_INVALID_PUBLISHER_NAME 15004L

//
// MessageId: ERROR_EVT_INVALID_EVENT_DATA
//
// MessageText:
//
// The event data raised by the publisher is not compatible with the event template definition in the publisher's manifest
//
#define ERROR_EVT_INVALID_EVENT_DATA     15005L

//
// MessageId: ERROR_EVT_CHANNEL_NOT_FOUND
//
// MessageText:
//
// The specified channel could not be found. Check channel configuration.
//
#define ERROR_EVT_CHANNEL_NOT_FOUND      15007L

//
// MessageId: ERROR_EVT_MALFORMED_XML_TEXT
//
// MessageText:
//
// The specified xml text was not well-formed. See Extended Error for more details.
//
#define ERROR_EVT_MALFORMED_XML_TEXT     15008L

//
// MessageId: ERROR_EVT_SUBSCRIPTION_TO_DIRECT_CHANNEL
//
// MessageText:
//
// The caller is trying to subscribe to a direct channel which is not allowed. The events for a direct channel go directly to a logfile and cannot be subscribed to.
//
#define ERROR_EVT_SUBSCRIPTION_TO_DIRECT_CHANNEL 15009L

//
// MessageId: ERROR_EVT_CONFIGURATION_ERROR
//
// MessageText:
//
// Configuration error.
//
#define ERROR_EVT_CONFIGURATION_ERROR    15010L

//
// MessageId: ERROR_EVT_QUERY_RESULT_STALE
//
// MessageText:
//
// The query result is stale / invalid. This may be due to the log being cleared or rolling over after the query result was created. Users should handle this code by releasing the query result object and reissuing the query.
//
#define ERROR_EVT_QUERY_RESULT_STALE     15011L

//
// MessageId: ERROR_EVT_QUERY_RESULT_INVALID_POSITION
//
// MessageText:
//
// Query result is currently at an invalid position.
//
#define ERROR_EVT_QUERY_RESULT_INVALID_POSITION 15012L

//
// MessageId: ERROR_EVT_NON_VALIDATING_MSXML
//
// MessageText:
//
// Registered MSXML doesn't support validation.
//
#define ERROR_EVT_NON_VALIDATING_MSXML   15013L

//
// MessageId: ERROR_EVT_FILTER_ALREADYSCOPED
//
// MessageText:
//
// An expression can only be followed by a change of scope operation if it itself evaluates to a node set and is not already part of some other change of scope operation.
//
#define ERROR_EVT_FILTER_ALREADYSCOPED   15014L

//
// MessageId: ERROR_EVT_FILTER_NOTELTSET
//
// MessageText:
//
// Can't perform a step operation from a term that does not represent an element set.
//
#define ERROR_EVT_FILTER_NOTELTSET       15015L

//
// MessageId: ERROR_EVT_FILTER_INVARG
//
// MessageText:
//
// Left hand side arguments to binary operators must be either attributes, nodes or variables and right hand side arguments must be constants.
//
#define ERROR_EVT_FILTER_INVARG          15016L

//
// MessageId: ERROR_EVT_FILTER_INVTEST
//
// MessageText:
//
// A step operation must involve either a node test or, in the case of a predicate, an algebraic expression against which to test each node in the node set identified by the preceding node set can be evaluated.
//
#define ERROR_EVT_FILTER_INVTEST         15017L

//
// MessageId: ERROR_EVT_FILTER_INVTYPE
//
// MessageText:
//
// This data type is currently unsupported.
//
#define ERROR_EVT_FILTER_INVTYPE         15018L

//
// MessageId: ERROR_EVT_FILTER_PARSEERR
//
// MessageText:
//
// A syntax error occurred at position %1!d!
//
#define ERROR_EVT_FILTER_PARSEERR        15019L

//
// MessageId: ERROR_EVT_FILTER_UNSUPPORTEDOP
//
// MessageText:
//
// This operator is unsupported by this implementation of the filter.
//
#define ERROR_EVT_FILTER_UNSUPPORTEDOP   15020L

//
// MessageId: ERROR_EVT_FILTER_UNEXPECTEDTOKEN
//
// MessageText:
//
// The token encountered was unexpected.
//
#define ERROR_EVT_FILTER_UNEXPECTEDTOKEN 15021L

//
// MessageId: ERROR_EVT_INVALID_OPERATION_OVER_ENABLED_DIRECT_CHANNEL
//
// MessageText:
//
// The requested operation cannot be performed over an enabled direct channel. The channel must first be disabled before performing the requested operation.
//
#define ERROR_EVT_INVALID_OPERATION_OVER_ENABLED_DIRECT_CHANNEL 15022L

//
// MessageId: ERROR_EVT_INVALID_CHANNEL_PROPERTY_VALUE
//
// MessageText:
//
// Channel property %1!s! contains invalid value. The value has invalid type, is outside of valid range, can't be updated or is not supported by this type of channel.
//
#define ERROR_EVT_INVALID_CHANNEL_PROPERTY_VALUE 15023L

//
// MessageId: ERROR_EVT_INVALID_PUBLISHER_PROPERTY_VALUE
//
// MessageText:
//
// Publisher property %1!s! contains invalid value. The value has invalid type, is outside of valid range, can't be updated or is not supported by this type of publisher.
//
#define ERROR_EVT_INVALID_PUBLISHER_PROPERTY_VALUE 15024L

//
// MessageId: ERROR_EVT_CHANNEL_CANNOT_ACTIVATE
//
// MessageText:
//
// The channel fails to activate.
//
#define ERROR_EVT_CHANNEL_CANNOT_ACTIVATE 15025L

//
// MessageId: ERROR_EVT_FILTER_TOO_COMPLEX
//
// MessageText:
//
// The xpath expression exceeded supported complexity. Please symplify it or split it into two or more simple expressions.
//
#define ERROR_EVT_FILTER_TOO_COMPLEX     15026L

//
// MessageId: ERROR_EVT_MESSAGE_NOT_FOUND
//
// MessageText:
//
// the message resource is present but the message is not found in the string/message table
//
#define ERROR_EVT_MESSAGE_NOT_FOUND      15027L

//
// MessageId: ERROR_EVT_MESSAGE_ID_NOT_FOUND
//
// MessageText:
//
// The message id for the desired message could not be found.
//
#define ERROR_EVT_MESSAGE_ID_NOT_FOUND   15028L

//
// MessageId: ERROR_EVT_UNRESOLVED_VALUE_INSERT
//
// MessageText:
//
// The substitution string for insert index (%1) could not be found.
//
#define ERROR_EVT_UNRESOLVED_VALUE_INSERT 15029L

//
// MessageId: ERROR_EVT_UNRESOLVED_PARAMETER_INSERT
//
// MessageText:
//
// The description string for parameter reference (%1) could not be found.
//
#define ERROR_EVT_UNRESOLVED_PARAMETER_INSERT 15030L

//
// MessageId: ERROR_EVT_MAX_INSERTS_REACHED
//
// MessageText:
//
// The maximum number of replacements has been reached.
//
#define ERROR_EVT_MAX_INSERTS_REACHED    15031L

//
// MessageId: ERROR_EVT_EVENT_DEFINITION_NOT_FOUND
//
// MessageText:
//
// The event definition could not be found for event id (%1).
//
#define ERROR_EVT_EVENT_DEFINITION_NOT_FOUND 15032L

//
// MessageId: ERROR_EVT_MESSAGE_LOCALE_NOT_FOUND
//
// MessageText:
//
// The locale specific resource for the desired message is not present.
//
#define ERROR_EVT_MESSAGE_LOCALE_NOT_FOUND 15033L

//
// MessageId: ERROR_EVT_VERSION_TOO_OLD
//
// MessageText:
//
// The resource is too old to be compatible.
//
#define ERROR_EVT_VERSION_TOO_OLD        15034L

//
// MessageId: ERROR_EVT_VERSION_TOO_NEW
//
// MessageText:
//
// The resource is too new to be compatible.
//
#define ERROR_EVT_VERSION_TOO_NEW        15035L

//
// MessageId: ERROR_EVT_CANNOT_OPEN_CHANNEL_OF_QUERY
//
// MessageText:
//
// The channel at index %1!d! of the query can't be opened.
//
#define ERROR_EVT_CANNOT_OPEN_CHANNEL_OF_QUERY 15036L

//
// MessageId: ERROR_EVT_PUBLISHER_DISABLED
//
// MessageText:
//
// The publisher has been disabled and its resource is not available. This usually occurs when the publisher is in the process of being uninstalled or upgraded.
//
#define ERROR_EVT_PUBLISHER_DISABLED     15037L

//
// MessageId: ERROR_EVT_FILTER_OUT_OF_RANGE
//
// MessageText:
//
// Attempted to create a numeric type that is outside of its valid range.
//
#define ERROR_EVT_FILTER_OUT_OF_RANGE    15038L

#endif /* DD_EVENT_H */
