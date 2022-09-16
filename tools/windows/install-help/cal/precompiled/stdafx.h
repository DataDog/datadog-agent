#pragma once

#include "../targetver.h"

#define NTDDI_VERSION NTDDI_VISTA
#define WIN32_LEAN_AND_MEAN // Exclude rarely-used stuff from Windows headers
// Windows Header Files:
#include <AccCtrl.h>
#include <AclAPI.h>
#include <UserEnv.h>
#include <bcrypt.h>
#include <ntsecapi.h>
#include <sddl.h>
#include <shlobj.h>
#include <shlwapi.h>
#include <windows.h>

#include <lm.h>
#include <lmaccess.h>
#include <lmerr.h>
#include <msiquery.h>
#include <stdlib.h>
#include <strsafe.h>

// std c++ lib
#include <filesystem>
#include <map>
#include <sstream>
#include <string>
#include <vector>

// WiX Header Files:
#include <wcautil.h>

// TODO: reference additional headers your program requires here
#include "../Error.h"
#include "../customaction.h"
#include "../PropertyView.h"
#include "../customactiondata.h"
#include "../ddreg.h"
#include "../resource.h"
#include "../strings.h"
#include "../winacl.h"

#ifdef _WIN64
// define __REGISTER_ALL_SERVICES to have the custom action install APM & process
// agent.  Otherwise, only the core service will be installed.
#define __REGISTER_ALL_SERVICES
#endif
