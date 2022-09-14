#pragma once

#include <string>
#include <map>
#include <Msi.h>

void parseKeyValueString(const std::wstring kvstring, std::map<std::wstring, std::wstring> &values);

class IPropertyView
{
  public:
    virtual bool present(const std::wstring &key) const = 0;
    virtual bool value(const std::wstring &key, std::wstring &val) const = 0;

  protected:
    virtual ~IPropertyView() { }
};

/*
 * Used by classes that must load values once at init time
 * and store them into the @values attribute for later access.
 */
class StaticPropertyView : public IPropertyView
{
  public:
    bool present(const std::wstring &key) const override;
    bool value(const std::wstring &key, std::wstring &val) const override;

  protected:
    std::map<std::wstring, std::wstring> values;
    virtual ~StaticPropertyView() { }
};

class CAPropertyView
{
  public:
    CAPropertyView(MSIHANDLE hInstall);
  protected:
    MSIHANDLE _hInstall;
    virtual ~CAPropertyView() { }
};

class ImmediateCAPropertyView : public CAPropertyView, public IPropertyView
{
  public:
    ImmediateCAPropertyView(MSIHANDLE hInstall);
    bool present(const std::wstring &key) const override;
    bool value(const std::wstring &key, std::wstring &val) const override;
};

class DeferredCAPropertyView : public CAPropertyView, public StaticPropertyView
{
  public:
    DeferredCAPropertyView(MSIHANDLE hInstall);
};

