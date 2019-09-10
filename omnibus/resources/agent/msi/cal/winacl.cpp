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
void ExplicitAccess::Build(LPWSTR pTrusteeName, DWORD AccessPermissions,
	ACCESS_MODE AccessMode, DWORD  Inheritance)
{
	LPWSTR localName = _wcsdup(pTrusteeName);
	::BuildExplicitAccessWithNameW(&data, localName, AccessPermissions, AccessMode, Inheritance);
	
}

void ExplicitAccess::BuildGrantUser(LPCWSTR name, DWORD rights) {
	LPWSTR localName = _wcsdup(name);

	data.grfAccessPermissions = rights;
	data.grfAccessMode = GRANT_ACCESS;
	data.grfInheritance = NO_INHERITANCE;
	data.Trustee.pMultipleTrustee = NULL;
	data.Trustee.MultipleTrusteeOperation = NO_MULTIPLE_TRUSTEE;
	data.Trustee.TrusteeForm = TRUSTEE_IS_NAME;
	data.Trustee.TrusteeType = TRUSTEE_IS_USER;
	data.Trustee.ptstrName = localName;
}

void ExplicitAccess::BuildGrantUser(LPCWSTR name, DWORD rights, DWORD inheritance_flags) {
	LPWSTR localName = _wcsdup(name);

	data.grfAccessPermissions = rights;
	data.grfAccessMode = GRANT_ACCESS;
	data.grfInheritance = inheritance_flags;
	data.Trustee.pMultipleTrustee = NULL;
	data.Trustee.MultipleTrusteeOperation = NO_MULTIPLE_TRUSTEE;
	data.Trustee.TrusteeForm = TRUSTEE_IS_NAME;
	data.Trustee.TrusteeType = TRUSTEE_IS_USER;
	data.Trustee.ptstrName = localName;
}


void ExplicitAccess::BuildGrantUser(SID *sid, DWORD rights, DWORD inheritance_flags) {
    this->deleteSid = true;
    data.grfAccessPermissions = rights;
    data.grfAccessMode = GRANT_ACCESS;
    data.grfInheritance = inheritance_flags;
    data.Trustee.pMultipleTrustee = NULL;
    data.Trustee.MultipleTrusteeOperation = NO_MULTIPLE_TRUSTEE;
    data.Trustee.TrusteeForm = TRUSTEE_IS_SID;
    data.Trustee.TrusteeType = TRUSTEE_IS_USER;
    data.Trustee.ptstrName = (LPWSTR)sid;
}


void ExplicitAccess::BuildGrantGroup(LPCWSTR name) {
	LPWSTR localName = _wcsdup(name);

	data.grfAccessPermissions = GENERIC_READ | GENERIC_EXECUTE | READ_CONTROL | KEY_READ;
	data.grfAccessMode = GRANT_ACCESS;
	data.grfInheritance = NO_INHERITANCE;
	data.Trustee.pMultipleTrustee = NULL;
	data.Trustee.MultipleTrusteeOperation = NO_MULTIPLE_TRUSTEE;
	data.Trustee.TrusteeForm = TRUSTEE_IS_NAME;
	data.Trustee.TrusteeType = TRUSTEE_IS_GROUP;
	data.Trustee.ptstrName = localName;
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
