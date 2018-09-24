#pragma once
extern std::wstring secretUserUsername;
extern std::wstring secretUserDescription;
extern std::wstring datadog_path;
extern std::wstring datadog_key_root;
extern std::wstring datadog_key_secret_key;
extern std::wstring datadog_key_secrets;
extern std::wstring datadog_acl_key_secrets;
extern std::wstring datadog_acl_key_datadog;
extern std::wstring datadog_service_name;

extern std::wstring ddAgentUserName;
extern std::wstring ddAgentUserPasswordProperty;
extern std::wstring ddAgentUserDescription;

extern std::wstring traceService;
extern std::wstring processService;

extern std::wstring propertyDDUserCreated;
extern std::wstring propertySecretUserCreated;
extern std::wstring propertySecretPasswordWritten;
extern std::wstring propertyRollbackState;

extern std::wstring logfilename;
extern std::wstring authtokenfilename;
extern std::wstring datadogyamlfile;

void toMbcs(std::string& target, LPCWSTR src);

#define MAX_CUSTOM_PROPERTY_SIZE        128