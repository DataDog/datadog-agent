#pragma once
#include "SID.h"
#include "TargetMachine.h"
#include <map>
#include <string>

class CustomActionData
{
  public:
    CustomActionData();
    ~CustomActionData();

    bool init(MSIHANDLE hInstall);
    bool init(const std::wstring &initstring);

    bool present(const std::wstring &key) const;
    bool value(const std::wstring &key, std::wstring &val) const;

    bool isUserDomainUser() const;

    bool isUserLocalUser() const;

    bool DoesUserExist() const;

    const std::wstring &UnqualifiedUsername() const;

    const std::wstring &Username() const;

    const std::wstring &Domain() const;

    PSID Sid() const;
    void Sid(sid_ptr &sid);

    bool installSysprobe() const;

    bool UserParamMismatch() const
    {
        return userParamMismatch;
    }

    const TargetMachine &GetTargetMachine() const;

  private:
    MSIHANDLE hInstall;
    TargetMachine machine;
    bool domainUser;
    bool userParamMismatch;
    std::map<std::wstring, std::wstring> values;
    std::wstring _unqualifiedUsername;
    std::wstring _domain;
    std::wstring _fqUsername;
    std::wstring pvsUser;   // previously installed user, read from registry
    std::wstring pvsDomain; // previously installed domain for user, read from registry
    sid_ptr _sid;
    bool doInstallSysprobe;
    bool _ddUserExists;
    bool findPreviousUserInfo();
    void checkForUserMismatch(bool previousInstall, bool userSupplied, std::wstring &computed_domain,
                              std::wstring &computed_user);
    void findSuppliedUserInfo(std::wstring &input, std::wstring &computed_domain, std::wstring &computed_user);
    bool parseUsernameData();
    bool parseSysprobeData();
};
