#include "stdafx.h"

RegKey::RegKey()
    : hKeyRoot(NULL)
{
}

RegKey::RegKey(HKEY parentKey, const wchar_t *subkey)
    : hKeyRoot(NULL)
{
    WcaLog(LOGMSG_STANDARD, "Creating/opening key %S", subkey);
    LSTATUS st = RegCreateKeyEx(parentKey, subkey,
                                0,    // reserved, must be zero
                                NULL, // class
                                0,    // no options
                                KEY_ALL_ACCESS,
                                NULL, // default security
                                &this->hKeyRoot,
                                NULL); // don't care about disposition
}

bool RegKey::getStringValue(const wchar_t *valname, std::wstring &val)
{
    wchar_t *retdata = NULL;
    DWORD dataSize = 0;
    DWORD type = 0;
    LSTATUS st = RegQueryValueEx(this->hKeyRoot, valname,
                                 0, // reserved
                                 &type,
                                 NULL, // no data this time
                                 &dataSize);
    if (st == ERROR_FILE_NOT_FOUND)
    {
        return false;
    }
    if (st != 0 && st != ERROR_MORE_DATA)
    {
        // should never happen
        return false;
    }
    // retdata indicates buffer size in bytes.  It's supposed to include the
    // null terminator, but I'm adding a couple bytes on the end just for fun
    retdata = (wchar_t *)new BYTE[dataSize + 4];
    memset(retdata, 0, dataSize + 4);
    dataSize += 2;
    st = RegQueryValueEx(this->hKeyRoot, valname,
                         0, // reserved
                         &type,
                         (LPBYTE)retdata, // no data this time
                         &dataSize);
    if (st == ERROR_SUCCESS)
    {
        val = retdata;
    }
    delete[] retdata;
    return st == ERROR_SUCCESS ? true : false;
}

bool RegKey::setStringValue(const wchar_t *valname, const wchar_t *value)
{
    RegSetValueEx(this->hKeyRoot, valname, 0, REG_SZ, (const BYTE *)value,
                  (DWORD)(wcslen(value) + 1) * sizeof(wchar_t));
    return true;
}


bool RegKey::getDWORDValue(const wchar_t *valname, DWORD &val)
{
    DWORD dataSize = 0;
    DWORD type = 0;
    LSTATUS st = RegQueryValueEx(this->hKeyRoot, valname,
                                 0, // reserved
                                 &type,
                                 NULL, // no data this time
                                 &dataSize);
    if (st == ERROR_FILE_NOT_FOUND)
    {
        return false;
    }
    if (st != 0 && st != ERROR_MORE_DATA)
    {
        // should never happen
        return false;
    }
    // retdata indicates buffer size in bytes. 
    if (dataSize != sizeof(DWORD)){
        return false;
    }
    if (type != REG_DWORD){
        return false;
    }
    DWORD retdata;
    st = RegQueryValueEx(this->hKeyRoot, valname,
                         0, // reserved
                         &type,
                         (LPBYTE)&retdata, // no data this time
                         &dataSize);
    if (st == ERROR_SUCCESS)
    {
        val = retdata;
    }
    return st == ERROR_SUCCESS ? true : false;
}

bool RegKey::setDWORDValue(const wchar_t *valname, DWORD value)
{
    RegSetValueEx(this->hKeyRoot, valname, 0, REG_DWORD, (const BYTE *)&value,
                  (DWORD)sizeof(DWORD));
    return true;
}
bool RegKey::deleteSubKey(const wchar_t *keyname)
{
    return RegDeleteKeyEx(this->hKeyRoot, keyname, 0, 0) == ERROR_SUCCESS ? true : false;
}

bool RegKey::deleteValue(const wchar_t *valname)
{
    return RegDeleteValue(this->hKeyRoot, valname) == ERROR_SUCCESS ? true : false;
}
bool RegKey::createSubKey(const wchar_t *keyname, RegKey &subkey, DWORD options)
{
    WcaLog(LOGMSG_STANDARD, "Creating/opening subkey %S", keyname);
    LSTATUS st = RegCreateKeyEx(this->hKeyRoot, keyname,
                                0,    // reserved, must be zero
                                NULL, // class
                                options, KEY_ALL_ACCESS,
                                NULL, // default security
                                &(subkey.hKeyRoot),
                                NULL); // don't care about disposition
    return st == ERROR_SUCCESS ? true : false;
}
RegKey::~RegKey()
{
    if (this->hKeyRoot)
    {
        RegCloseKey(this->hKeyRoot);
    }
}

ddRegKey::ddRegKey()
    : RegKey(HKEY_LOCAL_MACHINE, datadog_key_root.c_str())
{
}
