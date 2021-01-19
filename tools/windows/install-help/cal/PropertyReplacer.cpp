#include "stdafx.h"
#include "PropertyReplacer.h"

IPropertyReplacer::~IPropertyReplacer()
{
}

RegexPropertyReplacer::RegexPropertyReplacer(std::wstring wixPropertyName, std::wstring const &regex,
                                             formatter_t const &formatter)
    : _wixPropertyName(std::move(wixPropertyName))
    , _regex(std::wregex(regex))
    , _formatter(formatter)
{
}

RegexPropertyReplacer::RegexPropertyReplacer(std::wstring wixPropertyName, std::wstring propertyName,
                                             std::wstring const &regex)
    : RegexPropertyReplacer(wixPropertyName, regex, [propertyName](auto const &v) { return propertyName + L": " + v; })
{
}

void RegexPropertyReplacer::Replace(std::wstring &input, std::map<std::wstring, std::wstring> const &values)
{
    const auto &value = values.find(_wixPropertyName);
    if (value != values.end())
    {
        input = std::regex_replace(input, _regex, _formatter(value->second), std::regex_constants::format_first_only);
    }
}

RegexPropertyReplacer::~RegexPropertyReplacer()
{
}

const std::wstring proxySection =
    L"# proxy:\n#   https: http://<USERNAME>:<PASSWORD>@<PROXY_SERVER_FOR_HTTPS>:<PORT>\n#   http: "
    L"http://<USERNAME>:<PASSWORD>@<PROXY_SERVER_FOR_HTTP>:<PORT>\n#   no_proxy:\n#     - <HOSTNAME-1>\n#     - "
    L"<HOSTNAME-2>";

ProxyPropertyReplacer::ProxyPropertyReplacer()
    : _regex(std::wregex(proxySection))
{
}

void ProxyPropertyReplacer::Replace(std::wstring &input, std::map<std::wstring, std::wstring> const &values)
{
    const auto &proxyHost = values.find(L"PROXY_HOST");
    if (proxyHost != values.end())
    {
        const auto &proxyUser = values.find(L"PROXY_USER");
        const auto &proxyPassword = values.find(L"PROXY_PASSWORD");
        const auto &proxyPort = values.find(L"PROXY_PORT");
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
        input = std::regex_replace(input, _regex, newValue.str());
    }
}

ProxyPropertyReplacer::~ProxyPropertyReplacer()
{
}
