#include "PropertyReplacer.h"
#include "stdafx.h"

IPropertyReplacer::~IPropertyReplacer()
{
}

RegexPropertyReplacer::RegexPropertyReplacer(std::wstring wixPropertyName, std::wstring propertyName,
                                             std::wstring const &regex)
    : _wixPropertyName(std::move(wixPropertyName))
    , _propertyName(std::move(propertyName))
    , _regex(std::wregex(regex))
{
}

void RegexPropertyReplacer::Replace(std::wstring &input, std::map<std::wstring, std::wstring> const &values)
{
    const auto &value = values.find(_wixPropertyName);
    if (value != values.end())
    {
        input = std::regex_replace(input, _regex, _propertyName + L": " + value->second);
    }
}

RegexPropertyReplacer::~RegexPropertyReplacer()
{
}

ProxyPropertyReplacer::ProxyPropertyReplacer()
    : _regex(std::wregex(L"^[ \t#]*proxy:"))
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
