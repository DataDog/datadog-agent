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

class PropertyView : public IPropertyView
{
  public:
    bool present(const std::wstring &key) const;
    bool value(const std::wstring &key, std::wstring &val) const;

  protected:
    std::map<std::wstring, std::wstring> values;
    virtual ~PropertyView() { }
};

class CAPropertyView : public PropertyView
{
  public:
    CAPropertyView(MSIHANDLE hInstall);
  protected:
    MSIHANDLE _hInstall;
    virtual ~CAPropertyView() { }
};

class ImmediateCAPropertyView : public CAPropertyView
{
  public:
    ImmediateCAPropertyView(MSIHANDLE hInstall);
};

class DeferredCAPropertyView : public CAPropertyView
{
  public:
    DeferredCAPropertyView(MSIHANDLE hInstall);
};

