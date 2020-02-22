#pragma once

#ifdef __cplusplus
extern "C" {
#endif
    // provide this definition so that code shared with the custom action can log
    // straight to STDOUT
    typedef enum LOGLEVEL
    {
        LOGMSG_TRACEONLY,  // Never written to the log file (except in DEBUG builds)
        LOGMSG_VERBOSE,    // Written to log when LOGVERBOSE
        LOGMSG_STANDARD    // Written to log whenever informational logging is enabled
    } LOGLEVEL;
    void __cdecl WcaLog(__in LOGLEVEL llv, __in_z __format_string PCSTR fmt, ...);
#ifdef __cplusplus
}
#endif
