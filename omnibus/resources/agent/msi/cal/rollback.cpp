#include "stdafx.h"

static void getPropertyBool(MSIHANDLE h, std::wstring& name, bool& isset)
{
    std::string shortProperty;
    std::string shortVal;
    toMbcs(shortProperty, (LPCWSTR)name.c_str());
    wchar_t returnbuf[MAX_CUSTOM_PROPERTY_SIZE];
    DWORD sz = MAX_CUSTOM_PROPERTY_SIZE;
    UINT ret = MsiGetProperty(h, (LPCWSTR)name.c_str(), returnbuf, &sz);
    if (ret != 0) {
        // if we error out retrieving the property, assume that it's not set
        WcaLog(LOGMSG_STANDARD, "Failed to get property %s %d", shortProperty.c_str(), ret);
        isset = false;
        return;
    }
    if (sz == 0 || wcslen(returnbuf) == 0)
    {
        WcaLog(LOGMSG_STANDARD, "zero length property (not set) %s", shortProperty);
        isset = false;
        return;
    }
    toMbcs(shortVal, returnbuf);
    WcaLog(LOGMSG_STANDARD, "property %s set to %s", shortProperty, shortVal);
    isset = true;
    return;

}
static void getStatusProp(MSIHANDLE h, std::wstring& key, std::wstring& val)
{
    wchar_t * retstring = NULL;
    DWORD bufsz = 0;
    UINT ret = MsiGetProperty(h, (LPCWSTR) key.c_str(), L"", &bufsz);
    if(ret == 0 && bufsz == 0) {
        WcaLog(LOGMSG_STANDARD, "Statusprop not found");
        return;
    }
    else if (ERROR_MORE_DATA != ret) {
        WcaLog(LOGMSG_STANDARD, "unexpected error %d", ret);
        return;
    }
    bufsz += 1;
    retstring = new wchar_t[bufsz + 1];
    ret = MsiGetProperty(h, (LPCWSTR) key.c_str(), retstring, &bufsz);
    if(ERROR_SUCCESS != ret) {
        WcaLog(LOGMSG_STANDARD, "unexpected error %d", ret);
        return;
    }
    val = retstring;
    std::string shortret;
    toMbcs(shortret, retstring);
    WcaLog(LOGMSG_STANDARD, "Got state is %d %d %s", ret, bufsz, shortret.c_str());
    return;
}
static void parseProperty(std::wstring& property, std::map<std::wstring, bool>& retmap)
{
    // first, the string is KEY=VAL;KEY=VAL....
    // first split into key/value pairs
    std::wstringstream ss(property);
    std::wstring token;
    while (std::getline(ss, token, L';')) {
        // now 'token'  has the key=val; do the same thing for the key=value
        bool boolval = false;
        std::wstringstream instream(token);
        std::wstring key, val;
        if (std::getline(instream, key, L'=')) {
            std::getline(instream, val);
        }

        if (val.length() > 0) {
            if (val == L"true") {
                boolval = true;
            }
        }
        retmap[key] = boolval;
    }
    return;

}
/*
 * Rollback is initiated in the event of a failed installation.  Rollback
 * should do the following actions
 *
 * Remove the dd-user IFF this installation added the dd user
 * Remove the secret user IFF this installation added the user
 * Remove the secret user password from the registry IFF it was added by this installation
 *
 * Whether or not those operations were initiated by this installation is indicated
 * by properties set during the install
 */
void logProcCount();
 extern "C" UINT __stdcall RollbackInstallation(MSIHANDLE hInstall)
{
    HRESULT hr = S_OK;
    UINT er = ERROR_SUCCESS;
    bool bDDUserWasAdded = false;
    bool bDDSecretUserWasAdded = false;
    bool bDDSecretPasswordWasAdded = false;
    std::wstring propertystring;
    std::map<std::wstring, bool> params;
    
    // that's helpful.  WcaInitialize Log header silently limited to 32 chars
    hr = WcaInitialize(hInstall, "CA: Rollback");
    ExitOnFailure(hr, "Failed to initialize");
    logProcCount();
    WcaLog(LOGMSG_STANDARD, "Rollback Initialized.");
    
    getStatusProp(hInstall, propertyRollbackState, propertystring);
    // since a rollback CA is deferred, can only read one property.  So the string is
    // a concatenation of the properties we're interested in.  parse the resulting
    // string to see what we need to do.
    parseProperty(propertystring, params);

    // check and see what was done during the install so far
    bDDUserWasAdded = params[propertyDDUserCreated];
    bDDSecretUserWasAdded = params[propertySecretUserCreated];
    bDDSecretPasswordWasAdded = params[propertySecretPasswordWritten];

    if (bDDUserWasAdded) {
        WcaLog(LOGMSG_STANDARD, "User was added by this installation, deleting");
        DeleteUser(ddAgentUserName);
    }
    if (bDDSecretPasswordWasAdded) {
        WcaLog(LOGMSG_STANDARD, "secret user was added, deleting");
        DeleteUser(secretUserUsername);
    }
    if (bDDSecretPasswordWasAdded) {
        WcaLog(LOGMSG_STANDARD, "secret password added to registry, deleting");
        DeleteSecretsRegKey();
    }
    WcaLog(LOGMSG_STANDARD, "Custom action rollback complete");
LExit:
    er = SUCCEEDED(hr) ? ERROR_SUCCESS : ERROR_INSTALL_FAILURE;
    return WcaFinalize(er);


}
