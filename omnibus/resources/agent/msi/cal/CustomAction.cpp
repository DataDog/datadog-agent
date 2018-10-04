#include "stdafx.h"

static std::wstring secretUserUsername(L"datadog_secretuser");
static std::wstring secretUserDescription(L"DataDog user used to fetch secrets from KMS");
static std::wstring datadog_path = L"Datadog\\Datadog Agent";
static std::wstring datadog_key_root = L"SOFTWARE\\" + datadog_path;
static std::wstring datadog_key_secret_key = L"secrets";
static std::wstring datadog_key_secrets = L"SOFTWARE\\" + datadog_path + L"\\" + datadog_key_secret_key;
static std::wstring datadog_acl_key_secrets = L"MACHINE\\" + datadog_key_secrets;


int CreateUser(std::wstring& name, std::wstring& comment, bool writePassToReg = false);
DWORD DeleteUser(std::wstring& name);
DWORD DeleteSecretsRegKey();

#ifdef CA_CMD_TEST
#define LOGMSG_STANDARD 0
void WcaLog(int type, const char * fmt...)
{
	va_list args;
	va_start(args, fmt);
	vprintf(fmt, args);
	va_end(args);
	printf("\n");
}
#else
extern "C" UINT __stdcall AddDatadogSecretUser(
	MSIHANDLE hInstall
	)
{
	HRESULT hr = S_OK;
	UINT er = ERROR_SUCCESS;

	// that's helpful.  WcaInitialize Log header silently limited to 32 chars 
	hr = WcaInitialize(hInstall, "CA: AddDatadogSecretUser");
	// ExitOnFailure macro includes a goto LExit if hr has a failure.
	ExitOnFailure(hr, "Failed to initialize");

	WcaLog(LOGMSG_STANDARD, "Initialized.");

	er = CreateUser(secretUserUsername, secretUserDescription, true);
	if (0 != er) {
		hr = -1;
	}

LExit:
	er = SUCCEEDED(hr) ? ERROR_SUCCESS : ERROR_INSTALL_FAILURE;
	return WcaFinalize(er);
}

extern "C" UINT __stdcall RemoveDatadogSecretUser(MSIHANDLE hInstall) {
	HRESULT hr = S_OK;
	UINT er = ERROR_SUCCESS;

	// that's helpful.  WcaInitialize Log header silently limited to 32 chars
	hr = WcaInitialize(hInstall, "CA: RemoveDatadogSecretUser");
	ExitOnFailure(hr, "Failed to initialize");

	WcaLog(LOGMSG_STANDARD, "Initialized.");

	er = DeleteUser(secretUserUsername);
	if (0 != er) {
		hr = -1;
		goto LExit;
	} 
	er = DeleteSecretsRegKey();
	if (0 != er) {
		hr = -1;
		goto LExit;
	}


LExit:
	er = SUCCEEDED(hr) ? ERROR_SUCCESS : ERROR_INSTALL_FAILURE;
	return WcaFinalize(er);

}



// DllMain - Initialize and cleanup WiX custom action utils.
extern "C" BOOL WINAPI DllMain(
	__in HINSTANCE hInst,
	__in ULONG ulReason,
	__in LPVOID
	)
{
	switch(ulReason)
	{
	case DLL_PROCESS_ATTACH:
		WcaGlobalInitialize(hInst);
		// initialize random number generator
		srand(GetTickCount());
		break;

	case DLL_PROCESS_DETACH:
		WcaGlobalFinalize();
		break;
	}

	return TRUE;
}
#endif // CA_CMD_TEST
#define MIN_PASS_LEN 12
#define MAX_PASS_LEN 18

bool GeneratePassword(wchar_t* passbuf, int passbuflen) {
	if(passbuflen < MAX_PASS_LEN + 1) {
        return false;
    }
	
	const wchar_t * availLower = L"abcdefghijklmnopqrstuvwxyz";
	const wchar_t * availUpper = L"ABCDEFGHIJKLMNOPQRSTUVWXYZ"; 
	const wchar_t * availNum = L"1234567890";
	const wchar_t * availSpec = L"()`~!@#$%^&*-+=|{}[]:;'<>,.?/";

#define CHARTYPE_LOWER 0
#define CHARTYPE_UPPER 1
#define CHARTYPE_NUMBER 2
#define CHARTYPE_SPECIAL 3
	const wchar_t * classes[] = {
		availLower,
		availUpper,
		availNum,
		availSpec,
	};
	size_t classlengths[] = {
		wcslen(availLower),
		wcslen(availUpper),
		wcslen(availNum),
		wcslen(availSpec)
	};
	int numtypes = sizeof(classes) / sizeof(wchar_t*);

	int usedClasses[] = { 0, 0, 0, 0 };

	// we'll do a random length password between 12 and 18 chars
	int len = (rand() % (MAX_PASS_LEN - MIN_PASS_LEN)) + MIN_PASS_LEN;
	int times = 0;
	do {
		memset(usedClasses, 0, sizeof(usedClasses));
		memset(passbuf, 0, sizeof(wchar_t) * (MAX_PASS_LEN + 1));
		for (int i = 0; i < len; i++) {
			int chartype = rand() % numtypes;

			int max_ndx = int(classlengths[chartype] - 1);
			int ndx = rand() % max_ndx;

			passbuf[i] = classes[chartype][ndx];
			usedClasses[chartype]++;
		}
		times++;
	} while ((usedClasses[CHARTYPE_LOWER] < 2 || usedClasses[CHARTYPE_UPPER] < 2 || 
		      usedClasses[CHARTYPE_NUMBER] < 2 || usedClasses[CHARTYPE_SPECIAL] < 2) ||
		    ((usedClasses[CHARTYPE_LOWER] + usedClasses[CHARTYPE_UPPER]) < 
		     (usedClasses[CHARTYPE_NUMBER] + usedClasses[CHARTYPE_SPECIAL])));

	WcaLog(LOGMSG_STANDARD, "Took %d passes to generate the password", times);

	return true;

}
bool createRegistryKey() {
	LSTATUS status = 0;
	HKEY hKey;
	status = RegCreateKeyExW(HKEY_LOCAL_MACHINE,
		datadog_key_secrets.c_str(),
		0, // reserved is zero
		NULL, // class is null
		0, // no options
		KEY_ALL_ACCESS,
		NULL, // default security descriptor (we'll change this later)
		&hKey,
		NULL); // don't care about disposition... 
	if (ERROR_SUCCESS != status) {
		WcaLog(LOGMSG_STANDARD, "Couldn't create/open datadog reg key %d", GetLastError());
		return false;
	}
	RegCloseKey(hKey);
	return true;
}
bool writePasswordToRegistry(const wchar_t * name, const wchar_t* pass) {
	// RegCreateKey opens the key if it's there.
	LSTATUS status = 0;
	HKEY hKey;
	status = RegCreateKeyExW(HKEY_LOCAL_MACHINE,
		datadog_key_secrets.c_str(),
		0, // reserved is zero
		NULL, // class is null
		0, // no options
		KEY_ALL_ACCESS,
		NULL, // default security descriptor (we'll change this later)
		&hKey,
		NULL); // don't care about disposition... 
	if (ERROR_SUCCESS != status) {
		WcaLog(LOGMSG_STANDARD, "Couldn't create/open datadog reg key %d", GetLastError());
		return false;
	}
	status = RegSetValueExW(hKey,
		name,
		0, // must be zero
		REG_SZ,
		(const BYTE*)pass,
		DWORD((wcslen(pass) + 1)) * sizeof(wchar_t));
	RegCloseKey(hKey);
	return status == 0;

}
DWORD changeRegistryAcls(const wchar_t* name) {

	ExplicitAccess localsystem;
	localsystem.BuildGrantSid(TRUSTEE_IS_USER, GENERIC_ALL | KEY_ALL_ACCESS, SECURITY_LOCAL_SYSTEM_RID, 0);

	ExplicitAccess localAdmins;
	localAdmins.BuildGrantSid(TRUSTEE_IS_GROUP,  GENERIC_ALL | KEY_ALL_ACCESS, SECURITY_BUILTIN_DOMAIN_RID, DOMAIN_ALIAS_RID_ADMINS);
	
	ExplicitAccess suser;
	suser.BuildGrantUser(secretUserUsername.c_str(), GENERIC_READ | GENERIC_EXECUTE | READ_CONTROL | KEY_READ);

	WinAcl acl;
	acl.AddToArray(localsystem);
	acl.AddToArray(suser);
	acl.AddToArray(localAdmins);


	PACL newAcl = NULL;
	PACL oldAcl = NULL;
	DWORD ret = 0;
	// only want to set new acl info
	oldAcl = NULL;
	ret = acl.SetEntriesInAclW(oldAcl, &newAcl);

	ret = SetNamedSecurityInfoW((LPWSTR) name, SE_REGISTRY_KEY, DACL_SECURITY_INFORMATION | PROTECTED_DACL_SECURITY_INFORMATION,
		NULL, NULL, newAcl, NULL);

	if (0 != ret) {
		WcaLog(LOGMSG_STANDARD, "Failed to set named securit info %d", ret);
	}
	return ret;

}
int CreateUser(std::wstring& name, std::wstring& comment, bool writePassToReg) {
	USER_INFO_1 ui;
    wchar_t passbuf[MAX_PASS_LEN + 2];
    if ( !GeneratePassword(passbuf, MAX_PASS_LEN + 2)) {
        WcaLog(LOGMSG_STANDARD, "Failed to generate password");
        return -1;
    }
	memset(&ui, 0, sizeof(USER_INFO_1));
	WcaLog(LOGMSG_STANDARD, "entered createuser");
	ui.usri1_name = (LPWSTR) name.c_str();
	ui.usri1_password = passbuf;
	ui.usri1_priv = USER_PRIV_USER;
	ui.usri1_comment = (LPWSTR) comment.c_str();
	ui.usri1_flags = UF_DONT_EXPIRE_PASSWD;
	DWORD ret = 0;

	WcaLog(LOGMSG_STANDARD, "Calling NetUserAdd.");
	ret = NetUserAdd(NULL, // LOCAL_MACHINE
		1, // indicates we're using a USER_INFO_1
		(LPBYTE)&ui,
		NULL);
	WcaLog(LOGMSG_STANDARD, "NetUserAdd. %d", ret);
	if (ret != 0) {
		WcaLog(LOGMSG_STANDARD, "Create User failed %d", (int)ret);
        goto clearAndReturn;
	} 
	
    WcaLog(LOGMSG_STANDARD, "Successfully created user");
    if (writePassToReg) {

        // create the top level key HKLM\Software\Datadog Agent\secrets.  Key must be
        // created to change the ACLS.
        if (!createRegistryKey()) {
            WcaLog(LOGMSG_STANDARD, "Failed to create secret storage key");
            goto clearAndReturn;
        }
        
        // if we write the password to the registry,
        // change the ownership so that only LOCAL_SYSTEM and
        // the user itself can read it

        // of course, the security APIs use a different format than
        // the registry APIs
        ret = changeRegistryAcls(datadog_acl_key_secrets.c_str());
        if (0 == ret) {
            WcaLog(LOGMSG_STANDARD, "Changed registry perms");
        }
        else {
            WcaLog(LOGMSG_STANDARD, "Failed to change registry perms %d", ret);
            goto clearAndReturn;
        }

        // now that the ACLS are changed on the containing key, write
        // the password into it.
        writePasswordToRegistry(name.c_str(), ui.usri1_password);
    }

clearAndReturn:
	// clear the password so it's not sitting around in memory
	memset(passbuf, 0, (MAX_PASS_LEN + 2)* sizeof(wchar_t));
	return (int)ret;

}

DWORD DeleteUser(std::wstring& name) {
	NET_API_STATUS ret = NetUserDel(NULL, name.c_str());
	return (DWORD)ret;
}

DWORD DeleteSecretsRegKey() {
	HKEY hKey = NULL;
	DWORD ret = RegOpenKeyEx(HKEY_LOCAL_MACHINE, datadog_key_root.c_str(), 0, KEY_ALL_ACCESS, &hKey);
	if (ERROR_SUCCESS != ret) {
		WcaLog(LOGMSG_STANDARD, "Failed to open registry key for deletion %d", ret);
		return ret;
	}
	ret = RegDeleteKeyEx(hKey, datadog_key_secret_key.c_str(), KEY_WOW64_64KEY, 0);
	if (ERROR_SUCCESS != ret) {
		WcaLog(LOGMSG_STANDARD, "Failed to delete secret key %d", ret);
	}
	RegCloseKey(hKey);
	return ret;
}
