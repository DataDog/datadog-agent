#pragma once

#include "targetver.h"

#define NTDDI_VERSION NTDDI_VISTA
#define WIN32_LEAN_AND_MEAN             // Exclude rarely-used stuff from Windows headers
// Windows Header Files:
#include <windows.h>
#include <bcrypt.h>
#include <ntsecapi.h>
#include <AccCtrl.h>
#include <AclAPI.h>
#include <sddl.h>
#include <shlwapi.h>
#include <shlobj.h>
#include <UserEnv.h>

#include <stdlib.h>
#include <strsafe.h>
#include <msiquery.h>
#include <lm.h>
#include <lmaccess.h>
#include <lmerr.h>

// std c++ lib
#include <string>
#include <sstream>
#include <map>
#include <sstream>
#include <filesystem>
#include <vector>

// WiX Header Files:
#include <wcautil.h>


// TODO: reference additional headers your program requires here
#include "winacl.h"
#include "customactiondata.h"
#include "customaction.h"
#include "strings.h"
#include "ddreg.h"

#include "resource.h"

#ifdef _WIN64
// define __REGISTER_ALL_SERVICES to have the custom action install APM & process
// agent.  Otherwise, only the core service will be installed.
#define __REGISTER_ALL_SERVICES
#endif
