//
// installsteps.cpp
//
// On installation, record whether an install step was completed successfully.
// On rollback, indicate whether the given step needs to be undone

#include "stdafx.h"



DWORD OpenInstallKey(HKEY &hKey, bool rw) {
    // open the registry key where we're going to store this step
    
    DWORD pid = GetCurrentProcessId();
    wchar_t pidAsString[32];
    _itow_s(pid, pidAsString, 32, 10);
    std::string shortpath;
    toMbcs(shortpath, datadog_key_root.c_str());
    WcaLog(LOGMSG_STANDARD, "datadog_key_root: %s", shortpath.c_str());
    std::wstring thisProcKey = installStepsKey + L"\\" + pidAsString;
    DWORD disp = 0;
    
    toMbcs(shortpath, thisProcKey.c_str());
    WcaLog(LOGMSG_STANDARD, "attempting to create key %s", shortpath.c_str());
    DWORD status = RegCreateKeyExW(HKEY_LOCAL_MACHINE,
        thisProcKey.c_str(),
        0, // reserved, 0
        NULL, // ignored
        REG_OPTION_VOLATILE, // will cause keys to be deleted upon reboot
        rw ? KEY_ALL_ACCESS : KEY_QUERY_VALUE,
        NULL, // no inheritance
        &hKey,
        &disp);
    if (status != ERROR_SUCCESS) {
        WcaLog(LOGMSG_STANDARD, "createKey %d ", status);
        return status ;
    }
    if (false == rw && REG_CREATED_NEW_KEY == disp) {
        // key wasn't already there, but we opened for read
        // only; just fail
        WcaLog(LOGMSG_STANDARD, "Key already exists, but not rw");
        RegCloseKey(hKey);
        hKey = NULL;
        return ERROR_PATH_NOT_FOUND;
    }
    WcaLog(LOGMSG_STANDARD, "Created key 0x%x", (DWORD)hKey);
    return 0;

}
void MarkInstallStepComplete(std::wstring &step)
{
    HKEY hKey = NULL;
    DWORD val = 1;
    std::string shortkey;
    toMbcs(shortkey, step.c_str());
    DWORD ret = OpenInstallKey(hKey, true);
    if (ret) {
        WcaLog(LOGMSG_STANDARD, "Failed to open registry key for saving install step %s %d", shortkey.c_str(), ret);
        return;
    }
    WcaLog(LOGMSG_STANDARD,"Key is 0x%x", (DWORD)hKey);
    ret = RegSetValueExW(hKey, step.c_str(), 0, REG_DWORD, (BYTE *)&val, sizeof(DWORD));
    if (ret) {
        WcaLog(LOGMSG_STANDARD, "Failed to save registry key for saving install step %s %d", shortkey.c_str(),  ret);
    }
    WcaLog(LOGMSG_STANDARD, "Wrote save key for %s", shortkey.c_str());
    if (hKey != NULL) {
        RegCloseKey(hKey);
    }
}
bool WasInstallStepCompleted(std::wstring &step)
{
    HKEY hKey = NULL;
    bool retval = false;
    DWORD val = 0;
    DWORD sz = sizeof(DWORD);
    std::string shortkey;
    toMbcs(shortkey, step.c_str());
    
    DWORD ret = OpenInstallKey(hKey, false);
    if (ret) {
        WcaLog(LOGMSG_STANDARD, "Failed to open registry key for querying  install step %s %d", shortkey.c_str(), ret);
        return false;
    }
    ret = RegQueryValueExW(hKey, step.c_str(), NULL, NULL, (BYTE*)&val, &sz);
    if (ret) {
        WcaLog(LOGMSG_STANDARD, "Failed to query specific value for  install step %s %d", shortkey.c_str(), ret);
    }
    else {
        retval = (val == 0 ? false : true);
        WcaLog(LOGMSG_STANDARD, "install step %s: %d", shortkey.c_str(), retval);
    }
    RegCloseKey(hKey);
    return retval;
}
