#include "stdafx.h"
#include <utility>
#include "customactiondata.h"
#include "PropertyReplacer.h"
#include "LogonCli.h"

CustomActionData::CustomActionData(std::shared_ptr<IPropertyView> propertyView,
                                   std::shared_ptr<ITargetMachine> targetMachine)
    : _domainUser(false)
    , _doInstallSysprobe(true)
    , _ddnpmPresent(false)
    , _ddUserExists(false)
    , _targetMachine(std::move(targetMachine))
    , _propertyView(std::move(propertyView))
    , _logonCli(nullptr)
{
    try
    {
        _logonCli = new LogonCli();
    }
    catch (std::exception &e)
    {
        WcaLog(LOGMSG_STANDARD, "Could not load logonCli.dll: %s", e.what());
    }

    DWORD errCode = _targetMachine->Detect();
    if (errCode != ERROR_SUCCESS)
    {
        WcaLog(LOGMSG_STANDARD, "Could not determine machine information: %S", FormatErrorMessage(errCode).c_str());
        throw std::exception("Could not determine machine information");
    }

    // Process some data now
    if (!parseUsernameData() || !parseSysprobeData())
    {
        throw std::exception("Error parsing machine information");
    }
}

CustomActionData::CustomActionData(std::shared_ptr<IPropertyView> propertyView)
    : CustomActionData(std::move(propertyView), std::make_shared<TargetMachine>())
{
}

CustomActionData::~CustomActionData()
{
    delete _logonCli;
}

bool CustomActionData::present(const std::wstring &key) const
{
    return this->_propertyView->present(key);
}

bool CustomActionData::value(const std::wstring &key, std::wstring &val) const
{
    return this->_propertyView->value(key, val);
}

bool CustomActionData::isUserDomainUser() const
{
    return _domainUser;
}

bool CustomActionData::isUserLocalUser() const
{
    return !_domainUser;
}

bool CustomActionData::DoesUserExist() const
{
    return _ddUserExists;
}

bool CustomActionData::IsServiceAccount() const
{
    return _isServiceAccount;
}

const std::wstring &CustomActionData::UnqualifiedUsername() const
{
    return _user.Name;
}

const std::wstring &CustomActionData::FullyQualifiedUsername() const
{
    return _fullyQualifiedUsername;
}

const std::wstring &CustomActionData::Domain() const
{
    return _user.Domain;
}

PSID CustomActionData::Sid() const
{
    return _sid.get();
}

void CustomActionData::Sid(sid_ptr &sid)
{
    _sid = std::move(sid);
}

bool CustomActionData::installSysprobe() const
{
    return _doInstallSysprobe;
}

bool CustomActionData::npmPresent() const
{
    return this->_ddnpmPresent;
}

std::shared_ptr<ITargetMachine> CustomActionData::GetTargetMachine() const
{
    return _targetMachine;
}

// return value of this function is true if the data was parsed,
// false otherwise. Return value of this function doesn't indicate whether
// sysprobe is to be installed; this function sets the boolean that can
// be checked by installSysprobe();
bool CustomActionData::parseSysprobeData()
{
    std::wstring sysprobePresent;
    std::wstring addlocal;
    std::wstring npm;
    std::wstring npmFeature;
    this->_doInstallSysprobe = false;
    this->_ddnpmPresent = false;
    if (!this->_propertyView->value(L"SYSPROBE_PRESENT", sysprobePresent))
    {
        // key isn't even there.
        WcaLog(LOGMSG_STANDARD, "SYSPROBE_PRESENT not present");
        return true;
    }
    WcaLog(LOGMSG_STANDARD, "SYSPROBE_PRESENT is %S", sysprobePresent.c_str());
    if (sysprobePresent.compare(L"true") != 0)
    {
        // explicitly disabled
        WcaLog(LOGMSG_STANDARD, "SYSPROBE_PRESENT explicitly disabled %S", sysprobePresent.c_str());
        return true;
    }
    this->_doInstallSysprobe = true;

    if (!this->_propertyView->value(L"NPM", npm))
    {
        WcaLog(LOGMSG_STANDARD, "NPM property not present");
    }
    else
    {
        WcaLog(LOGMSG_STANDARD, "NPM enabled via NPM property");
        this->_ddnpmPresent = true;
    }

    if (this->_propertyView->value(L"NPMFEATURE", npmFeature))
    {
        // this property is set to "on" or "off" depending on the desired installed state
        // of the NPM feature.
        WcaLog(LOGMSG_STANDARD, "NPMFEATURE key is present and (%S)", npmFeature.c_str());
        if (_wcsicmp(npmFeature.c_str(), L"on") == 0)
        {
            this->_ddnpmPresent = true;
        }
    }
    else
    {
        WcaLog(LOGMSG_STANDARD, "NPMFEATURE not present");
    }

    return true;
}

std::optional<CustomActionData::User> CustomActionData::findPreviousUserInfo()
{
    ddRegKey regkeybase;
    User user;
    if (!regkeybase.getStringValue(keyInstalledUser.c_str(), user.Name) ||
        !regkeybase.getStringValue(keyInstalledDomain.c_str(), user.Domain) || user.Name.length() == 0 ||
        user.Domain.length() == 0)
    {
        WcaLog(LOGMSG_STANDARD, "previous user information not found in registry");
        return std::nullopt;
    }
    WcaLog(LOGMSG_STANDARD, "found previous user \"%S\\%S\" information in registry", user.Domain.c_str(),
           user.Name.c_str());
    return std::optional<User>(user);
}

std::optional<CustomActionData::User> CustomActionData::findSuppliedUserInfo()
{
    User user;
    std::wstring tmpName;
    if (!this->_propertyView->value(propertyDDAgentUserName, tmpName) || tmpName.length() == 0)
    {
        WcaLog(LOGMSG_STANDARD, "no username information detected from command line");
        return std::nullopt;
    }

    if (std::wstring::npos == tmpName.find(L'\\'))
    {
        WcaLog(LOGMSG_STANDARD, "supplied username \"%S\" doesn't have domain specifier, assuming local",
               tmpName.c_str());
        tmpName = L".\\" + tmpName;
    }

    std::wistringstream asStream(tmpName);
    // username is going to be of the form <domain>\<username>
    // if the <domain> is ".", then just do local machine
    getline(asStream, user.Domain, L'\\');
    getline(asStream, user.Name, L'\\');
    WcaLog(LOGMSG_STANDARD, "detected user \"%S\\%S\" information from command line", user.Domain.c_str(),
           user.Name.c_str());
    return std::optional<User>(user);
}

void CustomActionData::ensureDomainHasCorrectFormat()
{
    if (_user.Domain == L".")
    {
        if (_targetMachine->IsDomainController())
        {
            // User didn't specify a domain OR didn't specify a user, but we're on a domain controller
            // let's use the joined domain.
            _user.Domain = _targetMachine->JoinedDomainName();
            _domainUser = true;
            WcaLog(LOGMSG_STANDARD,
                   "No domain name supplied for installation on a Domain Controller, using joined domain \"%S\"",
                   _user.Domain.c_str());
        }
        else
        {
            WcaLog(LOGMSG_STANDARD, "Supplied qualified domain '.', using hostname");
            _user.Domain = _targetMachine->GetMachineName();
            _domainUser = false;
        }
    }
    else
    {
        if (0 == _wcsicmp(_user.Domain.c_str(), _targetMachine->GetMachineName().c_str()))
        {
            WcaLog(LOGMSG_STANDARD, "Supplied hostname as authority");
            _domainUser = false;
        }
        else if (0 == _wcsicmp(_user.Domain.c_str(), _targetMachine->DnsDomainName().c_str()))
        {
            WcaLog(LOGMSG_STANDARD, "Supplied domain name \"%S\"", _user.Domain.c_str());
            _domainUser = true;
        }
        else
        {
            // Compute a temporary fully qualified username to retrieve its SID
            // in order to determine if its prefix starts with NT AUTHORITY.
            auto tempFquname = _user.Domain + L"\\" + _user.Name;
            auto sidResult = GetSidForUser(nullptr, tempFquname.c_str());
            if (sidResult.Result != ERROR_NONE_MAPPED)
            {
                const auto ntAuthoritySid = WellKnownSID::NTAuthority();
                if (!ntAuthoritySid.has_value())
                {
                    WcaLog(LOGMSG_STANDARD, "Cannot check user SID against NT AUTHORITY: memory allocation failed");
                }
                else if (!EqualPrefixSid(
                             sidResult.Sid.get(),
                             ntAuthoritySid.value().get())) // NT Authority should never be considered a "domain".
                {
                    WcaLog(LOGMSG_STANDARD, "Warning: Supplied user in different domain (\"%S\" != \"%S\")",
                           _user.Domain.c_str(), _targetMachine->DnsDomainName().c_str());
                    _domainUser = true;
                }
            }
        }
    }
}

bool CustomActionData::parseUsernameData()
{
    std::optional<User> userFromPreviousInstall = findPreviousUserInfo();
    std::optional<User> userFromCommandLine = findSuppliedUserInfo();

    // if this is an upgrade (we found a previously recorded username in the registry)
    // and nothing was supplied on the command line, don't bother computing that.  Just use
    // the existing
    if (userFromCommandLine)
    {
        WcaLog(LOGMSG_STANDARD, "Using username from command line");
        _user = userFromCommandLine.value();
    }
    else if (userFromPreviousInstall)
    {
        WcaLog(LOGMSG_STANDARD, "Using username from previous install");
        _user = userFromPreviousInstall.value();
    }
    else
    {
        WcaLog(LOGMSG_STANDARD, "Using default username");
        // Didn't find a user in the registry nor from the command line
        // use default value. Order of construction is Domain then Name
        _user = {L".", ddAgentUserName};
    }

    ensureDomainHasCorrectFormat();

    _fullyQualifiedUsername = _user.Domain + L"\\" + _user.Name;
    auto sidResult = GetSidForUser(nullptr, FullyQualifiedUsername().c_str());

    if (sidResult.Result == ERROR_NONE_MAPPED)
    {
        WcaLog(LOGMSG_STANDARD, "No account \"%S\" found.", FullyQualifiedUsername().c_str());
        _ddUserExists = false;
    }
    else
    {
        if (sidResult.Result == ERROR_SUCCESS && sidResult.Sid != nullptr)
        {
            WcaLog(LOGMSG_STANDARD, R"(Found SID for "%S" in "%S")", FullyQualifiedUsername().c_str(),
                   sidResult.Domain.c_str());
            _ddUserExists = true;
            _sid = std::move(sidResult.Sid);

            if (_logonCli != nullptr)
            {
                BOOL isServiceAccount = FALSE;
                DWORD result = _logonCli->NetIsServiceAccount(
                    nullptr, const_cast<wchar_t *>(FullyQualifiedUsername().c_str()), &isServiceAccount);
                if (result != ERROR_SUCCESS)
                {
                    WcaLog(LOGMSG_STANDARD, "Could not lookup if \"%S\" is a service account: %S",
                           FullyQualifiedUsername().c_str(), FormatErrorMessage(result).c_str());
                }
                _isServiceAccount = isServiceAccount ? true : false;
            }

            WcaLog(LOGMSG_STANDARD, R"("%S" %S a managed service account)", FullyQualifiedUsername().c_str(),
                   _isServiceAccount ? L"is" : L"is not");
            // Use the domain returned by <see cref="LookupAccountName" /> because
            // it might be != from the one the user passed in.
            _user.Domain = sidResult.Domain;
        }
        else
        {
            WcaLog(LOGMSG_STANDARD, "Looking up SID for \"%S\": %S", FullyQualifiedUsername().c_str(),
                   FormatErrorMessage(sidResult.Result).c_str());
            return false;
        }
    }

    return true;
}
