// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

#define _WIN32_WINNT 0x0602
#include "event.h"

#include "_cgo_export.h"

DWORD WINAPI SubscriptionCallback(EVT_SUBSCRIBE_NOTIFY_ACTION action, PVOID pContext, EVT_HANDLE hEvent);
DWORD PrintEvent(EVT_HANDLE hEvent);
ULONGLONG startEventSubscribe(char *channel, char* query, ULONGLONG ullBookmark, int iFlags, PVOID ctx)
{
    EVT_HANDLE hBookmark = (EVT_HANDLE) (ULONG_PTR)ullBookmark;
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
    return (ULONGLONG)(ULONG_PTR)hSubscription;
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
		if ((ULONGLONG)ERROR_EVT_QUERY_RESULT_STALE == (ULONGLONG)(ULONG_PTR)hEvent)
		{
			goStaleCallback((ULONGLONG)(ULONG_PTR)hEvent, pContext);
		}
		else
		{
			goErrorCallback((ULONGLONG)(ULONG_PTR)hEvent, pContext);
		}
		break;

	case EvtSubscribeActionDeliver:
		goNotificationCallback((ULONGLONG) (ULONG_PTR)hEvent, pContext);
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

	if (pRenderedContent) {
		free(pRenderedContent);
	}

	return status;
}

LPWSTR FormatEvtField(EVT_HANDLE hMetadata, EVT_HANDLE hEvent, EVT_FORMAT_MESSAGE_FLAGS FormatId);
PEVT_VARIANT GetProviderName(EVT_HANDLE hEvent);
RichEvent* EnrichEvent(ULONGLONG ullEvent)
{
    EVT_HANDLE hProviderMetadata = NULL;
    LPWSTR pwsMessage = NULL;
    EVT_HANDLE hEvent = (EVT_HANDLE)(ULONG_PTR) ullEvent;
    RichEvent *richEvent = (RichEvent*)malloc(sizeof(RichEvent));

    // Get Provider name
    PEVT_VARIANT pRenderedValues = GetProviderName(hEvent);
    if (NULL == pRenderedValues) {
        free(richEvent);
        richEvent = NULL;
        goto cleanup;
    }

    LPCWSTR providerName = pRenderedValues[0].StringVal;
    if (NULL == providerName) {
        free(richEvent);
        richEvent = NULL;
        goto cleanup;
    }

    // Get Provider metadata
    hProviderMetadata = EvtOpenPublisherMetadata(NULL, providerName, NULL, 0, 0);


    if (NULL == hProviderMetadata)
    {
        wprintf(L"EvtOpenPublisherMetadata failed with %d\n", GetLastError());
        free(richEvent);
        richEvent = NULL;
        goto cleanup;
    }

    // Render the fields
    richEvent->message = FormatEvtField(hProviderMetadata, hEvent, EvtFormatMessageEvent);
    richEvent->task = FormatEvtField(hProviderMetadata, hEvent, EvtFormatMessageTask);
    richEvent->opcode = FormatEvtField(hProviderMetadata, hEvent, EvtFormatMessageOpcode);
    richEvent->level = FormatEvtField(hProviderMetadata, hEvent, EvtFormatMessageLevel);

cleanup:

    if (hEvent) {
        EvtClose(hEvent);
    }
    if (pRenderedValues) {
        free(pRenderedValues);
    }
    if (hProviderMetadata) {
        EvtClose(hProviderMetadata);
    }

    return richEvent;
}

// Extract the provider name from the event
PEVT_VARIANT GetProviderName(EVT_HANDLE hEvent)
{
    DWORD status = ERROR_SUCCESS;
    EVT_HANDLE hContext = NULL;
    DWORD dwBufferSize = 0;
    DWORD dwBufferUsed = 0;
    DWORD dwPropertyCount = 0;
    PEVT_VARIANT pRenderedValues = NULL;
    LPWSTR ppValues[] = {L"Event/System/Provider/@Name"};
    DWORD count = sizeof(ppValues)/sizeof(LPWSTR);

    // Identify the components of the event that you want to render. In this case,
    // render the provider's name and channel from the system section of the event.
    hContext = EvtCreateRenderContext(count, (LPCWSTR*)ppValues, EvtRenderContextValues);
    if (NULL == hContext)
    {
        wprintf(L"EvtCreateRenderContext failed with %lu\n", status = GetLastError());
        goto cleanup;
    }

    // The function returns an array of variant values for each element or attribute that
    // you want to retrieve from the event. The values are returned in the same order as
    // you requested them.
    if (!EvtRender(hContext, hEvent, EvtRenderEventValues, dwBufferSize, pRenderedValues, &dwBufferUsed, &dwPropertyCount))
    {
        if (ERROR_INSUFFICIENT_BUFFER == (status = GetLastError()))
        {
            dwBufferSize = dwBufferUsed;
            pRenderedValues = (PEVT_VARIANT)malloc(dwBufferSize);
            if (pRenderedValues)
            {
                EvtRender(hContext, hEvent, EvtRenderEventValues, dwBufferSize, pRenderedValues, &dwBufferUsed, &dwPropertyCount);
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
            wprintf(L"EvtRender failed with %d\n", GetLastError());
            goto cleanup;
        }
    }

cleanup:

    if (hContext) {
        EvtClose(hContext);
    }

    return pRenderedValues;
}

// Get the string representation of the given event field
LPWSTR FormatEvtField(EVT_HANDLE hMetadata, EVT_HANDLE hEvent, EVT_FORMAT_MESSAGE_FLAGS FormatId)
{
    LPWSTR pBuffer = NULL;
    DWORD dwBufferSize = 0;
    DWORD dwBufferUsed = 0;
    DWORD status = 0;

    if (!EvtFormatMessage(hMetadata, hEvent, 0, 0, NULL, FormatId, dwBufferSize, pBuffer, &dwBufferUsed))
    {
        status = GetLastError();
        if (ERROR_INSUFFICIENT_BUFFER == status)
        {
            // An event can contain one or more keywords. The function returns keywords
            // as a list of keyword strings. To process the list, you need to know the
            // size of the buffer, so you know when you have read the last string, or you
            // can terminate the list of strings with a second null terminator character
            // as this example does.
            if ((EvtFormatMessageKeyword == FormatId)) {
                pBuffer[dwBufferSize-1] = L'\0';
            }
            else {
                dwBufferSize = dwBufferUsed;
            }

            pBuffer = (LPWSTR)malloc(dwBufferSize * sizeof(WCHAR));

            if (pBuffer)
            {
                EvtFormatMessage(hMetadata, hEvent, 0, 0, NULL, FormatId, dwBufferSize, pBuffer, &dwBufferUsed);

                // Add the second null terminator character.
                if ((EvtFormatMessageKeyword == FormatId)) {
                    pBuffer[dwBufferUsed-1] = L'\0';
                }
            }
            else
            {
                wprintf(L"malloc failed\n");
            }
        }
        else if (ERROR_EVT_MESSAGE_NOT_FOUND == status || ERROR_EVT_MESSAGE_ID_NOT_FOUND == status)
            ;
        else
        {
        // Remove this log because it can get very spammy. It should be using
        // a function that will send logs to DD agent in debug / trace mode
        // TODO(achntrl): Replace the wprintf with DD agent logger
        //     wprintf(L"EvtFormatMessage failed with %u\n", status);
        }
    }

    return pBuffer;
}
