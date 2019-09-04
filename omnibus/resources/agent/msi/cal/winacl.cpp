#include "stdafx.h"


ExplicitAccess::ExplicitAccess()
: freeSid(false)
, deleteSid(false)
{
	memset(&data, 0, sizeof(EXPLICIT_ACCESS));
}

ExplicitAccess::~ExplicitAccess() {
	if (data.Trustee.TrusteeForm == TRUSTEE_IS_NAME && data.Trustee.ptstrName) {
		free(data.Trustee.ptstrName);
	}
	else if (data.Trustee.TrusteeForm == TRUSTEE_IS_SID && data.Trustee.ptstrName) {
        if (this->freeSid) {
            FreeSid((PSID)data.Trustee.ptstrName);
        }
        else if (this->deleteSid) {
            delete[](BYTE *)  data.Trustee.ptstrName;
        }
	}
}

void ExplicitAccess::Build(
	LPWSTR trusteeName,
	const DWORD accessPermissions,
	const ACCESS_MODE accessMode,
	const DWORD inheritance)
{
	data.grfAccessPermissions = accessPermissions;
	data.grfAccessMode = accessMode;
	data.grfInheritance = inheritance;
	data.Trustee.pMultipleTrustee = nullptr;
	data.Trustee.MultipleTrusteeOperation = NO_MULTIPLE_TRUSTEE;
	data.Trustee.TrusteeForm = TRUSTEE_IS_NAME;
	data.Trustee.TrusteeType = TRUSTEE_IS_USER;
	data.Trustee.ptstrName = trusteeName;
}

void ExplicitAccess::BuildGrantUser(LPCWSTR name, DWORD rights)
{
	Build(
		_wcsdup(name),
		rights,
		GRANT_ACCESS,
		NO_INHERITANCE);
}

void ExplicitAccess::BuildGrantUser(LPCWSTR name, DWORD rights, DWORD inheritance_flags)
{
	Build(
		_wcsdup(name),
		rights,
		GRANT_ACCESS,
		inheritance_flags
	);
}

void ExplicitAccess::BuildGrantUser(SID *sid, DWORD rights, DWORD inheritance_flags)
{
    this->deleteSid = true;
	Build(
		reinterpret_cast<LPWSTR>(sid),
		rights,
		GRANT_ACCESS,
		inheritance_flags
	);
	data.Trustee.TrusteeForm = TRUSTEE_IS_SID;
}

void ExplicitAccess::BuildGrantGroup(LPCWSTR name)
{
	Build(
		_wcsdup(name),
		GENERIC_READ | GENERIC_EXECUTE | READ_CONTROL | KEY_READ,
		GRANT_ACCESS,
		NO_INHERITANCE
	);
	data.Trustee.TrusteeType = TRUSTEE_IS_GROUP;
}

/*
void ExplicitAccess::BuildGrantUserSid(LPCWSTR name) {
	LPWSTR localName = _wcsdup(name);

	data.grfAccessPermissions = GENERIC_READ | GENERIC_EXECUTE | READ_CONTROL;
	data.grfAccessMode = GRANT_ACCESS;
	data.grfInheritance = NO_INHERITANCE;
	data.Trustee.pMultipleTrustee = NULL;
	data.Trustee.MultipleTrusteeOperation = NO_MULTIPLE_TRUSTEE;
	data.Trustee.TrusteeForm = TRUSTEE_IS_SID;
	data.Trustee.TrusteeType = TRUSTEE_IS_USER;
	data.Trustee.ptstrName = localName;
}*/

void ExplicitAccess::BuildGrantSid(TRUSTEE_TYPE ttype, DWORD rights, DWORD sub1, DWORD sub2)
{
	SID_IDENTIFIER_AUTHORITY SIDAuth = SECURITY_NT_AUTHORITY;
	PSID pSID = NULL;
	int count = 0;
	if (sub1) {
		count++;
	}
	if (sub2) {
		count++;
	}
	if (!AllocateAndInitializeSid(&SIDAuth, count, sub1, sub2, 0, 0, 0, 0, 0, 0, &pSID)) {
		return ;
	}
    this->freeSid = true;
	data.grfAccessPermissions = rights;
	data.grfAccessMode = GRANT_ACCESS;
	data.grfInheritance = NO_INHERITANCE;
	data.Trustee.pMultipleTrustee = NULL;
	data.Trustee.MultipleTrusteeOperation = NO_MULTIPLE_TRUSTEE;
	data.Trustee.TrusteeForm = TRUSTEE_IS_SID;
	data.Trustee.TrusteeType = ttype;
	data.Trustee.ptstrName = (LPTSTR) pSID;
}


#define ACCESS_ARRAY_INCREMENT		10
WinAcl::WinAcl() 
	:numberOfEntries(0), maxNumberOfEntries(0), explictAccessArray(NULL)
{

}

WinAcl::~WinAcl() {
	if (this->explictAccessArray) {
		delete[] this->explictAccessArray;
	}
}

void WinAcl::resizeAccessArray() {
	ULONG newSize = this->maxNumberOfEntries + ACCESS_ARRAY_INCREMENT;
	PEXPLICIT_ACCESSW newArray = new EXPLICIT_ACCESSW[newSize];
	memset(newArray, 0, sizeof(EXPLICIT_ACCESSW) * newSize);

	if (this->explictAccessArray) {
		memcpy(newArray, this->explictAccessArray, sizeof(EXPLICIT_ACCESSW) * this->maxNumberOfEntries);
		delete[]this->explictAccessArray;
		this->explictAccessArray = NULL;
	}
	this->maxNumberOfEntries = newSize;
	this->explictAccessArray = newArray;

}

void WinAcl::AddToArray(ExplicitAccess& ea) {
	if (this->numberOfEntries + 1 >= this->maxNumberOfEntries) {
		this->resizeAccessArray();
	}
	memcpy(&(this->explictAccessArray[this->numberOfEntries]), &(ea.RawAccess()), sizeof(EXPLICIT_ACCESS));

	this->numberOfEntries++;
}

DWORD WinAcl::SetEntriesInAclW(PACL pOldAcl, PACL* ppNewAcl) {
	DWORD ret = ::SetEntriesInAclW(this->numberOfEntries, this->explictAccessArray, pOldAcl, ppNewAcl);
	if (ERROR_SUCCESS == ret) {
		return ret;
	}
	return GetLastError();
}
