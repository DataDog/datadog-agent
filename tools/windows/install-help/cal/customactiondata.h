#pragma once

#include "TargetMachine.h"
#include "SecurityIdentifier.h"
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
    virtual const std::wstring &FullyQualifiedUsername() const = 0;
    virtual SecurityIdentifier const & Sid() const = 0;
    virtual void Sid(SecurityIdentifier &&sid) = 0;
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
    const std::wstring &FullyQualifiedUsername() const override;
    const std::wstring &Domain() const override;
    SecurityIdentifier const & Sid() const override;
    void Sid(SecurityIdentifier &&sid) override;
    bool installSysprobe() const override;
    std::shared_ptr<ITargetMachine> GetTargetMachine() const override;

    bool npmPresent() const;

  private:
    MSIHANDLE _hInstall;
    bool _domainUser;
    std::map<std::wstring, std::wstring> values;
    User _user;
    std::wstring _fullyQualifiedUsername;
    std::unique_ptr<SecurityIdentifier> _sid;
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
