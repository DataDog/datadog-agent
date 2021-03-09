#pragma once
#include "SID.h"
#include "TargetMachine.h"
#include <map>
#include <string>

class ICustomActionData
{
  public:
    virtual bool isUserDomainUser() const = 0;
    virtual bool isUserLocalUser() const = 0;
    virtual bool DoesUserExist() const = 0;
    virtual const std::wstring &UnqualifiedUsername() const = 0;
    virtual const std::wstring &Username() const = 0;
    virtual const std::wstring &Domain() const = 0;
    virtual PSID Sid() const = 0;
    virtual void Sid(sid_ptr &sid) = 0;
    virtual bool installSysprobe() const = 0;
    virtual bool UserParamMismatch() const = 0;
    virtual const ITargetMachine &GetTargetMachine() const = 0;

  protected:
    virtual ~ICustomActionData()
    {
    }
};

class CustomActionData : ICustomActionData
{
  public:
    CustomActionData();
    ~CustomActionData();

    bool init(MSIHANDLE hInstall);

    bool init(const std::wstring &initstring);

    bool present(const std::wstring &key) const;
    bool value(const std::wstring &key, std::wstring &val) const;

    bool isUserDomainUser() const override;
    bool isUserLocalUser() const override;
    bool DoesUserExist() const override;
    const std::wstring &UnqualifiedUsername() const override;
    const std::wstring &Username() const override;
    const std::wstring &Domain() const override;
    PSID Sid() const override;
    void Sid(sid_ptr &sid) override;
    bool installSysprobe() const override;
    bool UserParamMismatch() const override;
    const TargetMachine &GetTargetMachine() const override;

    bool npmPresent() const;
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
    bool ddnpmPresent;
    bool _ddUserExists;
    bool findPreviousUserInfo();
    void checkForUserMismatch(bool previousInstall, bool userSupplied, std::wstring &computed_domain,
                              std::wstring &computed_user);
    void findSuppliedUserInfo(std::wstring &input, std::wstring &computed_domain, std::wstring &computed_user);
    bool parseUsernameData();
    bool parseSysprobeData();
};
