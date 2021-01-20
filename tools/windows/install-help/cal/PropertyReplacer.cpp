#include "stdafx.h"
#include "PropertyReplacer.h"

PropertyReplacer::PropertyReplacer(std::wstring &input, std::wstring const &match)
    : _input(input)
{
    _matches.push_back(std::wregex(match));
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
    _matches.push_back(std::wregex(match));
    return *this;
}

PropertyReplacer match(std::wstring &input, std::wstring const &match)
{
    return PropertyReplacer(input, match);
}
