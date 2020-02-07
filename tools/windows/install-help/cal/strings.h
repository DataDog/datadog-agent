#pragma once
extern std::wstring datadog_path;
extern std::wstring datadog_key_root;
extern std::wstring datadog_acl_key_datadog;
extern std::wstring datadog_service_name;

extern std::wstring ddAgentUserName;
extern std::wstring ddAgentUserNameUnqualified;
extern std::wstring ddAgentUserDomain;
extern const wchar_t *ddAgentUserDomainPtr;

extern std::wstring ddAgentUserDescription;

extern std::wstring traceService;
extern std::wstring processService;
extern std::wstring agentService;

extern std::wstring propertyDDAgentUserName;
extern std::wstring propertyDDAgentUserPassword;
extern std::wstring propertyDDUserCreated;
extern std::wstring propertyEnableServicesDeferredKey;
extern std::wstring propertyRollbackState;
extern std::wstring propertyCustomActionData;

extern std::wstring programdataroot;
extern std::wstring logfilename;
extern std::wstring authtokenfilename;
extern std::wstring datadogyamlfile;
extern std::wstring confddir;
extern std::wstring logdir;
extern std::wstring installdir;
extern std::wstring embedded2Dir;
extern std::wstring embedded3Dir;

extern std::wstring strRollbackKeyName;
extern std::wstring strUninstallKeyName;

extern std::wstring agent_exe;
extern std::wstring trace_exe;
extern std::wstring process_exe;

extern std::wstring computername;
extern std::wstring domainname; // if domain joined, workgroup name otherwise
extern bool isDomainJoined;

// installation steps
extern std::wstring installCreatedDDUser;
extern std::wstring installCreatedDDDomain;
extern std::wstring installInstalledServices;

void initializeStringsFromStringTable();


bool loadDdAgentUserName(MSIHANDLE hInstall, LPCWSTR propertyName = NULL);
bool loadPropertyString(MSIHANDLE hInstall, LPCWSTR propertyName, std::wstring& dst);
bool loadPropertyString(MSIHANDLE hInstall, LPCWSTR propertyName, wchar_t **dst, DWORD *len);
bool loadDdAgentPassword(MSIHANDLE hInstall, wchar_t **dst, DWORD *len);

#define MAX_CUSTOM_PROPERTY_SIZE        128
