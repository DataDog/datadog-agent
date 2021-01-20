#pragma once
#include <functional>
#include <regex>
#include <string>

class PropertyReplacer
{
private:
    std::wstring &_input;
    std::vector<std::wregex> _matches;

public:
    PropertyReplacer(std::wstring &input, std::wstring const &match);

    bool replace_with(std::wstring const &replacement);

    PropertyReplacer &then(std::wstring const &match);
};

PropertyReplacer match(std::wstring &input, std::wstring const &match);
