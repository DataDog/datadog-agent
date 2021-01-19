#pragma once
#include <functional>
#include <map>
#include <regex>
#include <string>

class IPropertyReplacer
{
  public:
    virtual void Replace(std::wstring &input, std::map<std::wstring, std::wstring> const &values) = 0;
    virtual ~IPropertyReplacer();
};

class RegexPropertyReplacer : public IPropertyReplacer
{
  public:
    typedef std::function<std::wstring(std::wstring const &)> formatter_t;

  private:
    std::wstring _wixPropertyName;
    std::wstring _propertyName;
    std::wregex _regex;
    formatter_t _formatter;

  public:
    RegexPropertyReplacer(std::wstring wixPropertyName, std::wstring propertyName, std::wstring const &regex);
    RegexPropertyReplacer(std::wstring wixPropertyName, std::wstring const &regex, formatter_t const &formatter);
    void Replace(std::wstring &input, std::map<std::wstring, std::wstring> const &values) override;
    virtual ~RegexPropertyReplacer();
};

class ProxyPropertyReplacer : public IPropertyReplacer
{
  private:
    std::wregex _regex;

  public:
    ProxyPropertyReplacer();
    void Replace(std::wstring &input, std::map<std::wstring, std::wstring> const &values) override;
    virtual ~ProxyPropertyReplacer();
};
