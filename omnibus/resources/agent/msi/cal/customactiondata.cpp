#include "stdafx.h"

CustomActionData::CustomActionData() :
    domainPtr(NULL),
    userPtr(NULL)
{

}

CustomActionData::~CustomActionData()
{

}
bool CustomActionData::init(MSIHANDLE hi) 
{
    this->hInstall = hi;
    std::wstring data;
    if (!loadPropertyString(this->hInstall, propertyCustomActionData.c_str(), data)) {
        return false;
    }
    // first, the string is KEY=VAL;KEY=VAL....
    // first split into key/value pairs
    std::wstringstream ss(data);
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
            this->values[key] = val;
        }
    }

    // pre-populate the domain/user information
    this->parseUsernameData();
    return true;

}

bool CustomActionData::present(const std::wstring& key) const {
    return this->values.count(key) != 0 ? true : false;
}

bool CustomActionData::value(std::wstring& key, std::wstring &val)  {
    if (this->values.count(key) == 0) {
        return false;
    }
    val = this->values[key];
    return true;
}

bool CustomActionData::parseUsernameData()
{
    std::wstring tmpName = ddAgentUserName;
    if (this->value(propertyDDAgentUserName, tmpName)) {
        if (tmpName.length() == 0) {
            tmpName = ddAgentUserName;
        }
    }
    if (std::wstring::npos == tmpName.find(L'\\')) {
        WcaLog(LOGMSG_STANDARD, "loaded username doesn't have domain specifier, assuming local");
        this->fullusername = L".\\" + tmpName;
    }
    else {
        this->fullusername = tmpName;
    }
    // now create the splits between the domain and user for all to use, too
    std::wstring domain, user;
    std::wistringstream asStream(tmpName);
    // username is going to be of the form <domain>\<username>
    // if the <domain> is ".", then just do local machine
    getline(asStream, this->userdomain, L'\\');
    getline(asStream, this->username, L'\\');

    if (this->userdomain != L".") {
        this->domainPtr = this->userdomain.c_str();
    }
    this->userPtr = this->username.c_str();
    toMbcs(this->fullusermbcs, this->fullusername.c_str());

    if (this->domainPtr == NULL) {
        this->qualifieduser = this->username;
    }
    else {
        this->qualifieduser = this->fullusername;
    }
    return true;
}
