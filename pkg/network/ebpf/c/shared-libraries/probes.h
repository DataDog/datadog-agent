#ifndef __SHARED_LIBRARIES_PROBES_H
#define __SHARED_LIBRARIES_PROBES_H

#include "probes_generic.h"

// Define here all the possible libsets (i.e., groups of libraries that we want to filter)
// Each libset must have a corresponding __matchfunc_##libset function that will be used to match the library name
// and a define_probes_for_libset(libset) macro that will define all the probes for that libset
// Remember to update pkg/network/usm/sharedlibraries/libset.go to include the new libset and define
// the library suffixes for validation

#define __matchfunc_crypto match6chars(0, 'l', 'i', 'b', 's', 's', 'l') || match6chars(0, 'c', 'r', 'y', 'p', 't', 'o') || match6chars(0, 'g', 'n', 'u', 't', 'l', 's')

define_probes_for_libset(crypto)

#endif
