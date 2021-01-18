#pragma once
#include <string>
#include <map>
#include <regex>

class IPropertyReplacer
{
  public:
    virtual void Replace(std::wstring &input, std::map<std::wstring, std::wstring> const &values) = 0;
    virtual ~IPropertyReplacer();
};

class RegexPropertyReplacer : public IPropertyReplacer
{
  private:
    std::wstring _wixPropertyName;
    std::wstring _propertyName;
    std::wregex _regex;

  public:
    RegexPropertyReplacer(std::wstring wixPropertyName, std::wstring propertyName, std::wstring const &regex);
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
