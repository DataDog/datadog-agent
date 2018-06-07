// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build windows

#define _WIN32_WINNT 0x0602
#include "event.h"

#include "_cgo_export.h"

DWORD WINAPI SubscriptionCallback(EVT_SUBSCRIBE_NOTIFY_ACTION action, PVOID pContext, EVT_HANDLE hEvent);
DWORD PrintEvent(EVT_HANDLE hEvent);
ULONGLONG startEventSubscribe(char *channel, char* query, ULONGLONG ulBookmark, int iFlags, PVOID ctx)
{
    EVT_HANDLE hBookmark = (EVT_HANDLE) ulBookmark;
    EVT_SUBSCRIBE_FLAGS flags = (EVT_SUBSCRIBE_FLAGS) iFlags;
	DWORD status = ERROR_SUCCESS;
	EVT_HANDLE hSubscription = NULL;
	LPWSTR pwsChannel = NULL;
	LPWSTR pwsQuery = NULL;

    if (channel != NULL) {
        size_t chlen = mbstowcs(NULL, channel, 0) + 1;
        pwsChannel = malloc(chlen * sizeof(wchar_t));
        mbstowcs(pwsChannel, channel, chlen);
    }

    if (query != NULL) {
        size_t qlen = mbstowcs(NULL, query, 0) + 1;
        pwsQuery = malloc(qlen * sizeof(wchar_t));
        mbstowcs(pwsQuery, query, qlen);
    }


	// Subscribe to events beginning with the oldest event in the channel. The subscription
	// will return all current events in the channel and any future events that are raised
	// while the application is active.
	hSubscription = EvtSubscribe(NULL, NULL, pwsChannel, pwsQuery, NULL, ctx,
		(EVT_SUBSCRIBE_CALLBACK)SubscriptionCallback, flags);
	if (NULL == hSubscription)
	{
		status = GetLastError();

		if (ERROR_EVT_CHANNEL_NOT_FOUND == status)
			wprintf(L"Channel %s was not found.\n", pwsChannel);
		else if (ERROR_EVT_INVALID_QUERY == status)
			// You can call EvtGetExtendedStatus to get information as to why the query is not valid.
			wprintf(L"The query \"%s\" is not valid.\n", pwsQuery);
		else
			wprintf(L"EvtSubscribe failed with %lu.\n", status);

		goto cleanup;
	}
cleanup:
    if(pwsQuery) {
        free (pwsQuery);
    }
    if(pwsChannel){
        free(pwsChannel);
    }
    return (ULONGLONG)hSubscription;
}

// The callback that receives the events that match the query criteria.
DWORD WINAPI SubscriptionCallback(EVT_SUBSCRIBE_NOTIFY_ACTION action, PVOID pContext, EVT_HANDLE hEvent)
{
	UNREFERENCED_PARAMETER(pContext);

	DWORD status = ERROR_SUCCESS;

	switch (action)
	{
		// You should only get the EvtSubscribeActionError action if your subscription flags
		// includes EvtSubscribeStrict and the channel contains missing event records.
	case EvtSubscribeActionError:
		if ((ULONGLONG)ERROR_EVT_QUERY_RESULT_STALE == (ULONGLONG)hEvent)
		{
			goStaleCallback((ULONGLONG)hEvent, pContext);
		}
		else
		{
			goErrorCallback((ULONGLONG) hEvent, pContext);
		}
		break;

	case EvtSubscribeActionDeliver:
		goNotificationCallback((ULONGLONG) hEvent, pContext);
		break;

	default:
		wprintf(L"SubscriptionCallback: Unknown action.\n");
	}

cleanup:

	if (ERROR_SUCCESS != status)
	{
		// End subscription - Use some kind of IPC mechanism to signal
		// your application to close the subscription handle.
	}

	return status; // The service ignores the returned status.
}

// Render the event as an XML string and print it.
DWORD PrintEvent(EVT_HANDLE hEvent)
{
	DWORD status = ERROR_SUCCESS;
	DWORD dwBufferSize = 0;
	DWORD dwBufferUsed = 0;
	DWORD dwPropertyCount = 0;
	LPWSTR pRenderedContent = NULL;

	if (!EvtRender(NULL, hEvent, EvtRenderEventXml, dwBufferSize, pRenderedContent, &dwBufferUsed, &dwPropertyCount))
	{
		if (ERROR_INSUFFICIENT_BUFFER == (status = GetLastError()))
		{
			dwBufferSize = dwBufferUsed;
			pRenderedContent = (LPWSTR)malloc(dwBufferSize);
			if (pRenderedContent)
			{
				EvtRender(NULL, hEvent, EvtRenderEventXml, dwBufferSize, pRenderedContent, &dwBufferUsed, &dwPropertyCount);
			}
			else
			{
				wprintf(L"malloc failed\n");
				status = ERROR_OUTOFMEMORY;
				goto cleanup;
			}
		}

		if (ERROR_SUCCESS != (status = GetLastError()))
		{
			wprintf(L"EvtRender failed with %d\n", status);
			goto cleanup;
		}
	}

	wprintf(L"%s\n\n", pRenderedContent);

cleanup:

	if (pRenderedContent)
		free(pRenderedContent);

	return status;
}
