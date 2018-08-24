#include "stdafx.h"

#pragma comment(lib, "shlwapi.lib")
#define MIN_PASS_LEN 12
#define MAX_PASS_LEN 18

bool generatePassword(wchar_t* passbuf, int passbuflen) {
    if (passbuflen < MAX_PASS_LEN + 1) {
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
    localAdmins.BuildGrantSid(TRUSTEE_IS_GROUP, GENERIC_ALL | KEY_ALL_ACCESS, SECURITY_BUILTIN_DOMAIN_RID, DOMAIN_ALIAS_RID_ADMINS);


    WinAcl acl;
    acl.AddToArray(localsystem);
    acl.AddToArray(localAdmins);
#ifdef _ADD_DD_USER
    ExplicitAccess dduser;
    dduser.BuildGrantUser(ddAgentUserName.c_str(), GENERIC_ALL | KEY_ALL_ACCESS);
    acl.AddToArray(dduser);
#endif


    PACL newAcl = NULL;
    PACL oldAcl = NULL;
    DWORD ret = 0;
    // only want to set new acl info
    oldAcl = NULL;
    ret = acl.SetEntriesInAclW(oldAcl, &newAcl);

    ret = SetNamedSecurityInfoW((LPWSTR)name, SE_REGISTRY_KEY, DACL_SECURITY_INFORMATION | PROTECTED_DACL_SECURITY_INFORMATION,
        NULL, NULL, newAcl, NULL);

    if (0 != ret) {
        WcaLog(LOGMSG_STANDARD, "Failed to set named securit info %d", ret);
    }
    return ret;

}

DWORD addDdUserPermsToFile(std::wstring filename)
{
    if(!PathFileExistsW((LPCWSTR) filename.c_str()))
    {
        // return success; we don't need to do anything
        WcaLog(LOGMSG_STANDARD, "file doesn't exist, not doing anything");
        return 0;
    }
    ExplicitAccess dduser;
    dduser.BuildGrantUser(ddAgentUserName.c_str(), FILE_ALL_ACCESS);

    // get the current ACLs and append, rather than just set; if the file exists,
    // the user may have already set custom ACLs on the file, and we don't want
    // to disrupt that

    DWORD dwRes = 0;
    PACL pOldDACL = NULL, pNewDACL = NULL;
    PSECURITY_DESCRIPTOR pSD = NULL;
    WinAcl acl;
    acl.AddToArray(dduser);

    dwRes = GetNamedSecurityInfo(filename.c_str(), SE_FILE_OBJECT, 
          DACL_SECURITY_INFORMATION,
          NULL, NULL, &pOldDACL, NULL, &pSD);
    if (ERROR_SUCCESS == dwRes) {
        dwRes = acl.SetEntriesInAclW(pOldDACL, &pNewDACL);
        if(dwRes == 0) {
            dwRes = SetNamedSecurityInfoW((LPWSTR) filename.c_str(), SE_FILE_OBJECT, DACL_SECURITY_INFORMATION,
            NULL, NULL, pNewDACL, NULL);
        } else {
            WcaLog(LOGMSG_STANDARD, "%d setting entries in acl", dwRes);    
        }
    } else {
        WcaLog(LOGMSG_STANDARD, "%d getting existing perms", dwRes);
    }
    if(pSD){
        LocalFree((HLOCAL) pSD);
    }
    if(pNewDACL) {
        LocalFree((HLOCAL) pNewDACL);
    }
    return dwRes;
}

int doCreateUser(std::wstring& name, std::wstring& comment, const wchar_t* passbuf)
{
    USER_INFO_1 ui;
    memset(&ui, 0, sizeof(USER_INFO_1));
    WcaLog(LOGMSG_STANDARD, "entered createuser");
    ui.usri1_name = (LPWSTR)name.c_str();
    ui.usri1_password = (LPWSTR)passbuf;
    ui.usri1_priv = USER_PRIV_USER;
    ui.usri1_comment = (LPWSTR)comment.c_str();
    ui.usri1_flags = UF_DONT_EXPIRE_PASSWD;
    DWORD ret = 0;

    WcaLog(LOGMSG_STANDARD, "Calling NetUserAdd.");
    ret = NetUserAdd(NULL, // LOCAL_MACHINE
        1, // indicates we're using a USER_INFO_1
        (LPBYTE)&ui,
        NULL);
    WcaLog(LOGMSG_STANDARD, "NetUserAdd. %d", ret);
    return ret;

}

int CreateDDUser(MSIHANDLE hInstall)
{
    wchar_t passbuf[MAX_PASS_LEN + 2];
    if (!generatePassword(passbuf, MAX_PASS_LEN + 2)) {
        WcaLog(LOGMSG_STANDARD, "Failed to generate password");
        return -1;
    }
    int ret = doCreateUser(ddAgentUserName, ddAgentUserDescription, passbuf);
    if (ret == NERR_UserExists) {
        WcaLog(LOGMSG_STANDARD, "Attempting to reset password of existing user");
        // if the user exists, update the password with the newly generated
        // password.  We need to update the password on every install, b/c the
        // service registration code runs on every upgrade, and we need to know
        // the password.  Rather than store the password, just generate a new
        // one and use that
        USER_INFO_1003 newPassword;
        newPassword.usri1003_password = passbuf;
        ret = NetUserSetInfo(NULL, // always local server
            ddAgentUserName.c_str(),
            1003, // according to the docs there's no constant
            (LPBYTE)&newPassword,
            NULL);
    } else if (ret != 0) {
        // failed with some unexpected reason
        WcaLog(LOGMSG_STANDARD, "Failed to create dd agent user");
        goto ddUserReturn;
    }
    else {
        // user was successfully create.  Store that in case we need to rollback
        MsiSetProperty(hInstall, propertyDDUserCreated.c_str(), L"true");
    }
    // now store the password in the property so the installer can use it
    MsiSetProperty(hInstall, (LPCWSTR)ddAgentUserPasswordProperty.c_str(), (LPCWSTR)passbuf);

ddUserReturn:
    memset(passbuf, 0, (MAX_PASS_LEN + 2) * sizeof(wchar_t));
    return ret;
}
int CreateSecretUser(MSIHANDLE hInstall, std::wstring& name, std::wstring& comment)
{

    wchar_t passbuf[MAX_PASS_LEN + 2];
    bool doWritePassToReg = true;
    if (!generatePassword(passbuf, MAX_PASS_LEN + 2)) {
        WcaLog(LOGMSG_STANDARD, "Failed to generate password");
        return -1;
    }
    int ret = doCreateUser(name, comment, passbuf);
    if (ret == NERR_UserExists) {
        // user is already present. Assume this is an upgrade, in
        // which case the password is alreadyset and stored.
        WcaLog(LOGMSG_STANDARD, "Datadog secret user exists... upgrade?");

        // don't write the password later (but go ahead and rewrite
        // the permissions)
        doWritePassToReg = false;
    }
    else if (ret != 0) {
        WcaLog(LOGMSG_STANDARD, "Create User failed %d", (int)ret);
        goto clearAndReturn;
    } else {
        // note we created the user in case the install fails later
        MsiSetProperty(hInstall, propertySecretUserCreated.c_str(), L"true");
        WcaLog(LOGMSG_STANDARD, "Successfully created user");
    }

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
    if (doWritePassToReg) {
        if (writePasswordToRegistry(name.c_str(), passbuf)) {
            MsiSetProperty(hInstall, propertySecretPasswordWritten.c_str(), L"true");
        }
    }

clearAndReturn:
    // clear the password so it's not sitting around in memory
    memset(passbuf, 0, (MAX_PASS_LEN + 2) * sizeof(wchar_t));
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

