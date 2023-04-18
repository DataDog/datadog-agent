#pragma once

#include "SID.h"
#include "TargetMachine.h"
#include "PropertyView.h"

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
    virtual bool IsServiceAccount() const = 0;
    virtual const std::wstring &UnqualifiedUsername() const = 0;
    virtual const std::wstring &Domain() const = 0;
    virtual const std::wstring &FullyQualifiedUsername() const = 0;
    virtual PSID Sid() const = 0;
    virtual void Sid(sid_ptr &sid) = 0;
    virtual std::shared_ptr<ITargetMachine> GetTargetMachine() const = 0;

  protected:
    virtual ~ICustomActionData()
    {
    }
};

class LogonCli;

class CustomActionData : ICustomActionData
{
  private:
    struct User
    {
        std::wstring Domain;
        std::wstring Name;
    };
  public:
    CustomActionData(
        std::shared_ptr<IPropertyView> propertyView,
        std::shared_ptr<ITargetMachine> targetMachine);
    CustomActionData(std::shared_ptr<IPropertyView> propertyView);
    ~CustomActionData();

    bool present(const std::wstring &key) const;
    bool value(const std::wstring &key, std::wstring &val) const;

    bool isUserDomainUser() const override;
    bool isUserLocalUser() const override;
    bool DoesUserExist() const override;
    bool IsServiceAccount() const override;
    const std::wstring &UnqualifiedUsername() const override;
    const std::wstring &FullyQualifiedUsername() const override;
    const std::wstring &Domain() const override;
    PSID Sid() const override;
    void Sid(sid_ptr &sid) override;
    std::shared_ptr<ITargetMachine> GetTargetMachine() const override;

    void setClosedSourceConfig();

  private:
    bool _domainUser;
    User _user;
    std::wstring _fullyQualifiedUsername;
    sid_ptr _sid;
    bool _ddUserExists;
    bool _isServiceAccount;
    LogonCli *_logonCli;
    std::shared_ptr<ITargetMachine> _targetMachine;
    std::shared_ptr<IPropertyView> _propertyView;
    std::optional<User> findPreviousUserInfo();
    std::optional<User> findSuppliedUserInfo();
    void ensureDomainHasCorrectFormat();
    bool parseUsernameData();
    
};

