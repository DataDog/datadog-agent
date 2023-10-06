#pragma once

class RegKey
{
  public:
    RegKey();
    RegKey(HKEY parentKey, const wchar_t *subkey);
    ~RegKey();

    bool getStringValue(const wchar_t *valname, std::wstring &val);
    bool setStringValue(const wchar_t *valname, const wchar_t *value);

    bool getDWORDValue(const wchar_t *valname, DWORD &val);
    bool setDWORDValue(const wchar_t *valname, DWORD val);

    bool deleteSubKey(const wchar_t *keyname);
    bool deleteValue(const wchar_t *valname);
    bool createSubKey(const wchar_t *keyname, RegKey &subkey, DWORD options = 0);

  private:
    HKEY hKeyRoot;
};

class ddRegKey : public RegKey
{
  public:
    ddRegKey();
};
