#pragma once

class ExplicitAccess
{
  public:
    ExplicitAccess();
    ~ExplicitAccess();

    void Build(LPWSTR pTrusteeName, DWORD AccessPermissions, ACCESS_MODE AccessMode, DWORD Inheritance);

    void BuildGrantUser(LPCWSTR name, DWORD rights);
    void BuildGrantUser(LPCWSTR name, DWORD rights, DWORD inheritance_flags);
    void BuildGrantUser(SID *sid, DWORD rights, DWORD inheritance_flags);
    void BuildGrantGroup(LPCWSTR name);
    // void BuildGrantUserSid(LPCWSTR name);
    void BuildGrantSid(TRUSTEE_TYPE ttype, DWORD rights, DWORD sub1, DWORD sub2);
    const EXPLICIT_ACCESS_W &RawAccess()
    {
        return data;
    };

  private:
    EXPLICIT_ACCESS_W data;
    bool freeSid;
    bool deleteSid;
};
class WinAcl
{
  public:
    WinAcl();
    ~WinAcl();

    void AddToArray(ExplicitAccess &);

    DWORD SetEntriesInAclW(PACL pOldDacl, PACL *pNewDacl);

  private:
    void resizeAccessArray();
    ULONG numberOfEntries;
    ULONG maxNumberOfEntries;
    PEXPLICIT_ACCESSW explictAccessArray;
};
