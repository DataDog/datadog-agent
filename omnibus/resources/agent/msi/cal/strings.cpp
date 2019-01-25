#include "stdafx.h"

std::wstring datadog_path = L"Datadog\\Datadog Agent";
std::wstring datadog_key_root = L"SOFTWARE\\" + datadog_path;
std::wstring datadog_acl_key_datadog = L"MACHINE\\SOFTWARE\\" + datadog_path;
std::wstring installStepsKey = datadog_key_root + L"\\installSteps";
std::wstring datadog_service_name(L"DataDog Agent");

std::wstring ddAgentUserName(L".\\ddagentuser");
std::wstring ddAgentUserNameUnqualified;
std::wstring ddAgentUserDomain;
const wchar_t *ddAgentUserDomainPtr = NULL;
std::wstring ddAgentUserDescription(L"User context under which the DataDog Agent service runs");

std::wstring traceService(L"datadog-trace-agent");
std::wstring processService(L"datadog-process-agent");
std::wstring agentService(L"datadogagent");

std::wstring propertyDDUserCreated(L"DDUSERCREATED");
std::wstring propertyDDAgentUserName(L"DDAGENTUSER_NAME");
std::wstring propertyDDAgentUserPassword(L"DDAGENTUSER_PASSWORD");
std::wstring propertyEnableServicesDeferredKey(L"enableservices");
std::wstring propertyRollbackState(L"CustomActionData");
std::wstring propertyCustomActionData(L"CustomActionData");

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
    std::string propertyname;
    std::string propval;
    toMbcs(propertyname, propertyName);

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
    if (wcslen(szValueBuf) == 0){
        WcaLog(LOGMSG_STANDARD, "Property %s is empty", propertyname.c_str());
        delete [] szValueBuf;
        return false;
    }
    *dst=szValueBuf;
    *len = cchValueBuf;
    toMbcs(propval, szValueBuf);
    WcaLog(LOGMSG_STANDARD, "loaded property %s = %s", propertyname.c_str(), propval.c_str());

    
    return true;
}

bool loadDdAgentUserName(MSIHANDLE hInstall, LPCWSTR propertyName ) {
    std::wstring tmpName;
    if(loadPropertyString(hInstall, propertyName ? propertyName : propertyDDAgentUserName.c_str(), tmpName)){
        if(std::wstring::npos == tmpName.find(L'\\')) {
            WcaLog(LOGMSG_STANDARD, "loaded username doesn't have domain specifier, assuming local");
            ddAgentUserName = L".\\" + tmpName;
        } else {
            ddAgentUserName = tmpName;
        }
        // now create the splits between the domain and user for all to use, too
        std::wstring domain, user;
        std::wistringstream asStream(tmpName);
        // username is going to be of the form <domain>\<username>
        // if the <domain> is ".", then just do local machine
        getline(asStream, ddAgentUserDomain, L'\\');
        getline(asStream, ddAgentUserNameUnqualified, L'\\');
        if(domain == L"."){
            ddAgentUserDomainPtr = NULL;
        } else {
            ddAgentUserDomainPtr = ddAgentUserDomain.c_str();
        }

        return true;
    }
    return false;
}

bool loadDdAgentPassword(MSIHANDLE hInstall, wchar_t **pass, DWORD *len) {
    return loadPropertyString(hInstall, propertyDDAgentUserPassword.c_str(), pass, len);
}
