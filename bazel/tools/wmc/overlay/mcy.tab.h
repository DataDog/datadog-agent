/* A Bison parser, made by GNU Bison 3.8.2.  */

/* Bison interface for Yacc-like parsers in C

   Copyright (C) 1984, 1989-1990, 2000-2015, 2018-2021 Free Software Foundation,
   Inc.

   This program is free software: you can redistribute it and/or modify
   it under the terms of the GNU General Public License as published by
   the Free Software Foundation, either version 3 of the License, or
   (at your option) any later version.

   This program is distributed in the hope that it will be useful,
   but WITHOUT ANY WARRANTY; without even the implied warranty of
   MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
   GNU General Public License for more details.

   You should have received a copy of the GNU General Public License
   along with this program.  If not, see <https://www.gnu.org/licenses/>.  */

/* As a special exception, you may create a larger work that contains
   part or all of the Bison parser skeleton and distribute that work
   under terms of your choice, so long as that work isn't itself a
   parser generator using the skeleton or a modified version thereof
   as a parser skeleton.  Alternatively, if you modify or redistribute
   the parser skeleton itself, you may (at your option) remove this
   special exception, which will cause the skeleton and the resulting
   Bison output files to be licensed under the GNU General Public
   License without this special exception.

   This special exception was added by the Free Software Foundation in
   version 2.2 of Bison.  */

/* DO NOT RELY ON FEATURES THAT ARE NOT DOCUMENTED in the manual,
   especially those whose name start with YY_ or yy_.  They are
   private implementation details that can be changed or removed.  */

#ifndef YY_MCY_MCY_TAB_H_INCLUDED
# define YY_MCY_MCY_TAB_H_INCLUDED
/* Debug traces.  */
#ifndef MCY_DEBUG
# if defined YYDEBUG
#if YYDEBUG
#   define MCY_DEBUG 1
#  else
#   define MCY_DEBUG 0
#  endif
# else /* ! defined YYDEBUG */
#  define MCY_DEBUG 0
# endif /* ! defined YYDEBUG */
#endif  /* ! defined MCY_DEBUG */
#if MCY_DEBUG
extern int mcy_debug;
#endif

/* Token kinds.  */
#ifndef MCY_TOKENTYPE
# define MCY_TOKENTYPE
  enum mcy_tokentype
  {
    MCY_EMPTY = -2,
    MCY_EOF = 0,                   /* "end of file"  */
    MCY_error = 256,               /* error  */
    MCY_UNDEF = 257,               /* "invalid token"  */
    tSEVNAMES = 258,               /* tSEVNAMES  */
    tFACNAMES = 259,               /* tFACNAMES  */
    tLANNAMES = 260,               /* tLANNAMES  */
    tBASE = 261,                   /* tBASE  */
    tCODEPAGE = 262,               /* tCODEPAGE  */
    tTYPEDEF = 263,                /* tTYPEDEF  */
    tNL = 264,                     /* tNL  */
    tSYMNAME = 265,                /* tSYMNAME  */
    tMSGEND = 266,                 /* tMSGEND  */
    tSEVERITY = 267,               /* tSEVERITY  */
    tFACILITY = 268,               /* tFACILITY  */
    tLANGUAGE = 269,               /* tLANGUAGE  */
    tMSGID = 270,                  /* tMSGID  */
    tIDENT = 271,                  /* tIDENT  */
    tLINE = 272,                   /* tLINE  */
    tFILE = 273,                   /* tFILE  */
    tCOMMENT = 274,                /* tCOMMENT  */
    tNUMBER = 275,                 /* tNUMBER  */
    tTOKEN = 276                   /* tTOKEN  */
  };
  typedef enum mcy_tokentype mcy_token_kind_t;
#endif

/* Value type.  */
#if ! defined MCY_STYPE && ! defined MCY_STYPE_IS_DECLARED
union MCY_STYPE
{
#line 100 "mcy.y"

	WCHAR		*str;
	unsigned	num;
	struct token	*tok;
	struct lanmsg	*lmp;
	struct msg	*msg;
	struct lan_cp	lcp;

#line 102 "mcy.tab.h"

};
typedef union MCY_STYPE MCY_STYPE;
# define MCY_STYPE_IS_TRIVIAL 1
# define MCY_STYPE_IS_DECLARED 1
#endif


extern MCY_STYPE mcy_lval;


int mcy_parse (void);


#endif /* !YY_MCY_MCY_TAB_H_INCLUDED  */
