#include "crashdump.h"
#include "_cgo_export.h"

#include <stdio.h>

#define INITGUID
#include "guiddef.h"
DEFINE_GUID(IID_IDebugControl, 0x5182e668, 0x105e, 0x416e,
            0xad, 0x92, 0x24, 0xef, 0x80, 0x04, 0x24, 0xba);
/* 27fe5639-8407-4f47-8364-ee118fb08ac8 */
DEFINE_GUID(IID_IDebugClient, 0x27fe5639, 0x8407, 0x4f47,
            0x83, 0x64, 0xee, 0x11, 0x8f, 0xb0, 0x8a, 0xc8);

/* 4bf58045-d654-4c40-b0af-683090f356dc */
DEFINE_GUID(IID_IDebugOutputCallbacks, 0x4bf58045, 0xd654, 0x4c40,
            0xb0, 0xaf, 0x68, 0x30, 0x90, 0xf3, 0x56, 0xdc);

class StdioOutputCallbacks : public IDebugOutputCallbacks {
private:
    StdioOutputCallbacks();
    void * ctx;
public:
    StdioOutputCallbacks(void *ctx)
        : ctx(ctx)
    {};
    STDMETHOD(QueryInterface)(THIS_ _In_ REFIID ifid, _Out_ PVOID* iface);
    STDMETHOD_(ULONG, AddRef)(THIS);
    STDMETHOD_(ULONG, Release)(THIS);
    STDMETHOD(Output)(THIS_ IN ULONG Mask, IN PCSTR Text);
};
STDMETHODIMP
StdioOutputCallbacks::QueryInterface(THIS_ _In_ REFIID ifid, _Out_ PVOID* iface) {
    *iface = NULL;
    if (IsEqualIID(ifid, IID_IDebugOutputCallbacks)) {
        *iface = (IDebugOutputCallbacks*)this;
        AddRef();
        return S_OK;
    }
    else {
        return E_NOINTERFACE;
    }
}
STDMETHODIMP_(ULONG)
StdioOutputCallbacks::AddRef(THIS) { return 1; }
STDMETHODIMP_(ULONG)
StdioOutputCallbacks::Release(THIS) { return 0; }
STDMETHODIMP StdioOutputCallbacks::Output(THIS_ IN ULONG, IN PCSTR Text) {
    logLineCallback(this->ctx, Text);
    return S_OK;
}

/*
 * getAgentVersion
 *
 * Tries to get the version of Datadog system-probe or agent found in the dump.
*/
HRESULT getAgentVersion(IDebugClient* iClient, BUGCHECK_INFO* bugCheckInfo, long* extendedError)
{
    // List of DD modules to query version information. Agent should be last.
    const char* k_ddModuleNames[] = { "system-probe", "agent" };
    const ULONG k_ddModuleCount = sizeof(k_ddModuleNames)/sizeof(k_ddModuleNames[0]);

    IDebugSymbols2* iSymbols = NULL;
    HRESULT hr = E_FAIL;
    ULONG index = 0;
    ULONG64 base = 0;
    bool moduleFound = false;

    hr = iClient->QueryInterface(__uuidof(IDebugSymbols2), (void**)&iSymbols);
    if (S_OK != hr) {
        *extendedError = RCD_QUERY_SYMBOLS_INTERFACE_FAILED;
        goto end;
    }

    // The Datadog driver modules do not have embedded resource tables and
    // therefore no version information to query.
    // On the other hand, the agent does have one, its version is more meaningful
    // and provides a fuller end-to-end context (e.g. feature combinations).
    // However, since "agent" is a generic name, we will first try query system-probe.
    for (ULONG i = 0; i < k_ddModuleCount; ++i) {
        hr = iSymbols->GetModuleByModuleName(k_ddModuleNames[i], 0, &index, &base);
        if (S_OK != hr) {
            continue;
        }

        moduleFound = true;

        // This basically queries the resource table of the module.
        // Either \StringFileInfo\040904b0\FileVersion or \StringFileInfo\040904b0\ProductVersion
        // will do.
        hr = iSymbols->GetModuleVersionInformation(
                        index,
                        base,
                        "\\StringFileInfo\\040904b0\\FileVersion",
                        bugCheckInfo->agentVersion,
                        sizeof(bugCheckInfo->agentVersion) - 1,
                        NULL);

        if (S_OK == hr) {
            // We successfully got the version.
            // Null termination should already be included but we will guarantee it.
            bugCheckInfo->agentVersion[sizeof(bugCheckInfo->agentVersion) - 1] = 0;
            break;
        }
    }

    if (S_OK != hr) {
        *extendedError = !moduleFound ? RCD_DD_MODULE_NOT_FOUND :
                                        RCD_GET_MODULE_VERSION_INFO_FAILED;
        goto end;
    }

end:

    if (NULL != iSymbols) {
        iSymbols->Release();
    }

    return hr;
}


/*
 * readCrashDump
 *
 * the caller of this is calling from `go`.  We can't really log.
 * so return as the top level error which operation failed (as identified
 * by the enum), and store the actual hresult in the error parameter
 *  for logging by the caller
*/
READ_CRASH_DUMP_ERROR readCrashDump(char* fname, void* ctx, BUGCHECK_INFO* bugCheckInfo, long* extendedError)
{
    IDebugClient* iClient = NULL;
    IDebugControl* iControl = NULL;
    StdioOutputCallbacks iOutputCb(ctx);
    READ_CRASH_DUMP_ERROR ret = RCD_NONE;
    HRESULT hr = E_FAIL;

    if ((NULL == fname) || (NULL == bugCheckInfo) || (NULL == extendedError)) {
        ret = RCD_INVALID_ARG;
        goto end;
    }

    ZeroMemory(bugCheckInfo, sizeof(*bugCheckInfo));

    hr = DebugCreate(IID_IDebugClient, (void**)&iClient);
    if (S_OK != hr) {
        *extendedError = hr;
        ret = RCD_DEBUG_CREATE_FAILED;
        goto end;
    }

    hr = iClient->QueryInterface(IID_IDebugControl, (void**)&iControl);
    if (S_OK != hr) {
        *extendedError = hr;
        ret = RCD_QUERY_INTERFACE_FAILED;
        goto end;
    }

    hr = iClient->SetOutputCallbacks(&iOutputCb);
    if (S_OK != hr) {
        *extendedError = hr;
        ret = RCD_SET_OUTPUT_CALLBACKS_FAILED;
        goto end;
    }

    hr = iClient->OpenDumpFile(fname);
    if (S_OK != hr) {
        *extendedError = hr;
        ret = RCD_OPEN_DUMP_FILE_FAILED;
        goto end;
    }

    hr = iControl->WaitForEvent(0, INFINITE);
    if (S_OK != hr) {
        *extendedError = hr;
        ret = RCD_WAIT_FOR_EVENT_FAILED;
        goto end;
    }

    hr = iControl->ReadBugCheckData(
             &bugCheckInfo->code,
             &bugCheckInfo->arg1,
             &bugCheckInfo->arg2,
             &bugCheckInfo->arg3,
             &bugCheckInfo->arg4);
    if (S_OK != hr) {
        // OK to fail. This may not be a proper kernel dump and will fail with user-mode dumps.
        // Continue and try get other data.
        *extendedError = hr;
    }

    hr = iControl->Execute(DEBUG_OUTCTL_THIS_CLIENT, "kb", DEBUG_EXECUTE_DEFAULT);
    if (S_OK != hr) {
        *extendedError = hr;
        ret = RCD_EXECUTE_FAILED;
        goto end;
    }

    hr = getAgentVersion(iClient, bugCheckInfo, extendedError);
    if (S_OK != hr) {
        // Ignore the error and return what is available.
        hr = S_OK;
        goto end;
    }

end:

    if (NULL != iControl) {
        iControl->Release();
    }

    if (NULL != iClient) {
        iClient->Release();
    }

    return ret;
}
