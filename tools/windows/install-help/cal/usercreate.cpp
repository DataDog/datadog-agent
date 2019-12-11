#include "stdafx.h"

#pragma comment(lib, "shlwapi.lib")


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
DWORD changeRegistryAcls(CustomActionData& data, const wchar_t* name) {

    WcaLog(LOGMSG_STANDARD, "Changing registry ACL on %S", name);
    ExplicitAccess localsystem;
    localsystem.BuildGrantSid(TRUSTEE_IS_USER, GENERIC_ALL | KEY_ALL_ACCESS, SECURITY_LOCAL_SYSTEM_RID, 0);

    ExplicitAccess localAdmins;
    localAdmins.BuildGrantSid(TRUSTEE_IS_GROUP, GENERIC_ALL | KEY_ALL_ACCESS, SECURITY_BUILTIN_DOMAIN_RID, DOMAIN_ALIAS_RID_ADMINS);

    //ExplicitAccess suser;
    //suser.BuildGrantUser(secretUserUsername.c_str(), GENERIC_READ | GENERIC_EXECUTE | READ_CONTROL | KEY_READ);

    PSID  usersid = GetSidForUser(NULL, data.Username().c_str());
    ExplicitAccess dduser;
    dduser.BuildGrantUser((SID *)usersid, GENERIC_ALL | KEY_ALL_ACCESS,
        SUB_CONTAINERS_AND_OBJECTS_INHERIT);


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

DWORD addDdUserPermsToFile(CustomActionData& data, std::wstring &filename)
{

    if(!PathFileExistsW((LPCWSTR) filename.c_str()))
    {
        // return success; we don't need to do anything
        WcaLog(LOGMSG_STANDARD, "file %S doesn't exist, not doing anything", filename.c_str());
        return 0;
    }
    WcaLog(LOGMSG_STANDARD, "Changing file permissions on %S", filename.c_str());
    PSID  usersid = GetSidForUser(NULL, data.Username().c_str());
    ExplicitAccess dduser;
    dduser.BuildGrantUser((SID *)usersid, FILE_ALL_ACCESS,
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
            dwRes = SetNamedSecurityInfoW((LPWSTR) filename.c_str(), SE_FILE_OBJECT, 
            DACL_SECURITY_INFORMATION | PROTECTED_DACL_SECURITY_INFORMATION,
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
    for(DWORD i = 0; i < sizeInfo.AceCount; i++) {
        ACCESS_ALLOWED_ACE *ace;

        if (GetAce(pOldDacl, i, (LPVOID*)&ace)) {
            PSID compareSid = (PSID)(&ace->SidStart);
            if (EqualSid(compareSid, sidremove)) {
                WcaLog(LOGMSG_STANDARD, "Matched sid on file %S, removing", filename.c_str());
                if (!DeleteAce(pOldDacl, i)) {
                    WcaLog(LOGMSG_STANDARD, "Failed to delete ACE on file %S", filename.c_str());
                }
            }
        }
    }
    dwRes = SetNamedSecurityInfoW((LPWSTR) filename.c_str(), SE_FILE_OBJECT, DACL_SECURITY_INFORMATION,
            NULL, NULL, pOldDacl, NULL);
    if(dwRes != 0) {
        WcaLog(LOGMSG_STANDARD, "%d resetting permissions on %S", dwRes, filename.c_str());
    }

doneRemove:

    if(pSD){
        LocalFree((HLOCAL) pSD);
    }
    
    return ;
}

int doCreateUser(const std::wstring& name, const std::wstring& comment, const wchar_t* passbuf)
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

int doSetUserPassword(const std::wstring& name,  const wchar_t* passbuf)
{
    USER_INFO_1003 ui;
    memset(&ui,0, sizeof(USER_INFO_1003));
    ui.usri1003_password = (LPWSTR)passbuf;
    DWORD ret = NetUserSetInfo(NULL, name.c_str(), 1003, (LPBYTE)&ui, NULL);
    WcaLog(LOGMSG_STANDARD, "NetUserSetInfo Change Password %d", ret);
    return ret;
}
DWORD DeleteUser(const wchar_t* host, const wchar_t* name){
    NET_API_STATUS ret = NetUserDel(NULL, name);
    return (DWORD)ret;
}



bool isDomainController()
{
    bool ret = false;
    DWORD status = 0;
    SERVER_INFO_101 *si = NULL;
    DWORD le = 0;
    status = NetServerGetInfo(NULL, 101, (LPBYTE *)&si);
    if (NERR_Success != status) {
        le = GetLastError();
        WcaLog(LOGMSG_STANDARD, "Failed to get server info");
        return false;
    }
    if (SV_TYPE_WORKSTATION & si->sv101_type) {
        WcaLog(LOGMSG_STANDARD, "machine is type SV_TYPE_WORKSTATION");
    }
    if (SV_TYPE_SERVER & si->sv101_type) {
        WcaLog(LOGMSG_STANDARD, "machine is type SV_TYPE_SERVER\n");
    }
    if (SV_TYPE_DOMAIN_CTRL & si->sv101_type) {
        WcaLog(LOGMSG_STANDARD, "machine is type SV_TYPE_DOMAIN_CTRL\n");
        ret = true;
    }
    if (SV_TYPE_DOMAIN_BAKCTRL & si->sv101_type) {
        WcaLog(LOGMSG_STANDARD, "machine is type SV_TYPE_DOMAIN_BAKCTRL\n");
        ret = true;
    }
    if (si) {
        NetApiBufferFree((LPVOID)si);
    }
    return ret;
}

int doesUserExist(const CustomActionData& data, bool isDC)
{
    int retval = 0;
    SID *newsid = NULL;
    DWORD cbSid = 0;
    LPWSTR refDomain = NULL;
    DWORD cchRefDomain = 0;
    SID_NAME_USE use;
    DWORD err = 0;
    const wchar_t * userToTry = data.Username().c_str();
    const wchar_t * hostToTry = NULL;

    BOOL bRet = LookupAccountName(NULL, userToTry, newsid, &cbSid, refDomain, &cchRefDomain, &use);
    if (bRet) {
        err = GetLastError();
        // this should *never* happen, because we didn't pass in a buffer large enough for
        // the sid or the domain name.
        WcaLog(LOGMSG_STANDARD, "doesUserExist: Lookup Account Name: Unexpected error %d 0x%x", err, err);
        return -1;
    }
    err = GetLastError();
    if (ERROR_NONE_MAPPED == err) {
        // this user doesn't exist.  We're done
        return 0;
    }
    if (ERROR_INSUFFICIENT_BUFFER != err) {
        if (!isDC) {
            // can only try this if we're not on a primary/backup DC; on DCs we must
            // be able to contact the domain authority.  
            if (err >= ERROR_NO_TRUST_LSA_SECRET && err <= ERROR_TRUST_FAILURE) {
                WcaLog(LOGMSG_STANDARD, "Can't reach domain controller %d", err);
                // if the user specified a domain, then also must be able to contact
                // the domain authority
                if (data.isUserLocalUser() == NULL) {
                    WcaLog(LOGMSG_STANDARD, "trying fully qualified local account");
                    bRet = LookupAccountName(computername.c_str(), data.Username().c_str(), newsid, &cbSid, refDomain, &cchRefDomain, &use);
                    if (bRet) {
                        // this should *never* happen, because we didn't pass in a buffer large enough for
                        // the sid or the domain name.
                        WcaLog(LOGMSG_STANDARD, "doesUserExist: Lookup Account Name: Unexpected error %d 0x%x", err, err);
                        return -1;
                    }
                    err = GetLastError();
                    if (ERROR_NONE_MAPPED == err) {
                        // this user doesn't exist.  We're done
                        WcaLog(LOGMSG_STANDARD, "retried user doesn't exist");
                        return 0;
                    }
                    if (ERROR_INSUFFICIENT_BUFFER != err) {
                        WcaLog(LOGMSG_STANDARD, "Failed retry of lookup account name %d", err);
                        return -1;
                    }
                }
                else {
                    WcaLog(LOGMSG_STANDARD, "doesUserExist: Lookup Account Name: supplied domain, but can't check user database %d 0x%x", err, err);
                    return -1;
                }
            }
            else {
                WcaLog(LOGMSG_STANDARD, "doesUserExist: Lookup Account Name: Unexpected error %d 0x%x", err, err);
                return -1;
            }
            hostToTry = computername.c_str();

        }
        else {        // we don't know what happened
            // on a DC, can't try without domain access
            WcaLog(LOGMSG_STANDARD, "doesUserExist: Lookup Account Name: Expected insufficient buffer, got error %d 0x%x", err, err);
            return -1;
        }
    }
    newsid = (SID *) new BYTE[cbSid];
    ZeroMemory(newsid, cbSid);

    refDomain = new wchar_t[cchRefDomain + 1];
    ZeroMemory(refDomain, (cchRefDomain + 1) * sizeof(wchar_t));

    // try it again
    bRet = LookupAccountName(hostToTry, userToTry, newsid, &cbSid, refDomain, &cchRefDomain, &use);
    if (!bRet) {
        err = GetLastError();
        WcaLog(LOGMSG_STANDARD, "Failed to lookup account name %d", GetLastError());
        retval = -1;
        goto cleanAndFail;
    }
    if (!IsValidSid(newsid)) {
        WcaLog(LOGMSG_STANDARD, "New SID is invalid");
        retval = -1;
        goto cleanAndFail;
    }
    retval = 1;
    WcaLog(LOGMSG_STANDARD, "Got SID from %S", refDomain);

cleanAndFail:
    if (newsid) {
        delete[](BYTE*)newsid;
    }
    if (refDomain) {
        delete[] refDomain;
    }
    return retval;
}

