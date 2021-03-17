#pragma once
#include <functional>
#include <regex>
#include <string>
#include <optional>

class PropertyReplacer
{
  public:
    typedef std::function<std::wstring(std::wstring const &)> formatter_t;

private:
    std::wstring &_input;
    std::vector<std::wregex> _matches;
    PropertyReplacer(std::wstring &input, std::wstring const &match);
    formatter_t _formatter;
  public:
    bool replace_with(std::wstring const &replacement);

    PropertyReplacer &then(std::wstring const &match);

    static PropertyReplacer match(std::wstring &input, std::wstring const &match);
};

/// <summary>
/// Given a <paramref name="propertyName" />, returns an optional value associated with it.
/// </summary>
typedef std::function<std::optional<std::wstring>(std::wstring const &propertyName)> property_retriever;

/// <summary>
/// Replaces the properties in a YAML string.
/// </summary>
/// <param name="input">The string to replace the properties in.</param>
/// <param name="propertyRetriever">A functor that will be called for each property to replace, to obtain the value of the property.</param>
/// <param name="failedToReplace">An optional list of properties that were specified but that didn't match the input.</param>
/// <returns>A copy of the input string with the properties replaced.</returns>
std::wstring replace_yaml_properties(std::wstring input,
                                     const property_retriever &propertyRetriever,
                                     std::vector<std::wstring> *failedToReplace = nullptr);
