#ifndef __SHARED_LIBRARIES_PROBES_H
#define __SHARED_LIBRARIES_PROBES_H

#include "probes_generic.h"

#define __matchfunc_crypto match6chars(0, 'l', 'i', 'b', 's', 's', 'l') || match6chars(0, 'c', 'r', 'y', 'p', 't', 'o') || match6chars(0, 'g', 'n', 'u', 't', 'l', 's')

define_probes_for_libset(crypto)

#endif
