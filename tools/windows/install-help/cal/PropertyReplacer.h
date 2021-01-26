#pragma once
#include <functional>
#include <regex>
#include <string>
#include <optional>

class PropertyReplacer
{
private:
    std::wstring &_input;
    std::vector<std::wregex> _matches;
    PropertyReplacer(std::wstring &input, std::wstring const &match);
  public:
    bool replace_with(std::wstring const &replacement);

    PropertyReplacer &then(std::wstring const &match);

    static PropertyReplacer match(std::wstring &input, std::wstring const &match);
};

typedef std::function<std::optional<std::wstring>(std::wstring const &)> property_retriever;
std::wstring replace_yaml_properties(std::wstring input, const property_retriever &propertyRetriever);
