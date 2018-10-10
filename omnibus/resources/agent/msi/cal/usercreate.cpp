#include "stdafx.h"

#pragma comment(lib, "shlwapi.lib")
#define MIN_PASS_LEN 12
#define MAX_PASS_LEN 18

bool generatePassword(wchar_t* passbuf, int passbuflen) {
    if (passbuflen < MAX_PASS_LEN + 1) {
        return false;
    }
#define RANDOM_BUFFER_SIZE 128
    unsigned char randbuf[RANDOM_BUFFER_SIZE];
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

    NTSTATUS ret = BCryptGenRandom(NULL, randbuf, RANDOM_BUFFER_SIZE, BCRYPT_USE_SYSTEM_PREFERRED_RNG);
    if (0 != ret) {
        WcaLog(LOGMSG_STANDARD, "Failed to generate random data for password %d\n", ret);
        return false;
    }
    // we'll do a random length password between 12 and 18 chars
    int len = (randbuf[0] % (MAX_PASS_LEN - MIN_PASS_LEN)) + MIN_PASS_LEN;
    int times = 0;

    do {
        int randbufindex = 0;
        memset(usedClasses, 0, sizeof(usedClasses));
        memset(passbuf, 0, sizeof(wchar_t) * (MAX_PASS_LEN + 1));
        NTSTATUS ret = BCryptGenRandom(NULL, randbuf, RANDOM_BUFFER_SIZE, BCRYPT_USE_SYSTEM_PREFERRED_RNG);
        if (0 != ret) {
            WcaLog(LOGMSG_STANDARD, "Failed to generate random data for password %d\n", ret);
            return false;
        }

        for (int i = 0; i < len && randbufindex < RANDOM_BUFFER_SIZE - 2; i++) {
            int chartype = randbuf[randbufindex++] % numtypes;

            int max_ndx = int(classlengths[chartype] - 1);
            int ndx = randbuf[randbufindex++] % max_ndx;

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
DWORD changeRegistryAcls(const wchar_t* name) {

    ExplicitAccess localsystem;
    localsystem.BuildGrantSid(TRUSTEE_IS_USER, GENERIC_ALL | KEY_ALL_ACCESS, SECURITY_LOCAL_SYSTEM_RID, 0);

    ExplicitAccess localAdmins;
    localAdmins.BuildGrantSid(TRUSTEE_IS_GROUP, GENERIC_ALL | KEY_ALL_ACCESS, SECURITY_BUILTIN_DOMAIN_RID, DOMAIN_ALIAS_RID_ADMINS);

    //ExplicitAccess suser;
    //suser.BuildGrantUser(secretUserUsername.c_str(), GENERIC_READ | GENERIC_EXECUTE | READ_CONTROL | KEY_READ);

    ExplicitAccess dduser;
    dduser.BuildGrantUser(ddAgentUserName.c_str(), GENERIC_ALL | KEY_ALL_ACCESS);


    WinAcl acl;
    acl.AddToArray(localsystem);
    //acl.AddToArray(suser);
    acl.AddToArray(localAdmins);
    acl.AddToArray(dduser);


    PACL newAcl = NULL;
    PACL oldAcl = NULL;
    DWORD ret = 0;
    // only want to set new acl info
    oldAcl = NULL;
    ret = acl.SetEntriesInAclW(oldAcl, &newAcl);

    ret = SetNamedSecurityInfoW((LPWSTR)name, SE_REGISTRY_KEY, DACL_SECURITY_INFORMATION, // | PROTECTED_DACL_SECURITY_INFORMATION,
        NULL, NULL, newAcl, NULL);

    if (0 != ret) {
        WcaLog(LOGMSG_STANDARD, "Failed to set named security info %d", ret);
    }
    return ret;

}

DWORD addDdUserPermsToFile(std::wstring &filename)
{
    if(!PathFileExistsW((LPCWSTR) filename.c_str()))
    {
        // return success; we don't need to do anything
        WcaLog(LOGMSG_STANDARD, "file doesn't exist, not doing anything");
        return 0;
    }
    ExplicitAccess dduser;
    dduser.BuildGrantUser(ddAgentUserName.c_str(), FILE_ALL_ACCESS, 
                          SUB_CONTAINERS_AND_OBJECTS_INHERIT);

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

void removeUserPermsFromFile(std::wstring &filename, PSID sidremove)
{
    if(!PathFileExistsW((LPCWSTR) filename.c_str()))
    {
        // return success; we don't need to do anything
        WcaLog(LOGMSG_STANDARD, "file doesn't exist, not doing anything");
        return ;
    }
    ExplicitAccess dduser;
    // get the current ACLs;  check to see if the DD user is in there, if so
    // remove
    std::string shortfile;
    toMbcs(shortfile, filename.c_str());
    DWORD dwRes = 0;
    PACL pOldDacl = NULL;
    PSECURITY_DESCRIPTOR pSD = NULL;
    ACL_SIZE_INFORMATION sizeInfo;
    memset(&sizeInfo, 0, sizeof(ACL_SIZE_INFORMATION));

    dwRes = GetNamedSecurityInfo(filename.c_str(), SE_FILE_OBJECT, 
          DACL_SECURITY_INFORMATION,
          NULL, NULL, &pOldDacl, NULL, &pSD);
    if (ERROR_SUCCESS != dwRes) {
        WcaLog(LOGMSG_STANDARD, "Failed to get file DACL, not removing user perms");
        return;
    }
    BOOL bRet = GetAclInformation(pOldDacl, (PVOID)&sizeInfo, sizeof(ACL_SIZE_INFORMATION), AclSizeInformation);
    if(FALSE == bRet) {
        WcaLog(LOGMSG_STANDARD, "Failed to get DACL size information");
        goto doneRemove;
    }
    for(int i = 0; i < sizeInfo.AceCount; i++) {
        ACCESS_ALLOWED_ACE *ace;

        if (GetAce(pOldDacl, i, (LPVOID*)&ace)) {
            PSID compareSid = (PSID)(&ace->SidStart);
            if (EqualSid(compareSid, sidremove)) {
                WcaLog(LOGMSG_STANDARD, "Matched sid on file %s, removing", shortfile.c_str());
                if (!DeleteAce(pOldDacl, i)) {
                    WcaLog(LOGMSG_STANDARD, "Failed to delete ACE on file %s", shortfile.c_str());
                }
            }
        }
    }
    dwRes = SetNamedSecurityInfoW((LPWSTR) filename.c_str(), SE_FILE_OBJECT, DACL_SECURITY_INFORMATION,
            NULL, NULL, pOldDacl, NULL);
    if(dwRes != 0) {
        WcaLog(LOGMSG_STANDARD, "%d resetting permissions on %s", dwRes, shortfile.c_str());
    }

doneRemove:

    if(pSD){
        LocalFree((HLOCAL) pSD);
    }
    
    return ;
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
        if (0 == ret) {
            MarkInstallStepComplete(strDdUserPasswordChanged);
        }
    } else if (ret != 0) {
        // failed with some unexpected reason
        WcaLog(LOGMSG_STANDARD, "Failed to create dd agent user");
        goto ddUserReturn;
    }
    else {
        // user was successfully create.  Store that in case we need to rollback
        WcaLog(LOGMSG_STANDARD, "Created DD agent user");
        MarkInstallStepComplete(strDdUserCreated);
    }
    // now store the password in the property so the installer can use it
    MsiSetProperty(hInstall, (LPCWSTR)ddAgentUserPasswordProperty.c_str(), (LPCWSTR)passbuf);

ddUserReturn:
    memset(passbuf, 0, (MAX_PASS_LEN + 2) * sizeof(wchar_t));
    return ret;
}


DWORD DeleteUser(std::wstring& name) {
    NET_API_STATUS ret = NetUserDel(NULL, name.c_str());
    return (DWORD)ret;
}

UINT doRemoveDDUser()
{
    UINT er = 0;
    LOCALGROUP_MEMBERS_INFO_0 lmi0;
    memset(&lmi0, 0, sizeof(LOCALGROUP_MEMBERS_INFO_3));
    PSID sid = NULL;
    LSA_HANDLE hLsa = NULL;
    DWORD nErr;
    // change the rights on this user
    sid = GetSidForUser(NULL, (LPCWSTR)ddAgentUserName.c_str());
    if (!sid) {
        goto LExit;
    }
    if ((hLsa = GetPolicyHandle()) == NULL) {
        goto LExit;
    }

    // remove it from the "performance monitor users" group
    lmi0.lgrmi0_sid = sid;
    nErr = NetLocalGroupDelMembers(NULL, L"Performance Monitor Users", 0, (LPBYTE)&lmi0, 1);
    if(nErr == NERR_Success) {
        WcaLog(LOGMSG_STANDARD, "Added ddagentuser to Performance Monitor Users");
    } else if (nErr == ERROR_NO_SUCH_MEMBER || nErr == ERROR_MEMBER_NOT_IN_ALIAS ) {
        WcaLog(LOGMSG_STANDARD, "User wasn't in group, continuing %d", nErr);
    } else {
        WcaLog(LOGMSG_STANDARD, "Unexpected error removing user from group %d", nErr);
    }

    if (!RemovePrivileges(sid, hLsa, SE_DENY_INTERACTIVE_LOGON_NAME)) {
        WcaLog(LOGMSG_STANDARD, "failed to remove deny interactive login right");
    }

    if (!RemovePrivileges(sid, hLsa, SE_DENY_NETWORK_LOGON_NAME)) {
        WcaLog(LOGMSG_STANDARD, "failed to remove deny network login right");
    }
    if (!RemovePrivileges(sid, hLsa, SE_DENY_REMOTE_INTERACTIVE_LOGON_NAME)) {
        WcaLog(LOGMSG_STANDARD, "failed to remove deny remote interactive login right");
    }
    if (!RemovePrivileges(sid, hLsa, SE_SERVICE_LOGON_NAME)) {
        WcaLog(LOGMSG_STANDARD, "failed to remove service login right");
    }

    // remove the dd user from the \programdata\ file permissions 
    removeUserPermsFromFile(logfilename, sid);
    removeUserPermsFromFile(datadogyamlfile, sid);
    removeUserPermsFromFile(confddir, sid);
    removeUserPermsFromFile(programdataroot, sid);
    
    // delete the auth token file entirely
    DeleteFile(authtokenfilename.c_str());

    er = DeleteUser(ddAgentUserName);
    if (0 != er) {
        // don't actually fail on failure.  We're doing an uninstall,
        // and failing will just leave the system in a more confused state
        WcaLog(LOGMSG_STANDARD, "Didn't delete the datadog user %d", er);
    } 
    
LExit:
    if (sid) {
        delete[](BYTE *) sid;
    }
    if (hLsa) {
        LsaClose(hLsa);
    }
    return er;
}
