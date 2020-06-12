#include "stdafx.h"

CustomActionData::CustomActionData() :
    domainUser(false),
    doInstallSysprobe(false)
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
    // pre-populate sysprobe
    this->parseSysprobeData();
    return true;
}

bool CustomActionData::present(const std::wstring& key) const {
    return this->values.count(key) != 0 ? true : false;
}

bool CustomActionData::value(const std::wstring& key, std::wstring &val) const {
    const auto kvp = values.find(key);
    if (kvp == values.end()) {
        return false;
    }
    val = kvp->second;
    return true;
}

// return value of this function is true if the data was parsed,
// false otherwise. Return value of this function doesn't indicate whether
// sysprobe is to be installed; this function sets the boolean that can
// be checked by installSysprobe();
bool CustomActionData::parseSysprobeData()
{
    std::wstring sysprobePresent;
    std::wstring addlocal;
    this->doInstallSysprobe = false;
    if(!this->value(L"SYSPROBE_PRESENT", sysprobePresent))
    {
        // key isn't even there. 
        WcaLog(LOGMSG_STANDARD, "SYSPROBE_PRESENT not present");
        return true;
    }
    WcaLog(LOGMSG_STANDARD, "SYSPROBE_PRESENT is %S", sysprobePresent.c_str());
    if(sysprobePresent.compare(L"true") != 0) {
        // explicitly disabled
        WcaLog(LOGMSG_STANDARD, "SYSPROBE_PRESENT explicitly disabled %S", sysprobePresent.c_str());
        return true;
    }
    if(!this->value(L"ADDLOCAL", addlocal))
    {
        // should never happen.  But if the addlocalkey isn't there,
        // don't bother trying
        WcaLog(LOGMSG_STANDARD, "ADDLOCAL not present");

        return true;
    }
    WcaLog(LOGMSG_STANDARD, "ADDLOCAL is (%S)", addlocal.c_str());
    if(_wcsicmp(addlocal.c_str(), L"ALL")== 0){
        // installing all components, do it
        this->doInstallSysprobe = true;
        WcaLog(LOGMSG_STANDARD, "ADDLOCAL is ALL");
    } else if (addlocal.find(L"WindowsNP") != std::wstring::npos) {
        WcaLog(LOGMSG_STANDARD, "ADDLOCAL contains WindowsNP %S", addlocal.c_str());
        this->doInstallSysprobe = true;
    }
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
        WCHAR netBiosDomainName[256];
        DWORD size = sizeof netBiosDomainName/sizeof(WCHAR);
        if (DnsHostnameToComputerName(computed_domain.c_str(), netBiosDomainName, &size))
        {
            WcaLog(LOGMSG_VERBOSE, "Computed domain was %S. Equivalent NetBIOS name: %S", computed_domain.c_str(), netBiosDomainName);
            computed_domain = netBiosDomainName;
        } else {
            WcaLog(LOGMSG_STANDARD, "Warning: DnsHostnameToComputerName(%S) did not return success: %d", computed_domain.c_str(), GetLastError());
        }

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
