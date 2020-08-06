/*-*- Mode: C; c-basic-offset: 8; indent-tabs-mode: nil -*-*/

#ifndef foosdcommonhfoo
#define foosdcommonhfoo

/***
  This file is part of systemd.

  Copyright 2013 Lennart Poettering

  systemd is free software; you can redistribute it and/or modify it
  under the terms of the GNU Lesser General Public License as published by
  the Free Software Foundation; either version 2.1 of the License, or
  (at your option) any later version.

  systemd is distributed in the hope that it will be useful, but
  WITHOUT ANY WARRANTY; without even the implied warranty of
  MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the GNU
  Lesser General Public License for more details.

  You should have received a copy of the GNU Lesser General Public License
  along with systemd; If not, see <http://www.gnu.org/licenses/>.
***/

/* This is a private header; never even think of including this directly! */

#if defined __INCLUDE_LEVEL__ &&  __INCLUDE_LEVEL__ <= 1
#error "Do not include _sd-common.h directly; it is a private header."
#endif

#ifndef _sd_printf_
#  if __GNUC__ >= 4
#    define _sd_printf_(a,b) __attribute__ ((format (printf, a, b)))
#  else
#    define _sd_printf_(a,b)
#  endif
#endif

#ifndef _sd_sentinel_
#  define _sd_sentinel_ __attribute__((sentinel))
#endif

#ifndef _sd_packed_
#  define _sd_packed_ __attribute__((packed))
#endif

#ifndef _sd_pure_
#  define _sd_pure_ __attribute__((pure))
#endif

#ifndef _SD_STRINGIFY
#  define _SD_XSTRINGIFY(x) #x
#  define _SD_STRINGIFY(x) _SD_XSTRINGIFY(x)
#endif

#ifndef _SD_BEGIN_DECLARATIONS
#  ifdef __cplusplus
#    define _SD_BEGIN_DECLARATIONS                              \
        extern "C" {                                            \
        struct __useless_struct_to_allow_trailing_semicolon__
#  else
#    define _SD_BEGIN_DECLARATIONS                              \
        struct __useless_struct_to_allow_trailing_semicolon__
#  endif
#endif

#ifndef _SD_END_DECLARATIONS
#  ifdef __cplusplus
#    define _SD_END_DECLARATIONS                                \
        }                                                       \
        struct __useless_struct_to_allow_trailing_semicolon__
#  else
#    define _SD_END_DECLARATIONS                                \
        struct __useless_struct_to_allow_trailing_semicolon__
#  endif
#endif

#endif
