#include "stdafx.h"
#include "PropertyReplacer.h"
#include <fstream>
#include <utility>

namespace
{
    template <class Map>
    bool has_key(Map const &m, const typename Map::key_type &key)
    {
        auto const &it = m.find(key);
        return it != m.end();
    }

    std::wstring format_tags(std::map<std::wstring, std::wstring> & values)
    {
        std::wistringstream valueStream(values[L"TAGS"]);
        std::wstringstream result;
        std::wstring token;
        result << L"tags: ";
        while (std::getline(valueStream, token, wchar_t(',')))
        {
            result << std::endl << L"  - " << token;
        }
        return result.str();
    };

    std::wstring format_proxy(std::map<std::wstring, std::wstring> &values)
    {
        const auto &proxyHost = values.find(L"PROXY_HOST");
        const auto &proxyPort = values.find(L"PROXY_PORT");
        const auto &proxyUser = values.find(L"PROXY_USER");
        const auto &proxyPassword = values.find(L"PROXY_PASSWORD");
        std::wstringstream proxy;
        if (proxyUser != values.end())
        {
            proxy << proxyUser->second;
            if (proxyPassword != values.end())
            {
                proxy << L":" << proxyPassword->second;
            }
            proxy << L"@";
        }
        proxy << proxyHost->second;
        if (proxyPort != values.end())
        {
            proxy << L":" << proxyPort->second;
        }
        std::wstringstream newValue;
        newValue << L"proxy:" << std::endl
                 << L"\thttps: " << proxy.str() << std::endl
                 << L"\thttp: " << proxy.str() << std::endl;
        return newValue.str();
    };

} // namespace

CustomActionData::CustomActionData()
    : domainUser(false)
    , doInstallSysprobe(true)
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
    if (!loadPropertyString(this->hInstall, propertyCustomActionData.c_str(), data))
    {
        return false;
    }
    return init(data);
}

bool CustomActionData::init(const std::wstring &data)
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
    while (std::getline(ss, token, L';'))
    {
        // now 'token'  has the key=val; do the same thing for the key=value
        bool boolval = false;
        std::wstringstream instream(token);
        std::wstring key, val;
        if (std::getline(instream, key, L'='))
        {
            std::getline(instream, val);
        }

        if (val.length() > 0)
        {
            this->values[key] = val;
        }
    }

    return parseUsernameData() && parseSysprobeData() && updateYamlConfig();
}

bool CustomActionData::present(const std::wstring &key) const
{
    return this->values.count(key) != 0 ? true : false;
}

bool CustomActionData::value(const std::wstring &key, std::wstring &val) const
{
    const auto kvp = values.find(key);
    if (kvp == values.end())
    {
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

const std::wstring &CustomActionData::UnqualifiedUsername() const
{
    return _unqualifiedUsername;
}

const std::wstring &CustomActionData::Username() const
{
    return _fqUsername;
}

const std::wstring &CustomActionData::Domain() const
{
    return _domain;
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
    return doInstallSysprobe;
}

const TargetMachine &CustomActionData::GetTargetMachine() const
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
    if (!this->value(L"SYSPROBE_PRESENT", sysprobePresent))
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
    this->doInstallSysprobe = true;
    return true;
}

bool CustomActionData::updateYamlConfig()
{
    // Read config in memory. The config should be small enough
    // and we control its source - so it's fine to allocate up front.
    std::wifstream inputConfigStream(datadogyamlfile);
    std::wstring inputConfig;

    inputConfigStream.seekg(0, std::ios::end);
    size_t fileSize = inputConfigStream.tellg();
    if (fileSize <= 0)
    {
        WcaLog(LOGMSG_STANDARD, "ERROR: datadog.yaml file empty !");
        return false;
    }
    inputConfig.reserve(fileSize);
    inputConfigStream.seekg(0, std::ios::beg);

    inputConfig.assign(std::istreambuf_iterator<wchar_t>(inputConfigStream), std::istreambuf_iterator<wchar_t>());

    enum PropId { WxsKey, Regex, Replacement };
    typedef std::map<std::wstring, std::wstring> value_map;
    typedef std::function<std::wstring(value_map &)> formatter_func;
    typedef std::vector<std::tuple<std::wstring, std::wstring, formatter_func>> prop_list;
    for (auto prop : prop_list{
         {L"APIKEY",       L"^[ #]*api_key:.*",        [](auto &v) { return L"api_key: " + v[L"APIKEY"]; }},
         {L"SITE",         L"^[ #]*site:.*",           [](auto &v) { return L"site: " + v[L"SITE"]; }},
         {L"HOSTNAME",     L"^[ #]*hostname:.*",       [](auto &v) { return L"hostname: " + v[L"HOSTNAME"]; }},
         {L"LOGS_ENABLED", L"^[ #]*logs_enabled:.*",   [](auto &v) { return L"logs_enabled: " + v[L"LOGS_ENABLED"]; }},
         {L"CMD_PORT",     L"^[ #]*cmd_port:.*",       [](auto &v) { return L"cmd_port: " + v[L"CMD_PORT"]; }},
         {L"DD_URL",       L"^[ #]*dd_url:.*",         [](auto &v) { return L"dd_url: " + v[L"DD_URL"]; }},
         {L"PYVER",        L"^[ #]*python_version:.*", [](auto &v) { return L"python_version:" + v[L"PYVER"]; }},
         // This replacer will uncomment the logs_config section if LOGS_DD_URL is specified, regardless of its value
         {L"LOGS_DD_URL",  L"^[ #]*logs_config:.*",    [](auto &v) { return L"logs_config:"; }},
         // logs_dd_url and apm_dd_url are indented so override default formatter to specify correct indentation
         {L"LOGS_DD_URL",  L"^[ #]*logs_dd_url:.*",    [](auto &v) { return L"  logs_dd_url:" + v[L"LOGS_DD_URL"]; }},
         {L"TRACE_DD_URL", L"^[ #]*apm_dd_url:.*",     [](auto &v) { return L"  apm_dd_url:" + v[L"TRACE_DD_URL"]; }},
         {L"TAGS",         L"^[ #]*tags:(?:(?:.|\n)*?)^[ #]*- <TAG_KEY>:<TAG_VALUE>", format_tags},
         {L"PROXY_HOST",   L"^[ #]*proxy:.*", format_proxy},
         {L"HOSTNAME_FQDN_ENABLED", L"^[ #]*hostname_fqdn:.*", [](auto &v) { return L"hostname_fqdn:" + v[L"hostname_fqdn"]; }},
    })
    {
        if (has_key(values, std::get<WxsKey>(prop)))
        {
            match(inputConfig, std::get<Regex>(prop))
                .replace_with(std::get<Replacement>(prop)(values));
        }
    }

    // Special cases
    if (has_key(values, L"PROCESS_ENABLED"))
    {
        
        if (has_key(values, L"PROCESS_DD_URL"))
        {
            match(inputConfig, L"^[ #]*process_config:")
                .replace_with(L"process_config:\n  process_dd_url: " + values[L"PROCESS_DD_URL"]);
        }
        else
        {
            match(inputConfig, L"^[ #]*process_config:")
                .replace_with(L"process_config:");
        }

        match(inputConfig, L"process_config:")
            .then(L"^[ #]*enabled:.*")
            // Note that this is a string, and should be between ""
            .replace_with(L"  enabled: \"" + values[L"PROCESS_ENABLED"] + L"\"");
    }

    if (has_key(values, L"APM_ENABLED"))
    {
        match(inputConfig, L"^[ #]*apm_config:").replace_with(L"apm_config:");
        match(inputConfig, L"apm_config:")
            .then(L"^[ #]*enabled:.*")
            .replace_with(L"  enabled: " + values[L"APM_ENABLED"]);
    }
 
    return true;
}

bool CustomActionData::findPreviousUserInfo()
{
    ddRegKey regkeybase;
    bool previousInstall = false;
    if (!regkeybase.getStringValue(keyInstalledUser.c_str(), pvsUser) ||
        !regkeybase.getStringValue(keyInstalledDomain.c_str(), pvsDomain) || pvsUser.length() == 0 ||
        pvsDomain.length() == 0)
    {
        WcaLog(LOGMSG_STANDARD, "previous user registration not found in registry");
        previousInstall = false;
    }
    else
    {
        WcaLog(LOGMSG_STANDARD, "found previous user (%S) registration in registry", pvsUser.c_str());
        previousInstall = true;
    }
    return previousInstall;
}

void CustomActionData::checkForUserMismatch(bool previousInstall, bool userSupplied, std::wstring &computed_domain,
                                            std::wstring &computed_user)
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

void CustomActionData::findSuppliedUserInfo(std::wstring &input, std::wstring &computed_domain,
                                            std::wstring &computed_user)
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
            WcaLog(LOGMSG_STANDARD, "Supplied domain name %S %S", computed_domain.c_str(),
                   machine.DnsDomainName().c_str());
            domainUser = true;
        }
        else
        {
            WcaLog(LOGMSG_STANDARD, "Warning: Supplied user in different domain (%S != %S)", computed_domain.c_str(),
                   machine.DnsDomainName().c_str());
            domainUser = true;
        }
    }
}

bool CustomActionData::parseUsernameData()
{
    std::wstring tmpName = ddAgentUserName;
    bool previousInstall = findPreviousUserInfo();
    bool userSupplied = false;

    if (this->value(propertyDDAgentUserName, tmpName))
    {
        if (tmpName.length() == 0)
        {
            tmpName = ddAgentUserName;
        }
        else
        {
            userSupplied = true;
        }
    }
    if (std::wstring::npos == tmpName.find(L'\\'))
    {
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
    else
    {
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
        if (sidResult.Result == ERROR_SUCCESS && sidResult.Sid != nullptr)
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
            WcaLog(LOGMSG_STANDARD, "Looking up SID for \"%S\": %S", tmpName.c_str(),
                   FormatErrorMessage(sidResult.Result).c_str());
            return false;
        }
    }

    return true;
}
