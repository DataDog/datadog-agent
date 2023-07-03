#include "crashdump.h"
#include "_cgo_export.h"

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
 * readCrashDump
 *
 * the caller of this is calling from `go`.  We can't really log.
 * so return as the top level error which operation failed (as identified
 * by the enum), and store the actual hresult in the error parameter
 *  for logging by the caller
*/
READ_CRASH_DUMP_ERROR readCrashDump(char *fname, void *ctx, long * extendedError)
{
    IDebugClient* iClient = NULL;
    IDebugControl* iControl = NULL;
    StdioOutputCallbacks iOutputCb(ctx);
    READ_CRASH_DUMP_ERROR ret = RCD_NONE;
    HRESULT hr = DebugCreate(IID_IDebugClient, (void**)&iClient);
    if(S_OK != hr) {
        *extendedError = hr;
        ret = RCD_DEBUG_CREATE_FAILED;
        goto rcd_end;
    }
    hr = iClient->QueryInterface(IID_IDebugControl, (void**)&iControl);
    if(S_OK != hr) {
        *extendedError = hr;
        ret = RCD_QUERY_INTERFACE_FAILED;
        goto rcd_release_client;
    }    
    
    
    hr = iClient->SetOutputCallbacks(&iOutputCb);
    if(S_OK != hr) {
        *extendedError = hr;
        ret = RCD_SET_OUTPUT_CALLBACKS_FAILED;
        goto rcd_release_control;
    }
    hr = iClient->OpenDumpFile(fname);
    if(S_OK != hr) {
        *extendedError = hr;
        ret = RCD_OPEN_DUMP_FILE_FAILED;
        goto rcd_release_control;
    }
    hr = iControl->WaitForEvent(0, INFINITE);
    if(S_OK != hr) {
        *extendedError = hr;
        ret = RCD_WAIT_FOR_EVENT_FAILED;
        goto rcd_release_control;
    }
    hr = iControl->Execute(DEBUG_OUTCTL_THIS_CLIENT, "kb", DEBUG_EXECUTE_DEFAULT);
    if(S_OK != hr) {
        *extendedError = hr;
        ret = RCD_EXECUTE_FAILED;
        goto rcd_release_control;
    }
    // release intentionally left unchecked.
rcd_release_control:
    if(iControl){
        iControl->Release();
    }
rcd_release_client:
    if(iClient){
        iClient->Release();
    }
rcd_end:
    return ret;
   
}