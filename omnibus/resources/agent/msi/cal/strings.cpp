#include "stdafx.h"

std::wstring datadog_path = L"Datadog\\Datadog Agent";
std::wstring datadog_key_root = L"SOFTWARE\\" + datadog_path;
std::wstring datadog_acl_key_datadog = L"MACHINE\\SOFTWARE\\" + datadog_path;
std::wstring installStepsKey = datadog_key_root + L"\\installSteps";
std::wstring datadog_service_name(L"DataDog Agent");

std::wstring ddAgentUserName(L"ddagentuser");
std::wstring ddAgentUserPasswordProperty(L"DDAGENTUSER_PASSWORD");
std::wstring ddAgentUserDescription(L"User context under which the DataDog Agent service runs");

std::wstring traceService(L"datadog-trace-agent");
std::wstring processService(L"datadog-process-agent");
std::wstring agentService(L"datadogagent");

std::wstring propertyDDUserCreated(L"DDUSERCREATED");
std::wstring propertyDDAgentUserName(L"DDAGENTUSER_NAME");
std::wstring propertyRollbackState(L"CustomActionData");

std::wstring programdataroot(L"c:\\ProgramData\\DataDog\\");
std::wstring logfilename(L"c:\\ProgramData\\DataDog\\logs\\agent.log");
std::wstring authtokenfilename(L"c:\\ProgramData\\Datadog\\auth_token");
std::wstring datadogyamlfile(L"c:\\ProgramData\\Datadog\\datadog.yaml");
std::wstring confddir(L"c:\\ProgramData\\Datadog\\conf.d");
std::wstring logdir(L"c:\\ProgramData\\Datadog\\logs");

// installation steps
std::wstring strDdUserCreated(L"00-ddUserCreated");
std::wstring strDdUserPasswordChanged(L"01-ddUserPasswordChanged");
std::wstring strFilePermissionsChanged(L"02-ddUserFilePermsChanged");
std::wstring strAddDdUserToPerfmon(L"03-ddUserAddedToPerfmon");
std::wstring strChangedRegistryPermissions(L"04-ddRegPermsChanged");

void toMbcs(std::string& target, LPCWSTR src) {
    size_t len = wcslen(src);
    size_t narrowlen = (2 * len) + 1;
    char * tgt = new char[narrowlen];
    wcstombs(tgt, src, narrowlen);
    target = tgt;
    delete[] tgt;
    return;
}

bool loadPropertyString(MSIHANDLE hInstall, LPCWSTR propertyName, std::wstring& dststr)
{
    wchar_t *dst = NULL;
    DWORD len = 0;
    if(loadPropertyString(hInstall, propertyName, &dst, &len)) {
        dststr = dst;
        delete [] dst;
        return true;
    }
    return false;
}

bool loadPropertyString(MSIHANDLE hInstall, LPCWSTR propertyName, wchar_t **dst, DWORD *len)
{
    TCHAR* szValueBuf = NULL;
    DWORD cchValueBuf = 0;
    UINT uiStat =  MsiGetProperty(hInstall, propertyName, L"", &cchValueBuf);
    //cchValueBuf now contains the size of the property's string, without null termination
    if (ERROR_MORE_DATA == uiStat)
    {
        ++cchValueBuf; // add 1 for null termination
        szValueBuf = new wchar_t[cchValueBuf];
        if (szValueBuf)
        {
            uiStat = MsiGetProperty(hInstall, propertyName, szValueBuf, &cchValueBuf);
        }
    }
    if (ERROR_SUCCESS != uiStat)
    {
        if (szValueBuf != NULL) 
           delete[] szValueBuf;
        WcaLog(LOGMSG_STANDARD, "failed to get  property");
        return false;
    }
    *dst=szValueBuf;
    *len = cchValueBuf;
    std::string propertyname;
    std::string propval;
    toMbcs(propertyname, propertyName);
    toMbcs(propval, szValueBuf);
    WcaLog(LOGMSG_STANDARD, "loaded property %s = %s", propertyname.c_str(), propval.c_str());

    
    return ERROR_SUCCESS;
}

bool loadDdAgentUserName(MSIHANDLE hInstall) {
    return loadPropertyString(hInstall, propertyDDAgentUserName.c_str(), ddAgentUserName);
}

bool loadDdAgentPassword(MSIHANDLE hInstall, wchar_t **pass, DWORD *len) {
    return loadPropertyString(hInstall, ddAgentUserPasswordProperty.c_str(), pass, len);
}
