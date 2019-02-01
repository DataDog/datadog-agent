#include "stdafx.h"

ddRegKey::ddRegKey() :
    hKeyRoot(NULL)
{
    LSTATUS st = RegCreateKeyEx(HKEY_LOCAL_MACHINE,
        datadog_key_root.c_str(),
        0, //reserved, must be zero
        NULL, // class
        0, // no options
        KEY_ALL_ACCESS,
        NULL, // default security
        &this->hKeyRoot,
        NULL); // don't care about disposition

}

ddRegKey::~ddRegKey()
{
    if (this->hKeyRoot) {
        RegCloseKey(this->hKeyRoot);
    }
}

bool ddRegKey::getStringValue(const wchar_t* valname, std::wstring& val)
{
    wchar_t * retdata = NULL;
    DWORD dataSize = 0;
    DWORD type = 0;
    LSTATUS st = RegQueryValueEx(this->hKeyRoot,
        valname,
        0, //reserved
        &type,
        NULL, // no data this time
        &dataSize);
    if (st == ERROR_FILE_NOT_FOUND) {
        return false;
    }
    if (st != 0 && st != ERROR_MORE_DATA) {
        // should never happen
        return false;
    }
    // retdata indicates buffer size in bytes.  It's supposed to include the
    // null terminator, but I'm adding a couple bytes on the end just for fun
    retdata = (wchar_t*) new BYTE[dataSize + 4];
    memset(retdata, 0, dataSize + 4);
    dataSize += 2;
    st = RegQueryValueEx(this->hKeyRoot,
        valname,
        0, //reserved
        &type,
        (LPBYTE)retdata, // no data this time
        &dataSize);
    if (st == ERROR_SUCCESS) {
        val = retdata;
    }
    delete[] retdata;
    return st == ERROR_SUCCESS ? true : false;

}
