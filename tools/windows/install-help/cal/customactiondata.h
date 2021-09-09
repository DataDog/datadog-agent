#pragma once

#include "SID.h"
#include "TargetMachine.h"

#include <map>
#include <Msi.h>
#include <optional>
#include <string>


class ICustomActionData
{
  public:
    virtual bool isUserDomainUser() const = 0;
    virtual bool isUserLocalUser() const = 0;
    virtual bool DoesUserExist() const = 0;
    virtual const std::wstring &UnqualifiedUsername() const = 0;
    virtual const std::wstring &Domain() const = 0;
    virtual std::wstring Username() const = 0;
    virtual PSID Sid() const = 0;
    virtual void Sid(sid_ptr &sid) = 0;
    virtual bool installSysprobe() const = 0;
    virtual std::shared_ptr<ITargetMachine> GetTargetMachine() const = 0;

  protected:
    virtual ~ICustomActionData()
    {
    }
};

class CustomActionData : ICustomActionData
{
  private:
    struct User
    {
        std::wstring Domain;
        std::wstring Name;
    };
  public:
    CustomActionData(std::shared_ptr<ITargetMachine> targetMachine);
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
    std::wstring Username() const override;
    const std::wstring &Domain() const override;
    PSID Sid() const override;
    void Sid(sid_ptr &sid) override;
    bool installSysprobe() const override;
    std::shared_ptr<ITargetMachine> GetTargetMachine() const override;

    bool npmPresent() const;

  private:
    MSIHANDLE _hInstall;
    bool _domainUser;
    std::map<std::wstring, std::wstring> values;
    User _user;
    sid_ptr _sid;
    bool _doInstallSysprobe;
    bool _ddnpmPresent;
    bool _ddUserExists;
    std::shared_ptr<ITargetMachine> _targetMachine;
    std::optional<User> findPreviousUserInfo();
    std::optional<User> findSuppliedUserInfo();
    void ensureDomainHasCorrectFormat();
    bool parseUsernameData();
    bool parseSysprobeData();
};
