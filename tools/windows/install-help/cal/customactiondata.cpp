#include "stdafx.h"

CustomActionData::CustomActionData() :
    domainUser(false)
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
    return init(data);

}

bool CustomActionData::init(const std::wstring& data)
{
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
        tmpName = L".\\" + tmpName;
    }
    // now create the splits between the domain and user for all to use, too
    std::wstring computed_domain, computed_user;
    std::wistringstream asStream(tmpName);
    // username is going to be of the form <domain>\<username>
    // if the <domain> is ".", then just do local machine
    getline(asStream, computed_domain, L'\\');
    getline(asStream, computed_user, L'\\');

    if (computed_domain == L".") {
        WcaLog(LOGMSG_STANDARD, "Supplied qualified domain '.', using hostname");
        computed_domain = computername;
        this->domainUser = false;
    } else {
        if(0 == _wcsicmp(computed_domain.c_str(), computername.c_str())){
            WcaLog(LOGMSG_STANDARD, "Supplied hostname as authority");
            this->domainUser = false;
        } else if(0 == _wcsicmp(computed_domain.c_str(), domainname.c_str())){
            WcaLog(LOGMSG_STANDARD, "Supplied domain name %S %S", computed_domain.c_str(), domainname.c_str());
            this->domainUser = true;
        } else {
            WcaLog(LOGMSG_STANDARD, "Warning: Supplied user in different domain (%S != %S)", computed_domain.c_str(), domainname.c_str());
            this->domainUser = true;
        }
    }
    this->domain = computed_domain;
    this->username = computed_domain + L"\\" + computed_user;
    this->uqusername = computed_user;

    WcaLog(LOGMSG_STANDARD, "Computed fully qualified username %S", this->username.c_str());
    return true;
}
