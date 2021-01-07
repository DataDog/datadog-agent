#include "stdafx.h"

CustomActionData::CustomActionData()
: domainUser(false)
, doInstallSysprobe(false)
, userParamMismatch(false)
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
    DWORD errCode = machine.Detect();
    if (errCode != ERROR_SUCCESS)
    {
        WcaLog(LOGMSG_STANDARD, "Could not determine machine information: %S", FormatErrorMessage(errCode).c_str());
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

    return parseUsernameData()
        && parseSysprobeData();
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

bool CustomActionData::isUserDomainUser() const
{
    return domainUser;
}

bool CustomActionData::isUserLocalUser() const
{
    return !domainUser;
}

bool CustomActionData::DoesUserExist() const
{
    return _ddUserExists;
}

const std::wstring& CustomActionData::UnqualifiedUsername() const
{
    return _unqualifiedUsername;
}

const std::wstring& CustomActionData::Username() const
{
    return _fqUsername;
}

const std::wstring& CustomActionData::Domain() const
{
    return _domain;
}

PSID CustomActionData::Sid() const
{
    return _sid.get();
}

void CustomActionData::Sid(sid_ptr& sid)
{
    _sid = std::move(sid);
}

bool CustomActionData::installSysprobe() const
{
    return doInstallSysprobe;
}

const TargetMachine& CustomActionData::GetTargetMachine() const
{
    return machine;
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

bool CustomActionData::findPreviousUserInfo()
{
    ddRegKey regkeybase;
    bool previousInstall = false;
    if (!regkeybase.getStringValue(keyInstalledUser.c_str(), pvsUser) ||
        !regkeybase.getStringValue(keyInstalledDomain.c_str(), pvsDomain) ||
        pvsUser.length() == 0 ||
        pvsDomain.length() == 0)
    {
        WcaLog(LOGMSG_STANDARD, "previous user registration not found in registry");
        previousInstall = false;
    }
    else {
        WcaLog(LOGMSG_STANDARD, "found previous user (%S) registration in registry", pvsUser.c_str());
        previousInstall = true;
    }
    return previousInstall;
}

void CustomActionData::checkForUserMismatch(bool previousInstall, bool userSupplied, std::wstring& computed_domain, std::wstring& computed_user)
{
    if (!previousInstall && userSupplied)
    {
        WcaLog(LOGMSG_STANDARD, "using supplied username");
    }
    if (previousInstall && userSupplied)
    {
        WcaLog(LOGMSG_STANDARD, "user info supplied on command line and by previous install, checking");
        if (_wcsicmp(pvsDomain.c_str(), computed_domain.c_str()) != 0)
        {
            WcaLog(LOGMSG_STANDARD, "supplied domain and computed domain don't match");
            this->userParamMismatch = true;
        }
        if (_wcsicmp(pvsUser.c_str(), computed_user.c_str()) != 0)
        {
            WcaLog(LOGMSG_STANDARD, "supplied user and computed user don't match");
            this->userParamMismatch = true;
        }
    }
    if (previousInstall)
    {
        // this is a bit obtuse, but there's no way of passing the failure up
        // from here, so even if we set `userParamMismatch` above, we'll hit this
        // code.  That's ok, the install will be failed in `canInstall()`.
        computed_domain = pvsDomain;
        computed_user = pvsUser;
        WcaLog(LOGMSG_STANDARD, "Using previously installed user");
    }
}

void CustomActionData::findSuppliedUserInfo(std::wstring& input, std::wstring& computed_domain, std::wstring& computed_user)
{
    std::wistringstream asStream(input);
    // username is going to be of the form <domain>\<username>
    // if the <domain> is ".", then just do local machine
    getline(asStream, computed_domain, L'\\');
    getline(asStream, computed_user, L'\\');

    if (computed_domain == L".")
    {
        WcaLog(LOGMSG_STANDARD, "Supplied qualified domain '.', using hostname");
        computed_domain = machine.GetMachineName();
        domainUser = false;
    }
    else
    {
        if (0 == _wcsicmp(computed_domain.c_str(), machine.GetMachineName().c_str()))
        {
            WcaLog(LOGMSG_STANDARD, "Supplied hostname as authority");
            domainUser = false;
        }
        else if (0 == _wcsicmp(computed_domain.c_str(), machine.DnsDomainName().c_str()))
        {
            WcaLog(LOGMSG_STANDARD, "Supplied domain name %S %S", computed_domain.c_str(), machine.DnsDomainName().c_str());
            domainUser = true;
        }
        else
        {
            WcaLog(LOGMSG_STANDARD, "Warning: Supplied user in different domain (%S != %S)", computed_domain.c_str(), machine.DnsDomainName().c_str());
            domainUser = true;
        }
    }
}

bool CustomActionData::parseUsernameData()
{
    std::wstring tmpName = ddAgentUserName;
    bool previousInstall = findPreviousUserInfo();
    bool userSupplied = false;

    if (this->value(propertyDDAgentUserName, tmpName)) {
        if (tmpName.length() == 0) {
            tmpName = ddAgentUserName;
        } else {
            userSupplied = true;
        }
    }
    if (std::wstring::npos == tmpName.find(L'\\')) {
        WcaLog(LOGMSG_STANDARD, "loaded username doesn't have domain specifier, assuming local");
        tmpName = L".\\" + tmpName;
    }
    // now create the splits between the domain and user for all to use, too
    std::wstring computed_domain, computed_user;

    // if this is an upgrade (we found a previously recorded username in the registry)
    // and nothing was supplied on the command line, don't bother computing that.  Just use
    // the existing
    if (previousInstall && !userSupplied)
    {
        computed_domain = pvsDomain;
        computed_user = pvsUser;
        WcaLog(LOGMSG_STANDARD, "Using username from previous install");
    }
    else {
        findSuppliedUserInfo(tmpName, computed_domain, computed_user);
        checkForUserMismatch(previousInstall, userSupplied, computed_domain, computed_user);
    }

    _domain = computed_domain;
    _fqUsername = computed_domain + L"\\" + computed_user;
    _unqualifiedUsername = computed_user;

    auto sidResult = GetSidForUser(nullptr, _fqUsername.c_str());

    if (sidResult.Result == ERROR_NONE_MAPPED)
    {
        WcaLog(LOGMSG_STANDARD, "No account \"%S\" found.", _fqUsername.c_str());
        _ddUserExists = false;
    }
    else
    {
        if (sidResult.Result == ERROR_SUCCESS &&
            sidResult.Sid != nullptr)
        {
            WcaLog(LOGMSG_STANDARD, "Found SID for \"%S\" in \"%S\"", _fqUsername.c_str(), sidResult.Domain.c_str());
            _ddUserExists = true;
            _sid = std::move(sidResult.Sid);

            // The domain can be the local computer
            _domain = sidResult.Domain;

            // Use the domain returned by <see cref="LookupAccountName" /> because
            // it might be != from the one the user passed in.
            _fqUsername = _domain + L"\\" + _unqualifiedUsername;
        }
        else
        {
            WcaLog(LOGMSG_STANDARD, "Looking up SID for \"%S\": %S", tmpName.c_str(), FormatErrorMessage(sidResult.Result).c_str());
            return false;
        }
    }

    return true;
}
