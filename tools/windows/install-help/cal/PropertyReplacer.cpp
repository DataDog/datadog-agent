#include "stdafx.h"
#include "PropertyReplacer.h"

namespace
{
    template <class Map>
    bool has_key(Map const &m, const typename Map::key_type &key)
    {
        auto const &it = m.find(key);
        return it != m.end();
    }

    typedef std::function<std::wstring(std::wstring const &, const property_retriever &)> formatter_func;

    /// <summary>
    /// Simply concatenates <paramref name="str"/> with the value of the matching property.
    /// </summary>
    /// <param name="str">The string to use as a replacement.</param>
    /// <returns>A function that conforms to <see cref="formatter_func"> that when called with a property value,
    /// will return a concatenated string of <paramref name="str"/> and the property value. </returns>
    formatter_func format_simple_value(const std::wstring &str)
    {
        return [str](std::wstring const &propertyValue, const property_retriever &)
        {
            return str + propertyValue;
        };
    }

    std::wstring format_tags(const std::wstring &tags, const property_retriever &)
    {
        std::wistringstream valueStream(tags);
        std::wstringstream result;
        std::wstring token;
        result << L"tags: ";
        while (std::getline(valueStream, token, wchar_t(',')))
        {
            result << std::endl << L"  - " << token;
        }
        return result.str();
    };

    std::wstring format_proxy(const std::wstring &proxyHost, const property_retriever &propertyRetriever)
    {
        const auto &proxyPort = propertyRetriever(L"PROXY_PORT");
        const auto &proxyUser = propertyRetriever(L"PROXY_USER");
        const auto &proxyPassword = propertyRetriever(L"PROXY_PASSWORD");
        std::wstringstream proxy;
        if (proxyUser)
        {
            proxy << *proxyUser;
            if (proxyPassword)
            {
                proxy << L":" << *proxyPassword;
            }
            proxy << L"@";
        }
        proxy << proxyHost;
        if (proxyPort)
        {
            proxy << L":" << *proxyPort;
        }
        std::wstringstream newValue;
        newValue << L"proxy:" << std::endl
                 << L"  https: " << proxy.str() << std::endl
                 << L"  http: " << proxy.str() << std::endl;
        return newValue.str();
    };

} // namespace

PropertyReplacer::PropertyReplacer(std::wstring &input, std::wstring const &match)
    : _input(input)
{
    _matches.emplace_back(match);
}

bool PropertyReplacer::replace_with(std::wstring const &replacement)
{
    auto start = _input.begin();
    auto end = _input.end();
    std::size_t offset = 0;
    for (auto matchIt = _matches.begin(); matchIt != _matches.end();)
    {
        std::match_results<decltype(start)> results;
        if (!std::regex_search(start + offset, end, results, *matchIt, std::regex_constants::format_first_only))
        {
            return false;
        }
        if (++matchIt == _matches.end())
        {
            _input.erase(offset + results.position(), results.length());
            _input.insert(offset + results.position(), replacement);
        }
        else
        {
            offset += results.position();
        }
    }
    return true;
}

PropertyReplacer &PropertyReplacer::then(std::wstring const &match)
{
    _matches.emplace_back(match);
    return *this;
}

PropertyReplacer PropertyReplacer::match(std::wstring &input, std::wstring const &match)
{
    return PropertyReplacer(input, match);
}

std::wstring replace_yaml_properties(std::wstring input, const property_retriever &propertyRetriever)
{
    enum PropId
    {
        WxsKey,
        Regex,
        Replacement
    };
    typedef std::vector<std::tuple<std::wstring, std::wstring, formatter_func>> prop_list;
    for (auto prop : prop_list{
        {L"APIKEY",       L"^[ #]*api_key:.*",        format_simple_value(L"api_key: ") },
        {L"SITE",         L"^[ #]*site:.*",           format_simple_value(L"site: ") },
        {L"HOSTNAME",     L"^[ #]*hostname:.*",       format_simple_value(L"hostname: ") },
        {L"LOGS_ENABLED", L"^[ #]*logs_enabled:.*",   format_simple_value(L"logs_enabled: ") },
        {L"CMD_PORT",     L"^[ #]*cmd_port:.*",       format_simple_value(L"cmd_port: ") },
        {L"DD_URL",       L"^[ #]*dd_url:.*",         format_simple_value(L"dd_url: ") },
        {L"PYVER",        L"^[ #]*python_version:.*", format_simple_value(L"python_version: ") },
        // This replacer will uncomment the logs_config section if LOGS_DD_URL is specified, regardless of its value
        {L"LOGS_DD_URL",  L"^[ #]*logs_config:.*",    [](auto const &v, auto const &) { return L"logs_config:"; }},
        // logs_dd_url and apm_dd_url are indented so override default formatter to specify correct indentation
        {L"LOGS_DD_URL",  L"^[ #]*logs_dd_url:.*",    format_simple_value(L"  logs_dd_url:") },
        {L"TRACE_DD_URL", L"^[ #]*apm_dd_url:.*",     format_simple_value(L"  apm_dd_url:") },
        {L"TAGS",         L"^[ #]*tags:(?:(?:.|\n)*?)^[ #]*- <TAG_KEY>:<TAG_VALUE>", format_tags},
        {L"PROXY_HOST",   L"^[ #]*proxy:.*",          format_proxy},
        {L"HOSTNAME_FQDN_ENABLED", L"^[ #]*hostname_fqdn:.*", format_simple_value(L"hostname_fqdn:") },
    })
    {
        auto propValue = propertyRetriever(std::get<WxsKey>(prop));
        if (propValue)
        {
            WcaLog(LOGMSG_STANDARD, "Found %S=%S in MSI install database\n", std::get<WxsKey>(prop).c_str(), *propValue->c_str());
            PropertyReplacer::match(input, std::get<Regex>(prop)).replace_with(std::get<Replacement>(prop)(*propValue, propertyRetriever));
        }
    }

    // Special cases
    auto processEnabled = propertyRetriever(L"PROCESS_ENABLED");
    if (processEnabled)
    {
        auto processDdUrl = propertyRetriever(L"PROCESS_DD_URL");
        if (processDdUrl)
        {
            PropertyReplacer::match(input, L"^[ #]*process_config:")
                .replace_with(L"process_config:\n  process_dd_url: " + *processDdUrl);
        }
        else
        {
            PropertyReplacer::match(input, L"^[ #]*process_config:").replace_with(L"process_config:");
        }

        PropertyReplacer::match(input, L"process_config:")
            .then(L"^[ #]*enabled:.*")
            // Note that this is a string, and should be between ""
            .replace_with(L"  enabled: \"" + *processEnabled + L"\"");
    }

    auto apmEnabled = propertyRetriever(L"APM_ENABLED");
    if (apmEnabled)
    {
        PropertyReplacer::match(input, L"^[ #]*apm_config:").replace_with(L"apm_config:");
        PropertyReplacer::match(input, L"apm_config:")
            .then(L"^[ #]*enabled:.*")
            .replace_with(L"  enabled: " + *apmEnabled);
    }

    return input;
}
