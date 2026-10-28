/* A Bison parser, made by GNU Bison 3.8.2.  */

/* Bison implementation for Yacc-like parsers in C

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

/* C LALR(1) parser skeleton written by Richard Stallman, by
   simplifying the original so-called "semantic" parser.  */

/* DO NOT RELY ON FEATURES THAT ARE NOT DOCUMENTED in the manual,
   especially those whose name start with YY_ or yy_.  They are
   private implementation details that can be changed or removed.  */

/* All symbols defined below should begin with yy or YY, to avoid
   infringing on user name space.  This should be done even for local
   variables, as they might otherwise be expanded by user macros.
   There are some unavoidable exceptions within include files to
   define necessary library symbols; they are noted "INFRINGES ON
   USER NAME SPACE" below.  */

/* Identify Bison output, and Bison version.  */
#define YYBISON 30802

/* Bison version string.  */
#define YYBISON_VERSION "3.8.2"

/* Skeleton name.  */
#define YYSKELETON_NAME "yacc.c"

/* Pure parsers.  */
#define YYPURE 0

/* Push parsers.  */
#define YYPUSH 0

/* Pull parsers.  */
#define YYPULL 1

/* Substitute the type names.  */
#define YYSTYPE         MCY_STYPE
/* Substitute the variable and function names.  */
#define yyparse         mcy_parse
#define yylex           mcy_lex
#define yyerror         mcy_error
#define yydebug         mcy_debug
#define yynerrs         mcy_nerrs
#define yylval          mcy_lval
#define yychar          mcy_char

/* First part of user prologue.  */
#line 39 "mcy.y"


#include "config.h"

#include <stdio.h>
#include <stdlib.h>
#include <assert.h>

#include "wmc.h"
#include "utils.h"
#include "lang.h"

static const char err_syntax[]	= "Syntax error\n";
static const char err_number[]	= "Number expected\n";
static const char err_ident[]	= "Identifier expected\n";
static const char err_assign[]	= "'=' expected\n";
static const char err_popen[]	= "'(' expected\n";
static const char err_pclose[]	= "')' expected\n";
static const char err_colon[]	= "':' expected\n";
static const char err_msg[]	= "Message expected\n";

/* Scanner switches */
int want_nl = 0;		/* Request next newlinw */
int want_line = 0;		/* Request next complete line */
int want_file = 0;		/* Request next ident as filename */

struct node *nodehead = NULL;	/* The list of all parsed elements */
static struct node *nodetail;
struct lan_blk *lanblockhead;	/* List of parsed elements transposed */

static int base = 16;		/* Current printout base to use (8, 10 or 16) */
static WCHAR *cast = NULL;	/* Current typecast to use */

static int last_id = 0;		/* The last message ID parsed */
static int last_sev = 0;	/* Last severity code parsed */
static int last_fac = 0;	/* Last facility code parsed */
static WCHAR *last_sym = NULL;/* Last alias symbol parsed */
static int have_sev;		/* Set if severity parsed for current message */
static int have_fac;		/* Set if facility parsed for current message */
static int have_sym;		/* Set is symbol parsed for current message */

static struct cp_xlat *cpxlattab = NULL;	/* Codepage translation table */
static int ncpxlattab = 0;

/* Prototypes */
static WCHAR *merge(WCHAR *s1, WCHAR *s2);
static struct lanmsg *new_lanmsg(struct lan_cp *lcp, WCHAR *msg);
static struct msg *add_lanmsg(struct msg *msg, struct lanmsg *lanmsg);
static struct msg *complete_msg(struct msg *msg, int id);
static void add_node(enum node_type type, void *p);
static void do_add_token(enum tok_enum type, struct token *tok, const char *code);
static void test_id(int id);
static int check_languages(struct node *head);
static struct lan_blk *block_messages(struct node *head);
static void add_cpxlat(int lan, int cpin, int cpout);
static struct cp_xlat *find_cpxlat(int lan);


#line 138 "mcy.tab.c"

# ifndef YY_CAST
#  ifdef __cplusplus
#   define YY_CAST(Type, Val) static_cast<Type> (Val)
#   define YY_REINTERPRET_CAST(Type, Val) reinterpret_cast<Type> (Val)
#  else
#   define YY_CAST(Type, Val) ((Type) (Val))
#   define YY_REINTERPRET_CAST(Type, Val) ((Type) (Val))
#  endif
# endif
# ifndef YY_NULLPTR
#  if defined __cplusplus
#   if 201103L <= __cplusplus
#    define YY_NULLPTR nullptr
#   else
#    define YY_NULLPTR 0
#   endif
#  else
#   define YY_NULLPTR ((void*)0)
#  endif
# endif

#include "mcy.tab.h"
/* Symbol kind.  */
enum yysymbol_kind_t
{
  YYSYMBOL_YYEMPTY = -2,
  YYSYMBOL_YYEOF = 0,                      /* "end of file"  */
  YYSYMBOL_YYerror = 1,                    /* error  */
  YYSYMBOL_YYUNDEF = 2,                    /* "invalid token"  */
  YYSYMBOL_tSEVNAMES = 3,                  /* tSEVNAMES  */
  YYSYMBOL_tFACNAMES = 4,                  /* tFACNAMES  */
  YYSYMBOL_tLANNAMES = 5,                  /* tLANNAMES  */
  YYSYMBOL_tBASE = 6,                      /* tBASE  */
  YYSYMBOL_tCODEPAGE = 7,                  /* tCODEPAGE  */
  YYSYMBOL_tTYPEDEF = 8,                   /* tTYPEDEF  */
  YYSYMBOL_tNL = 9,                        /* tNL  */
  YYSYMBOL_tSYMNAME = 10,                  /* tSYMNAME  */
  YYSYMBOL_tMSGEND = 11,                   /* tMSGEND  */
  YYSYMBOL_tSEVERITY = 12,                 /* tSEVERITY  */
  YYSYMBOL_tFACILITY = 13,                 /* tFACILITY  */
  YYSYMBOL_tLANGUAGE = 14,                 /* tLANGUAGE  */
  YYSYMBOL_tMSGID = 15,                    /* tMSGID  */
  YYSYMBOL_tIDENT = 16,                    /* tIDENT  */
  YYSYMBOL_tLINE = 17,                     /* tLINE  */
  YYSYMBOL_tFILE = 18,                     /* tFILE  */
  YYSYMBOL_tCOMMENT = 19,                  /* tCOMMENT  */
  YYSYMBOL_tNUMBER = 20,                   /* tNUMBER  */
  YYSYMBOL_tTOKEN = 21,                    /* tTOKEN  */
  YYSYMBOL_22_ = 22,                       /* '='  */
  YYSYMBOL_23_ = 23,                       /* '('  */
  YYSYMBOL_24_ = 24,                       /* ')'  */
  YYSYMBOL_25_ = 25,                       /* ':'  */
  YYSYMBOL_26_ = 26,                       /* '+'  */
  YYSYMBOL_YYACCEPT = 27,                  /* $accept  */
  YYSYMBOL_file = 28,                      /* file  */
  YYSYMBOL_items = 29,                     /* items  */
  YYSYMBOL_decl = 30,                      /* decl  */
  YYSYMBOL_global = 31,                    /* global  */
  YYSYMBOL_smaps = 32,                     /* smaps  */
  YYSYMBOL_smap = 33,                      /* smap  */
  YYSYMBOL_fmaps = 34,                     /* fmaps  */
  YYSYMBOL_fmap = 35,                      /* fmap  */
  YYSYMBOL_alias = 36,                     /* alias  */
  YYSYMBOL_lmaps = 37,                     /* lmaps  */
  YYSYMBOL_lmap = 38,                      /* lmap  */
  YYSYMBOL_optcp = 39,                     /* optcp  */
  YYSYMBOL_cmaps = 40,                     /* cmaps  */
  YYSYMBOL_cmap = 41,                      /* cmap  */
  YYSYMBOL_clan = 42,                      /* clan  */
  YYSYMBOL_msg = 43,                       /* msg  */
  YYSYMBOL_44_1 = 44,                      /* $@1  */
  YYSYMBOL_msgid = 45,                     /* msgid  */
  YYSYMBOL_id = 46,                        /* id  */
  YYSYMBOL_sevfacsym = 47,                 /* sevfacsym  */
  YYSYMBOL_sym = 48,                       /* sym  */
  YYSYMBOL_sev = 49,                       /* sev  */
  YYSYMBOL_fac = 50,                       /* fac  */
  YYSYMBOL_bodies = 51,                    /* bodies  */
  YYSYMBOL_body = 52,                      /* body  */
  YYSYMBOL_lang = 53,                      /* lang  */
  YYSYMBOL_lines = 54,                     /* lines  */
  YYSYMBOL_token = 55,                     /* token  */
  YYSYMBOL_setnl = 56,                     /* setnl  */
  YYSYMBOL_setline = 57,                   /* setline  */
  YYSYMBOL_setfile = 58                    /* setfile  */
};
typedef enum yysymbol_kind_t yysymbol_kind_t;




#ifdef short
# undef short
#endif

/* On compilers that do not define __PTRDIFF_MAX__ etc., make sure
   <limits.h> and (if available) <stdint.h> are included
   so that the code can choose integer types of a good width.  */

#ifndef __PTRDIFF_MAX__
# include <limits.h> /* INFRINGES ON USER NAME SPACE */
# if defined __STDC_VERSION__ && 199901 <= __STDC_VERSION__
#  include <stdint.h> /* INFRINGES ON USER NAME SPACE */
#  define YY_STDINT_H
# endif
#endif

/* Narrow types that promote to a signed type and that can represent a
   signed or unsigned integer of at least N bits.  In tables they can
   save space and decrease cache pressure.  Promoting to a signed type
   helps avoid bugs in integer arithmetic.  */

#ifdef __INT_LEAST8_MAX__
typedef __INT_LEAST8_TYPE__ yytype_int8;
#elif defined YY_STDINT_H
typedef int_least8_t yytype_int8;
#else
typedef signed char yytype_int8;
#endif

#ifdef __INT_LEAST16_MAX__
typedef __INT_LEAST16_TYPE__ yytype_int16;
#elif defined YY_STDINT_H
typedef int_least16_t yytype_int16;
#else
typedef short yytype_int16;
#endif

/* Work around bug in HP-UX 11.23, which defines these macros
   incorrectly for preprocessor constants.  This workaround can likely
   be removed in 2023, as HPE has promised support for HP-UX 11.23
   (aka HP-UX 11i v2) only through the end of 2022; see Table 2 of
   <https://h20195.www2.hpe.com/V2/getpdf.aspx/4AA4-7673ENW.pdf>.  */
#ifdef __hpux
# undef UINT_LEAST8_MAX
# undef UINT_LEAST16_MAX
# define UINT_LEAST8_MAX 255
# define UINT_LEAST16_MAX 65535
#endif

#if defined __UINT_LEAST8_MAX__ && __UINT_LEAST8_MAX__ <= __INT_MAX__
typedef __UINT_LEAST8_TYPE__ yytype_uint8;
#elif (!defined __UINT_LEAST8_MAX__ && defined YY_STDINT_H \
       && UINT_LEAST8_MAX <= INT_MAX)
typedef uint_least8_t yytype_uint8;
#elif !defined __UINT_LEAST8_MAX__ && UCHAR_MAX <= INT_MAX
typedef unsigned char yytype_uint8;
#else
typedef short yytype_uint8;
#endif

#if defined __UINT_LEAST16_MAX__ && __UINT_LEAST16_MAX__ <= __INT_MAX__
typedef __UINT_LEAST16_TYPE__ yytype_uint16;
#elif (!defined __UINT_LEAST16_MAX__ && defined YY_STDINT_H \
       && UINT_LEAST16_MAX <= INT_MAX)
typedef uint_least16_t yytype_uint16;
#elif !defined __UINT_LEAST16_MAX__ && USHRT_MAX <= INT_MAX
typedef unsigned short yytype_uint16;
#else
typedef int yytype_uint16;
#endif

#ifndef YYPTRDIFF_T
# if defined __PTRDIFF_TYPE__ && defined __PTRDIFF_MAX__
#  define YYPTRDIFF_T __PTRDIFF_TYPE__
#  define YYPTRDIFF_MAXIMUM __PTRDIFF_MAX__
# elif defined PTRDIFF_MAX
#  ifndef ptrdiff_t
#   include <stddef.h> /* INFRINGES ON USER NAME SPACE */
#  endif
#  define YYPTRDIFF_T ptrdiff_t
#  define YYPTRDIFF_MAXIMUM PTRDIFF_MAX
# else
#  define YYPTRDIFF_T long
#  define YYPTRDIFF_MAXIMUM LONG_MAX
# endif
#endif

#ifndef YYSIZE_T
# ifdef __SIZE_TYPE__
#  define YYSIZE_T __SIZE_TYPE__
# elif defined size_t
#  define YYSIZE_T size_t
# elif defined __STDC_VERSION__ && 199901 <= __STDC_VERSION__
#  include <stddef.h> /* INFRINGES ON USER NAME SPACE */
#  define YYSIZE_T size_t
# else
#  define YYSIZE_T unsigned
# endif
#endif

#define YYSIZE_MAXIMUM                                  \
  YY_CAST (YYPTRDIFF_T,                                 \
           (YYPTRDIFF_MAXIMUM < YY_CAST (YYSIZE_T, -1)  \
            ? YYPTRDIFF_MAXIMUM                         \
            : YY_CAST (YYSIZE_T, -1)))

#define YYSIZEOF(X) YY_CAST (YYPTRDIFF_T, sizeof (X))


/* Stored state numbers (used for stacks). */
typedef yytype_uint8 yy_state_t;

/* State numbers in computations.  */
typedef int yy_state_fast_t;

#ifndef YY_
# if defined YYENABLE_NLS && YYENABLE_NLS
#  if ENABLE_NLS
#   include <libintl.h> /* INFRINGES ON USER NAME SPACE */
#   define YY_(Msgid) dgettext ("bison-runtime", Msgid)
#  endif
# endif
# ifndef YY_
#  define YY_(Msgid) Msgid
# endif
#endif


#ifndef YY_ATTRIBUTE_PURE
# if defined __GNUC__ && 2 < __GNUC__ + (96 <= __GNUC_MINOR__)
#  define YY_ATTRIBUTE_PURE __attribute__ ((__pure__))
# else
#  define YY_ATTRIBUTE_PURE
# endif
#endif

#ifndef YY_ATTRIBUTE_UNUSED
# if defined __GNUC__ && 2 < __GNUC__ + (7 <= __GNUC_MINOR__)
#  define YY_ATTRIBUTE_UNUSED __attribute__ ((__unused__))
# else
#  define YY_ATTRIBUTE_UNUSED
# endif
#endif

/* Suppress unused-variable warnings by "using" E.  */
#if ! defined lint || defined __GNUC__
# define YY_USE(E) ((void) (E))
#else
# define YY_USE(E) /* empty */
#endif

/* Suppress an incorrect diagnostic about yylval being uninitialized.  */
#if defined __GNUC__ && ! defined __ICC && 406 <= __GNUC__ * 100 + __GNUC_MINOR__
# if __GNUC__ * 100 + __GNUC_MINOR__ < 407
#  define YY_IGNORE_MAYBE_UNINITIALIZED_BEGIN                           \
    _Pragma ("GCC diagnostic push")                                     \
    _Pragma ("GCC diagnostic ignored \"-Wuninitialized\"")
# else
#  define YY_IGNORE_MAYBE_UNINITIALIZED_BEGIN                           \
    _Pragma ("GCC diagnostic push")                                     \
    _Pragma ("GCC diagnostic ignored \"-Wuninitialized\"")              \
    _Pragma ("GCC diagnostic ignored \"-Wmaybe-uninitialized\"")
# endif
# define YY_IGNORE_MAYBE_UNINITIALIZED_END      \
    _Pragma ("GCC diagnostic pop")
#else
# define YY_INITIAL_VALUE(Value) Value
#endif
#ifndef YY_IGNORE_MAYBE_UNINITIALIZED_BEGIN
# define YY_IGNORE_MAYBE_UNINITIALIZED_BEGIN
# define YY_IGNORE_MAYBE_UNINITIALIZED_END
#endif
#ifndef YY_INITIAL_VALUE
# define YY_INITIAL_VALUE(Value) /* Nothing. */
#endif

#if defined __cplusplus && defined __GNUC__ && ! defined __ICC && 6 <= __GNUC__
# define YY_IGNORE_USELESS_CAST_BEGIN                          \
    _Pragma ("GCC diagnostic push")                            \
    _Pragma ("GCC diagnostic ignored \"-Wuseless-cast\"")
# define YY_IGNORE_USELESS_CAST_END            \
    _Pragma ("GCC diagnostic pop")
#endif
#ifndef YY_IGNORE_USELESS_CAST_BEGIN
# define YY_IGNORE_USELESS_CAST_BEGIN
# define YY_IGNORE_USELESS_CAST_END
#endif


#define YY_ASSERT(E) ((void) (0 && (E)))

#if !defined yyoverflow

/* The parser invokes alloca or malloc; define the necessary symbols.  */

# ifdef YYSTACK_USE_ALLOCA
#  if YYSTACK_USE_ALLOCA
#   ifdef __GNUC__
#    define YYSTACK_ALLOC __builtin_alloca
#   elif defined __BUILTIN_VA_ARG_INCR
#    include <alloca.h> /* INFRINGES ON USER NAME SPACE */
#   elif defined _AIX
#    define YYSTACK_ALLOC __alloca
#   elif defined _MSC_VER
#    include <malloc.h> /* INFRINGES ON USER NAME SPACE */
#    define alloca _alloca
#   else
#    define YYSTACK_ALLOC alloca
#    if ! defined _ALLOCA_H && ! defined EXIT_SUCCESS
#     include <stdlib.h> /* INFRINGES ON USER NAME SPACE */
      /* Use EXIT_SUCCESS as a witness for stdlib.h.  */
#     ifndef EXIT_SUCCESS
#      define EXIT_SUCCESS 0
#     endif
#    endif
#   endif
#  endif
# endif

# ifdef YYSTACK_ALLOC
   /* Pacify GCC's 'empty if-body' warning.  */
#  define YYSTACK_FREE(Ptr) do { /* empty */; } while (0)
#  ifndef YYSTACK_ALLOC_MAXIMUM
    /* The OS might guarantee only one guard page at the bottom of the stack,
       and a page size can be as small as 4096 bytes.  So we cannot safely
       invoke alloca (N) if N exceeds 4096.  Use a slightly smaller number
       to allow for a few compiler-allocated temporary stack slots.  */
#   define YYSTACK_ALLOC_MAXIMUM 4032 /* reasonable circa 2006 */
#  endif
# else
#  define YYSTACK_ALLOC YYMALLOC
#  define YYSTACK_FREE YYFREE
#  ifndef YYSTACK_ALLOC_MAXIMUM
#   define YYSTACK_ALLOC_MAXIMUM YYSIZE_MAXIMUM
#  endif
#  if (defined __cplusplus && ! defined EXIT_SUCCESS \
       && ! ((defined YYMALLOC || defined malloc) \
             && (defined YYFREE || defined free)))
#   include <stdlib.h> /* INFRINGES ON USER NAME SPACE */
#   ifndef EXIT_SUCCESS
#    define EXIT_SUCCESS 0
#   endif
#  endif
#  ifndef YYMALLOC
#   define YYMALLOC malloc
#   if ! defined malloc && ! defined EXIT_SUCCESS
void *malloc (YYSIZE_T); /* INFRINGES ON USER NAME SPACE */
#   endif
#  endif
#  ifndef YYFREE
#   define YYFREE free
#   if ! defined free && ! defined EXIT_SUCCESS
void free (void *); /* INFRINGES ON USER NAME SPACE */
#   endif
#  endif
# endif
#endif /* !defined yyoverflow */

#if (! defined yyoverflow \
     && (! defined __cplusplus \
         || (defined MCY_STYPE_IS_TRIVIAL && MCY_STYPE_IS_TRIVIAL)))

/* A type that is properly aligned for any stack member.  */
union yyalloc
{
  yy_state_t yyss_alloc;
  YYSTYPE yyvs_alloc;
};

/* The size of the maximum gap between one aligned stack and the next.  */
# define YYSTACK_GAP_MAXIMUM (YYSIZEOF (union yyalloc) - 1)

/* The size of an array large to enough to hold all stacks, each with
   N elements.  */
# define YYSTACK_BYTES(N) \
     ((N) * (YYSIZEOF (yy_state_t) + YYSIZEOF (YYSTYPE)) \
      + YYSTACK_GAP_MAXIMUM)

# define YYCOPY_NEEDED 1

/* Relocate STACK from its old location to the new one.  The
   local variables YYSIZE and YYSTACKSIZE give the old and new number of
   elements in the stack, and YYPTR gives the new location of the
   stack.  Advance YYPTR to a properly aligned location for the next
   stack.  */
# define YYSTACK_RELOCATE(Stack_alloc, Stack)                           \
    do                                                                  \
      {                                                                 \
        YYPTRDIFF_T yynewbytes;                                         \
        YYCOPY (&yyptr->Stack_alloc, Stack, yysize);                    \
        Stack = &yyptr->Stack_alloc;                                    \
        yynewbytes = yystacksize * YYSIZEOF (*Stack) + YYSTACK_GAP_MAXIMUM; \
        yyptr += yynewbytes / YYSIZEOF (*yyptr);                        \
      }                                                                 \
    while (0)

#endif

#if defined YYCOPY_NEEDED && YYCOPY_NEEDED
/* Copy COUNT objects from SRC to DST.  The source and destination do
   not overlap.  */
# ifndef YYCOPY
#  if defined __GNUC__ && 1 < __GNUC__
#   define YYCOPY(Dst, Src, Count) \
      __builtin_memcpy (Dst, Src, YY_CAST (YYSIZE_T, (Count)) * sizeof (*(Src)))
#  else
#   define YYCOPY(Dst, Src, Count)              \
      do                                        \
        {                                       \
          YYPTRDIFF_T yyi;                      \
          for (yyi = 0; yyi < (Count); yyi++)   \
            (Dst)[yyi] = (Src)[yyi];            \
        }                                       \
      while (0)
#  endif
# endif
#endif /* !YYCOPY_NEEDED */

/* YYFINAL -- State number of the termination state.  */
#define YYFINAL  30
/* YYLAST -- Last index in YYTABLE.  */
#define YYLAST   165

/* YYNTOKENS -- Number of terminals.  */
#define YYNTOKENS  27
/* YYNNTS -- Number of nonterminals.  */
#define YYNNTS  32
/* YYNRULES -- Number of rules.  */
#define YYNRULES  104
/* YYNSTATES -- Number of states.  */
#define YYNSTATES  155

/* YYMAXUTOK -- Last valid token kind.  */
#define YYMAXUTOK   276


/* YYTRANSLATE(TOKEN-NUM) -- Symbol number corresponding to TOKEN-NUM
   as returned by yylex, with out-of-bounds checking.  */
#define YYTRANSLATE(YYX)                                \
  (0 <= (YYX) && (YYX) <= YYMAXUTOK                     \
   ? YY_CAST (yysymbol_kind_t, yytranslate[YYX])        \
   : YYSYMBOL_YYUNDEF)

/* YYTRANSLATE[TOKEN-NUM] -- Symbol number corresponding to TOKEN-NUM
   as returned by yylex.  */
static const yytype_int8 yytranslate[] =
{
       0,     2,     2,     2,     2,     2,     2,     2,     2,     2,
       2,     2,     2,     2,     2,     2,     2,     2,     2,     2,
       2,     2,     2,     2,     2,     2,     2,     2,     2,     2,
       2,     2,     2,     2,     2,     2,     2,     2,     2,     2,
      23,    24,     2,    26,     2,     2,     2,     2,     2,     2,
       2,     2,     2,     2,     2,     2,     2,     2,    25,     2,
       2,    22,     2,     2,     2,     2,     2,     2,     2,     2,
       2,     2,     2,     2,     2,     2,     2,     2,     2,     2,
       2,     2,     2,     2,     2,     2,     2,     2,     2,     2,
       2,     2,     2,     2,     2,     2,     2,     2,     2,     2,
       2,     2,     2,     2,     2,     2,     2,     2,     2,     2,
       2,     2,     2,     2,     2,     2,     2,     2,     2,     2,
       2,     2,     2,     2,     2,     2,     2,     2,     2,     2,
       2,     2,     2,     2,     2,     2,     2,     2,     2,     2,
       2,     2,     2,     2,     2,     2,     2,     2,     2,     2,
       2,     2,     2,     2,     2,     2,     2,     2,     2,     2,
       2,     2,     2,     2,     2,     2,     2,     2,     2,     2,
       2,     2,     2,     2,     2,     2,     2,     2,     2,     2,
       2,     2,     2,     2,     2,     2,     2,     2,     2,     2,
       2,     2,     2,     2,     2,     2,     2,     2,     2,     2,
       2,     2,     2,     2,     2,     2,     2,     2,     2,     2,
       2,     2,     2,     2,     2,     2,     2,     2,     2,     2,
       2,     2,     2,     2,     2,     2,     2,     2,     2,     2,
       2,     2,     2,     2,     2,     2,     2,     2,     2,     2,
       2,     2,     2,     2,     2,     2,     2,     2,     2,     2,
       2,     2,     2,     2,     2,     2,     1,     2,     3,     4,
       5,     6,     7,     8,     9,    10,    11,    12,    13,    14,
      15,    16,    17,    18,    19,    20,    21
};

#if MCY_DEBUG
/* YYRLINE[YYN] -- Source line where rule number YYN was defined.  */
static const yytype_int16 yyrline[] =
{
       0,   125,   125,   132,   133,   136,   137,   138,   139,   142,
     143,   144,   145,   146,   147,   148,   149,   150,   151,   152,
     153,   154,   155,   156,   157,   158,   159,   160,   161,   173,
     174,   180,   181,   182,   185,   192,   193,   199,   200,   201,
     204,   211,   212,   215,   216,   217,   223,   224,   225,   228,
     236,   237,   238,   239,   242,   243,   244,   250,   251,   252,
     255,   265,   266,   267,   268,   271,   272,   282,   282,   285,
     290,   293,   294,   295,   296,   299,   300,   301,   302,   305,
     306,   307,   310,   318,   319,   322,   330,   331,   337,   338,
     339,   342,   350,   379,   380,   381,   384,   385,   386,   387,
     393,   394,   397,   400,   403
};
#endif

/** Accessing symbol of state STATE.  */
#define YY_ACCESSING_SYMBOL(State) YY_CAST (yysymbol_kind_t, yystos[State])

#if MCY_DEBUG || 0
/* The user-facing name of the symbol whose (internal) number is
   YYSYMBOL.  No bounds checking.  */
static const char *yysymbol_name (yysymbol_kind_t yysymbol) YY_ATTRIBUTE_UNUSED;

/* YYTNAME[SYMBOL-NUM] -- String name of the symbol SYMBOL-NUM.
   First, the terminals, then, starting at YYNTOKENS, nonterminals.  */
static const char *const yytname[] =
{
  "\"end of file\"", "error", "\"invalid token\"", "tSEVNAMES",
  "tFACNAMES", "tLANNAMES", "tBASE", "tCODEPAGE", "tTYPEDEF", "tNL",
  "tSYMNAME", "tMSGEND", "tSEVERITY", "tFACILITY", "tLANGUAGE", "tMSGID",
  "tIDENT", "tLINE", "tFILE", "tCOMMENT", "tNUMBER", "tTOKEN", "'='",
  "'('", "')'", "':'", "'+'", "$accept", "file", "items", "decl", "global",
  "smaps", "smap", "fmaps", "fmap", "alias", "lmaps", "lmap", "optcp",
  "cmaps", "cmap", "clan", "msg", "$@1", "msgid", "id", "sevfacsym", "sym",
  "sev", "fac", "bodies", "body", "lang", "lines", "token", "setnl",
  "setline", "setfile", YY_NULLPTR
};

static const char *
yysymbol_name (yysymbol_kind_t yysymbol)
{
  return yytname[yysymbol];
}
#endif

#define YYPACT_NINF (-37)

#define yypact_value_is_default(Yyn) \
  ((Yyn) == YYPACT_NINF)

#define YYTABLE_NINF (-105)

#define yytable_value_is_error(Yyn) \
  0

/* YYPACT[STATE-NUM] -- Index in YYTABLE of the portion describing
   STATE-NUM.  */
static const yytype_int16 yypact[] =
{
     132,   -37,    18,    44,    46,    47,    48,    49,    50,   -37,
      10,   113,   -37,   -37,   -37,   -37,   -37,    11,   -37,    14,
     -37,    19,   -37,    85,   -37,    20,   -37,   147,   -37,   -13,
     -37,   -37,    87,   -37,    66,   -37,    80,   -37,    82,   -37,
     -37,   -37,    64,   -37,   -37,   -37,   107,   -37,    51,    52,
      53,     3,   -37,   -37,   -37,   -37,   -37,   -37,     7,   -37,
      54,   -37,     8,   -37,    55,   -37,    17,   -37,    56,   -37,
     -37,   -37,    15,   -37,    57,   -37,   -37,   -37,   148,   -37,
      88,   -37,    90,   -37,    58,    -3,   -37,   -37,   -37,   -37,
     -37,   -37,   109,   -37,   -37,   -37,   -37,   114,   -37,   -37,
     -37,   -37,   121,   -37,   -37,   -37,   -37,   122,   -37,   -37,
     -37,   -37,   -37,   -37,   -37,    38,   -37,   129,   -37,    36,
     -37,    36,   -37,     0,   -37,     2,    91,   -37,   -37,   144,
     149,   -37,   -37,   -37,    37,   -37,   123,   -37,     5,   -37,
     -37,   -37,   -37,   -37,     4,   -37,   -37,   -37,   -37,   -37,
      39,   124,   -37,   -37,   -37
};

/* YYDEFACT[STATE-NUM] -- Default reduction number in state STATE-NUM.
   Performed when YYTABLE does not specify something else to do.  Zero
   means the default is an error.  */
static const yytype_int8 yydefact[] =
{
       0,     8,     0,     0,     0,     0,     0,     0,     0,     7,
       0,     0,     3,     5,     6,    75,    12,     0,    16,     0,
      20,     0,    30,     0,    24,     0,    27,     0,    70,    71,
       1,     4,    67,    11,     0,    15,     0,    19,     0,    29,
      28,    23,     0,    26,    25,    72,     0,    69,     0,     0,
       0,     0,    78,    76,    77,    33,   100,   101,     0,    31,
       0,    39,     0,    37,     0,    48,     0,    46,     0,    59,
      65,    66,     0,    57,     0,    74,    73,    81,     0,    84,
       0,    87,     0,    90,     0,    68,    88,   103,    10,     9,
      32,    36,     0,    14,    13,    38,    42,     0,    18,    17,
      47,    53,     0,    22,    21,    58,    64,     0,    80,    79,
      83,    82,    86,    85,    95,     0,    89,     0,    35,    43,
      41,    43,    52,     0,    63,     0,     0,    98,    96,     0,
       0,    34,    40,    51,     0,    62,     0,    94,     0,    99,
      91,    97,    45,    44,     0,    61,    60,    93,    92,    50,
      54,     0,    49,    56,    55
};

/* YYPGOTO[NTERM-NUM].  */
static const yytype_int8 yypgoto[] =
{
     -37,   -37,   -37,    77,   -37,   -37,    35,   -37,     1,   -27,
     -37,    29,   -37,   -37,    30,   -37,   -37,   -37,   -37,   -37,
     -37,   -37,   -37,   -37,   -37,    41,   -37,   -37,   -36,   -37,
     -37,   -37
};

/* YYDEFGOTO[NTERM-NUM].  */
static const yytype_uint8 yydefgoto[] =
{
       0,    10,    11,    12,    13,    58,    59,    62,    63,   131,
      66,    67,   152,    72,    73,    74,    14,    51,    15,    47,
      32,    52,    53,    54,    85,    86,    87,   129,    60,   115,
     117,   134
};

/* YYTABLE[YYPACT[STATE-NUM]] -- What to do in state STATE-NUM.  If
   positive, shift that token.  If negative, reduce the rule whose
   number is the opposite.  If YYTABLE_NINF, syntax error.  */
static const yytype_int16 yytable[] =
{
      64,   133,    68,   135,    83,   149,   147,    45,    88,    93,
      30,    84,    33,    46,   148,    35,   103,    84,    98,    16,
      37,    41,   150,    56,    56,  -104,    64,   136,    57,    57,
      68,    89,    94,    56,    34,    70,    71,    36,    57,   104,
      17,    99,    38,    42,   111,    18,   113,    20,    22,    24,
      26,    28,    77,    79,    81,    91,    96,   101,   106,   114,
     126,   130,   144,    95,   151,    69,    19,    55,    21,    23,
      25,    27,    29,    78,    80,    82,    92,    97,   102,   107,
    -102,    61,    56,    65,    70,    71,    39,    57,    31,   110,
     138,   112,   137,    90,   132,   100,    56,    48,    56,    49,
      50,    57,   105,    57,    56,    40,    56,    56,    75,    57,
     118,    57,    57,    -2,     1,   120,     2,     3,     4,     5,
       6,     7,   122,   124,   145,   153,   116,    76,     8,   119,
     127,     0,     9,     1,   121,     2,     3,     4,     5,     6,
       7,   123,   125,   146,   154,   139,   128,     8,    43,   108,
     142,     9,     0,     0,     0,   140,     0,     0,     0,     0,
       0,   141,     0,    44,   109,   143
};

static const yytype_int8 yycheck[] =
{
      36,     1,    38,     1,     1,     1,     1,    20,     1,     1,
       0,    14,     1,    26,     9,     1,     1,    14,     1,     1,
       1,     1,    18,    16,    16,    25,    62,    25,    21,    21,
      66,    24,    24,    16,    23,    20,    21,    23,    21,    24,
      22,    24,    23,    23,    80,     1,    82,     1,     1,     1,
       1,     1,     1,     1,     1,     1,     1,     1,     1,     1,
      22,    25,    25,    62,    25,     1,    22,     1,    22,    22,
      22,    22,    22,    22,    22,    22,    22,    22,    22,    22,
      22,     1,    16,     1,    20,    21,     1,    21,    11,     1,
     126,     1,     1,    58,   121,    66,    16,    10,    16,    12,
      13,    21,    72,    21,    16,    20,    16,    16,     1,    21,
       1,    21,    21,     0,     1,     1,     3,     4,     5,     6,
       7,     8,     1,     1,     1,     1,    85,    20,    15,    20,
       1,    -1,    19,     1,    20,     3,     4,     5,     6,     7,
       8,    20,    20,    20,    20,     1,    17,    15,     1,     1,
       1,    19,    -1,    -1,    -1,    11,    -1,    -1,    -1,    -1,
      -1,    17,    -1,    16,    16,    16
};

/* YYSTOS[STATE-NUM] -- The symbol kind of the accessing symbol of
   state STATE-NUM.  */
static const yytype_int8 yystos[] =
{
       0,     1,     3,     4,     5,     6,     7,     8,    15,    19,
      28,    29,    30,    31,    43,    45,     1,    22,     1,    22,
       1,    22,     1,    22,     1,    22,     1,    22,     1,    22,
       0,    30,    47,     1,    23,     1,    23,     1,    23,     1,
      20,     1,    23,     1,    16,    20,    26,    46,    10,    12,
      13,    44,    48,    49,    50,     1,    16,    21,    32,    33,
      55,     1,    34,    35,    55,     1,    37,    38,    55,     1,
      20,    21,    40,    41,    42,     1,    20,     1,    22,     1,
      22,     1,    22,     1,    14,    51,    52,    53,     1,    24,
      33,     1,    22,     1,    24,    35,     1,    22,     1,    24,
      38,     1,    22,     1,    24,    41,     1,    22,     1,    16,
       1,    55,     1,    55,     1,    56,    52,    57,     1,    20,
       1,    20,     1,    20,     1,    20,    22,     1,    17,    54,
      25,    36,    36,     1,    58,     1,    25,     1,    55,     1,
      11,    17,     1,    16,    25,     1,    20,     1,     9,     1,
      18,    25,    39,     1,    20
};

/* YYR1[RULE-NUM] -- Symbol kind of the left-hand side of rule RULE-NUM.  */
static const yytype_int8 yyr1[] =
{
       0,    27,    28,    29,    29,    30,    30,    30,    30,    31,
      31,    31,    31,    31,    31,    31,    31,    31,    31,    31,
      31,    31,    31,    31,    31,    31,    31,    31,    31,    31,
      31,    32,    32,    32,    33,    33,    33,    34,    34,    34,
      35,    35,    35,    36,    36,    36,    37,    37,    37,    38,
      38,    38,    38,    38,    39,    39,    39,    40,    40,    40,
      41,    41,    41,    41,    41,    42,    42,    44,    43,    45,
      45,    46,    46,    46,    46,    47,    47,    47,    47,    48,
      48,    48,    49,    49,    49,    50,    50,    50,    51,    51,
      51,    52,    53,    53,    53,    53,    54,    54,    54,    54,
      55,    55,    56,    57,    58
};

/* YYR2[RULE-NUM] -- Number of symbols on the right-hand side of rule RULE-NUM.  */
static const yytype_int8 yyr2[] =
{
       0,     2,     1,     1,     2,     1,     1,     1,     1,     5,
       5,     3,     2,     5,     5,     3,     2,     5,     5,     3,
       2,     5,     5,     3,     2,     3,     3,     2,     3,     3,
       2,     1,     2,     1,     4,     3,     2,     1,     2,     1,
       4,     3,     2,     0,     2,     2,     1,     2,     1,     7,
       6,     4,     3,     2,     0,     2,     2,     1,     2,     1,
       5,     5,     4,     3,     2,     1,     1,     0,     4,     3,
       2,     0,     1,     2,     2,     0,     2,     2,     2,     3,
       3,     2,     3,     3,     2,     3,     3,     2,     1,     2,
       1,     4,     5,     5,     4,     2,     1,     2,     1,     2,
       1,     1,     0,     0,     0
};


enum { YYENOMEM = -2 };

#define yyerrok         (yyerrstatus = 0)
#define yyclearin       (yychar = MCY_EMPTY)

#define YYACCEPT        goto yyacceptlab
#define YYABORT         goto yyabortlab
#define YYERROR         goto yyerrorlab
#define YYNOMEM         goto yyexhaustedlab


#define YYRECOVERING()  (!!yyerrstatus)

#define YYBACKUP(Token, Value)                                    \
  do                                                              \
    if (yychar == MCY_EMPTY)                                        \
      {                                                           \
        yychar = (Token);                                         \
        yylval = (Value);                                         \
        YYPOPSTACK (yylen);                                       \
        yystate = *yyssp;                                         \
        goto yybackup;                                            \
      }                                                           \
    else                                                          \
      {                                                           \
        yyerror (YY_("syntax error: cannot back up")); \
        YYERROR;                                                  \
      }                                                           \
  while (0)

/* Backward compatibility with an undocumented macro.
   Use MCY_error or MCY_UNDEF. */
#define YYERRCODE MCY_UNDEF


/* Enable debugging if requested.  */
#if MCY_DEBUG

# ifndef YYFPRINTF
#  include <stdio.h> /* INFRINGES ON USER NAME SPACE */
#  define YYFPRINTF fprintf
# endif

# define YYDPRINTF(Args)                        \
do {                                            \
  if (yydebug)                                  \
    YYFPRINTF Args;                             \
} while (0)




# define YY_SYMBOL_PRINT(Title, Kind, Value, Location)                    \
do {                                                                      \
  if (yydebug)                                                            \
    {                                                                     \
      YYFPRINTF (stderr, "%s ", Title);                                   \
      yy_symbol_print (stderr,                                            \
                  Kind, Value); \
      YYFPRINTF (stderr, "\n");                                           \
    }                                                                     \
} while (0)


/*-----------------------------------.
| Print this symbol's value on YYO.  |
`-----------------------------------*/

static void
yy_symbol_value_print (FILE *yyo,
                       yysymbol_kind_t yykind, YYSTYPE const * const yyvaluep)
{
  FILE *yyoutput = yyo;
  YY_USE (yyoutput);
  if (!yyvaluep)
    return;
  YY_IGNORE_MAYBE_UNINITIALIZED_BEGIN
  YY_USE (yykind);
  YY_IGNORE_MAYBE_UNINITIALIZED_END
}


/*---------------------------.
| Print this symbol on YYO.  |
`---------------------------*/

static void
yy_symbol_print (FILE *yyo,
                 yysymbol_kind_t yykind, YYSTYPE const * const yyvaluep)
{
  YYFPRINTF (yyo, "%s %s (",
             yykind < YYNTOKENS ? "token" : "nterm", yysymbol_name (yykind));

  yy_symbol_value_print (yyo, yykind, yyvaluep);
  YYFPRINTF (yyo, ")");
}

/*------------------------------------------------------------------.
| yy_stack_print -- Print the state stack from its BOTTOM up to its |
| TOP (included).                                                   |
`------------------------------------------------------------------*/

static void
yy_stack_print (yy_state_t *yybottom, yy_state_t *yytop)
{
  YYFPRINTF (stderr, "Stack now");
  for (; yybottom <= yytop; yybottom++)
    {
      int yybot = *yybottom;
      YYFPRINTF (stderr, " %d", yybot);
    }
  YYFPRINTF (stderr, "\n");
}

# define YY_STACK_PRINT(Bottom, Top)                            \
do {                                                            \
  if (yydebug)                                                  \
    yy_stack_print ((Bottom), (Top));                           \
} while (0)


/*------------------------------------------------.
| Report that the YYRULE is going to be reduced.  |
`------------------------------------------------*/

static void
yy_reduce_print (yy_state_t *yyssp, YYSTYPE *yyvsp,
                 int yyrule)
{
  int yylno = yyrline[yyrule];
  int yynrhs = yyr2[yyrule];
  int yyi;
  YYFPRINTF (stderr, "Reducing stack by rule %d (line %d):\n",
             yyrule - 1, yylno);
  /* The symbols being reduced.  */
  for (yyi = 0; yyi < yynrhs; yyi++)
    {
      YYFPRINTF (stderr, "   $%d = ", yyi + 1);
      yy_symbol_print (stderr,
                       YY_ACCESSING_SYMBOL (+yyssp[yyi + 1 - yynrhs]),
                       &yyvsp[(yyi + 1) - (yynrhs)]);
      YYFPRINTF (stderr, "\n");
    }
}

# define YY_REDUCE_PRINT(Rule)          \
do {                                    \
  if (yydebug)                          \
    yy_reduce_print (yyssp, yyvsp, Rule); \
} while (0)

/* Nonzero means print parse trace.  It is left uninitialized so that
   multiple parsers can coexist.  */
int yydebug;
#else /* !MCY_DEBUG */
# define YYDPRINTF(Args) ((void) 0)
# define YY_SYMBOL_PRINT(Title, Kind, Value, Location)
# define YY_STACK_PRINT(Bottom, Top)
# define YY_REDUCE_PRINT(Rule)
#endif /* !MCY_DEBUG */


/* YYINITDEPTH -- initial size of the parser's stacks.  */
#ifndef YYINITDEPTH
# define YYINITDEPTH 200
#endif

/* YYMAXDEPTH -- maximum size the stacks can grow to (effective only
   if the built-in stack extension method is used).

   Do not make this value too large; the results are undefined if
   YYSTACK_ALLOC_MAXIMUM < YYSTACK_BYTES (YYMAXDEPTH)
   evaluated with infinite-precision integer arithmetic.  */

#ifndef YYMAXDEPTH
# define YYMAXDEPTH 10000
#endif






/*-----------------------------------------------.
| Release the memory associated to this symbol.  |
`-----------------------------------------------*/

static void
yydestruct (const char *yymsg,
            yysymbol_kind_t yykind, YYSTYPE *yyvaluep)
{
  YY_USE (yyvaluep);
  if (!yymsg)
    yymsg = "Deleting";
  YY_SYMBOL_PRINT (yymsg, yykind, yyvaluep, yylocationp);

  YY_IGNORE_MAYBE_UNINITIALIZED_BEGIN
  YY_USE (yykind);
  YY_IGNORE_MAYBE_UNINITIALIZED_END
}


/* Lookahead token kind.  */
int yychar;

/* The semantic value of the lookahead symbol.  */
YYSTYPE yylval;
/* Number of syntax errors so far.  */
int yynerrs;




/*----------.
| yyparse.  |
`----------*/

int
yyparse (void)
{
    yy_state_fast_t yystate = 0;
    /* Number of tokens to shift before error messages enabled.  */
    int yyerrstatus = 0;

    /* Refer to the stacks through separate pointers, to allow yyoverflow
       to reallocate them elsewhere.  */

    /* Their size.  */
    YYPTRDIFF_T yystacksize = YYINITDEPTH;

    /* The state stack: array, bottom, top.  */
    yy_state_t yyssa[YYINITDEPTH];
    yy_state_t *yyss = yyssa;
    yy_state_t *yyssp = yyss;

    /* The semantic value stack: array, bottom, top.  */
    YYSTYPE yyvsa[YYINITDEPTH];
    YYSTYPE *yyvs = yyvsa;
    YYSTYPE *yyvsp = yyvs;

  int yyn;
  /* The return value of yyparse.  */
  int yyresult;
  /* Lookahead symbol kind.  */
  yysymbol_kind_t yytoken = YYSYMBOL_YYEMPTY;
  /* The variables used to return semantic value and location from the
     action routines.  */
  YYSTYPE yyval;



#define YYPOPSTACK(N)   (yyvsp -= (N), yyssp -= (N))

  /* The number of symbols on the RHS of the reduced rule.
     Keep to zero when no symbol should be popped.  */
  int yylen = 0;

  YYDPRINTF ((stderr, "Starting parse\n"));

  yychar = MCY_EMPTY; /* Cause a token to be read.  */

  goto yysetstate;


/*------------------------------------------------------------.
| yynewstate -- push a new state, which is found in yystate.  |
`------------------------------------------------------------*/
yynewstate:
  /* In all cases, when you get here, the value and location stacks
     have just been pushed.  So pushing a state here evens the stacks.  */
  yyssp++;


/*--------------------------------------------------------------------.
| yysetstate -- set current state (the top of the stack) to yystate.  |
`--------------------------------------------------------------------*/
yysetstate:
  YYDPRINTF ((stderr, "Entering state %d\n", yystate));
  YY_ASSERT (0 <= yystate && yystate < YYNSTATES);
  YY_IGNORE_USELESS_CAST_BEGIN
  *yyssp = YY_CAST (yy_state_t, yystate);
  YY_IGNORE_USELESS_CAST_END
  YY_STACK_PRINT (yyss, yyssp);

  if (yyss + yystacksize - 1 <= yyssp)
#if !defined yyoverflow && !defined YYSTACK_RELOCATE
    YYNOMEM;
#else
    {
      /* Get the current used size of the three stacks, in elements.  */
      YYPTRDIFF_T yysize = yyssp - yyss + 1;

# if defined yyoverflow
      {
        /* Give user a chance to reallocate the stack.  Use copies of
           these so that the &'s don't force the real ones into
           memory.  */
        yy_state_t *yyss1 = yyss;
        YYSTYPE *yyvs1 = yyvs;

        /* Each stack pointer address is followed by the size of the
           data in use in that stack, in bytes.  This used to be a
           conditional around just the two extra args, but that might
           be undefined if yyoverflow is a macro.  */
        yyoverflow (YY_("memory exhausted"),
                    &yyss1, yysize * YYSIZEOF (*yyssp),
                    &yyvs1, yysize * YYSIZEOF (*yyvsp),
                    &yystacksize);
        yyss = yyss1;
        yyvs = yyvs1;
      }
# else /* defined YYSTACK_RELOCATE */
      /* Extend the stack our own way.  */
      if (YYMAXDEPTH <= yystacksize)
        YYNOMEM;
      yystacksize *= 2;
      if (YYMAXDEPTH < yystacksize)
        yystacksize = YYMAXDEPTH;

      {
        yy_state_t *yyss1 = yyss;
        union yyalloc *yyptr =
          YY_CAST (union yyalloc *,
                   YYSTACK_ALLOC (YY_CAST (YYSIZE_T, YYSTACK_BYTES (yystacksize))));
        if (! yyptr)
          YYNOMEM;
        YYSTACK_RELOCATE (yyss_alloc, yyss);
        YYSTACK_RELOCATE (yyvs_alloc, yyvs);
#  undef YYSTACK_RELOCATE
        if (yyss1 != yyssa)
          YYSTACK_FREE (yyss1);
      }
# endif

      yyssp = yyss + yysize - 1;
      yyvsp = yyvs + yysize - 1;

      YY_IGNORE_USELESS_CAST_BEGIN
      YYDPRINTF ((stderr, "Stack size increased to %ld\n",
                  YY_CAST (long, yystacksize)));
      YY_IGNORE_USELESS_CAST_END

      if (yyss + yystacksize - 1 <= yyssp)
        YYABORT;
    }
#endif /* !defined yyoverflow && !defined YYSTACK_RELOCATE */


  if (yystate == YYFINAL)
    YYACCEPT;

  goto yybackup;


/*-----------.
| yybackup.  |
`-----------*/
yybackup:
  /* Do appropriate processing given the current state.  Read a
     lookahead token if we need one and don't already have one.  */

  /* First try to decide what to do without reference to lookahead token.  */
  yyn = yypact[yystate];
  if (yypact_value_is_default (yyn))
    goto yydefault;

  /* Not known => get a lookahead token if don't already have one.  */

  /* YYCHAR is either empty, or end-of-input, or a valid lookahead.  */
  if (yychar == MCY_EMPTY)
    {
      YYDPRINTF ((stderr, "Reading a token\n"));
      yychar = yylex ();
    }

  if (yychar <= MCY_EOF)
    {
      yychar = MCY_EOF;
      yytoken = YYSYMBOL_YYEOF;
      YYDPRINTF ((stderr, "Now at end of input.\n"));
    }
  else if (yychar == MCY_error)
    {
      /* The scanner already issued an error message, process directly
         to error recovery.  But do not keep the error token as
         lookahead, it is too special and may lead us to an endless
         loop in error recovery. */
      yychar = MCY_UNDEF;
      yytoken = YYSYMBOL_YYerror;
      goto yyerrlab1;
    }
  else
    {
      yytoken = YYTRANSLATE (yychar);
      YY_SYMBOL_PRINT ("Next token is", yytoken, &yylval, &yylloc);
    }

  /* If the proper action on seeing token YYTOKEN is to reduce or to
     detect an error, take that action.  */
  yyn += yytoken;
  if (yyn < 0 || YYLAST < yyn || yycheck[yyn] != yytoken)
    goto yydefault;
  yyn = yytable[yyn];
  if (yyn <= 0)
    {
      if (yytable_value_is_error (yyn))
        goto yyerrlab;
      yyn = -yyn;
      goto yyreduce;
    }

  /* Count tokens shifted since error; after three, turn off error
     status.  */
  if (yyerrstatus)
    yyerrstatus--;

  /* Shift the lookahead token.  */
  YY_SYMBOL_PRINT ("Shifting", yytoken, &yylval, &yylloc);
  yystate = yyn;
  YY_IGNORE_MAYBE_UNINITIALIZED_BEGIN
  *++yyvsp = yylval;
  YY_IGNORE_MAYBE_UNINITIALIZED_END

  /* Discard the shifted token.  */
  yychar = MCY_EMPTY;
  goto yynewstate;


/*-----------------------------------------------------------.
| yydefault -- do the default action for the current state.  |
`-----------------------------------------------------------*/
yydefault:
  yyn = yydefact[yystate];
  if (yyn == 0)
    goto yyerrlab;
  goto yyreduce;


/*-----------------------------.
| yyreduce -- do a reduction.  |
`-----------------------------*/
yyreduce:
  /* yyn is the number of a rule to reduce with.  */
  yylen = yyr2[yyn];

  /* If YYLEN is nonzero, implement the default value of the action:
     '$$ = $1'.

     Otherwise, the following line sets YYVAL to garbage.
     This behavior is undocumented and Bison
     users should not rely upon it.  Assigning to YYVAL
     unconditionally makes the parser a bit smaller, and it avoids a
     GCC warning that YYVAL may be used uninitialized.  */
  yyval = yyvsp[1-yylen];


  YY_REDUCE_PRINT (yyn);
  switch (yyn)
    {
  case 2: /* file: items  */
#line 125 "mcy.y"
                {
		if(!check_languages(nodehead))
			xyyerror("No messages defined\n");
		lanblockhead = block_messages(nodehead);
	}
#line 1295 "mcy.tab.c"
    break;

  case 6: /* decl: msg  */
#line 137 "mcy.y"
                        { add_node(nd_msg, (yyvsp[0].msg)); }
#line 1301 "mcy.tab.c"
    break;

  case 7: /* decl: tCOMMENT  */
#line 138 "mcy.y"
                        { add_node(nd_comment, (yyvsp[0].str)); }
#line 1307 "mcy.tab.c"
    break;

  case 8: /* decl: error  */
#line 139 "mcy.y"
                        { xyyerror(err_syntax); /* `Catch all' error */ }
#line 1313 "mcy.tab.c"
    break;

  case 10: /* global: tSEVNAMES '=' '(' smaps error  */
#line 143 "mcy.y"
                                        { xyyerror(err_pclose); }
#line 1319 "mcy.tab.c"
    break;

  case 11: /* global: tSEVNAMES '=' error  */
#line 144 "mcy.y"
                                        { xyyerror(err_popen); }
#line 1325 "mcy.tab.c"
    break;

  case 12: /* global: tSEVNAMES error  */
#line 145 "mcy.y"
                                        { xyyerror(err_assign); }
#line 1331 "mcy.tab.c"
    break;

  case 14: /* global: tFACNAMES '=' '(' fmaps error  */
#line 147 "mcy.y"
                                        { xyyerror(err_pclose); }
#line 1337 "mcy.tab.c"
    break;

  case 15: /* global: tFACNAMES '=' error  */
#line 148 "mcy.y"
                                        { xyyerror(err_popen); }
#line 1343 "mcy.tab.c"
    break;

  case 16: /* global: tFACNAMES error  */
#line 149 "mcy.y"
                                        { xyyerror(err_assign); }
#line 1349 "mcy.tab.c"
    break;

  case 18: /* global: tLANNAMES '=' '(' lmaps error  */
#line 151 "mcy.y"
                                        { xyyerror(err_pclose); }
#line 1355 "mcy.tab.c"
    break;

  case 19: /* global: tLANNAMES '=' error  */
#line 152 "mcy.y"
                                        { xyyerror(err_popen); }
#line 1361 "mcy.tab.c"
    break;

  case 20: /* global: tLANNAMES error  */
#line 153 "mcy.y"
                                        { xyyerror(err_assign); }
#line 1367 "mcy.tab.c"
    break;

  case 22: /* global: tCODEPAGE '=' '(' cmaps error  */
#line 155 "mcy.y"
                                        { xyyerror(err_pclose); }
#line 1373 "mcy.tab.c"
    break;

  case 23: /* global: tCODEPAGE '=' error  */
#line 156 "mcy.y"
                                        { xyyerror(err_popen); }
#line 1379 "mcy.tab.c"
    break;

  case 24: /* global: tCODEPAGE error  */
#line 157 "mcy.y"
                                        { xyyerror(err_assign); }
#line 1385 "mcy.tab.c"
    break;

  case 25: /* global: tTYPEDEF '=' tIDENT  */
#line 158 "mcy.y"
                                        { cast = (yyvsp[0].str); }
#line 1391 "mcy.tab.c"
    break;

  case 26: /* global: tTYPEDEF '=' error  */
#line 159 "mcy.y"
                                        { xyyerror(err_number); }
#line 1397 "mcy.tab.c"
    break;

  case 27: /* global: tTYPEDEF error  */
#line 160 "mcy.y"
                                        { xyyerror(err_assign); }
#line 1403 "mcy.tab.c"
    break;

  case 28: /* global: tBASE '=' tNUMBER  */
#line 161 "mcy.y"
                                        {
		switch(base)
		{
		case 8:
		case 10:
		case 16:
			base = (yyvsp[0].num);
			break;
		default:
			xyyerror("Numberbase must be 8, 10 or 16\n");
		}
	}
#line 1420 "mcy.tab.c"
    break;

  case 29: /* global: tBASE '=' error  */
#line 173 "mcy.y"
                                        { xyyerror(err_number); }
#line 1426 "mcy.tab.c"
    break;

  case 30: /* global: tBASE error  */
#line 174 "mcy.y"
                                        { xyyerror(err_assign); }
#line 1432 "mcy.tab.c"
    break;

  case 33: /* smaps: error  */
#line 182 "mcy.y"
                        { xyyerror(err_ident); }
#line 1438 "mcy.tab.c"
    break;

  case 34: /* smap: token '=' tNUMBER alias  */
#line 185 "mcy.y"
                                  {
		(yyvsp[-3].tok)->token = (yyvsp[-1].num);
		(yyvsp[-3].tok)->alias = (yyvsp[0].str);
		if((yyvsp[-1].num) & (~0x3))
			xyyerror("Severity value out of range (0x%08x > 0x3)\n", (yyvsp[-1].num));
		do_add_token(tok_severity, (yyvsp[-3].tok), "severity");
	}
#line 1450 "mcy.tab.c"
    break;

  case 35: /* smap: token '=' error  */
#line 192 "mcy.y"
                                { xyyerror(err_number); }
#line 1456 "mcy.tab.c"
    break;

  case 36: /* smap: token error  */
#line 193 "mcy.y"
                                { xyyerror(err_assign); }
#line 1462 "mcy.tab.c"
    break;

  case 39: /* fmaps: error  */
#line 201 "mcy.y"
                        { xyyerror(err_ident); }
#line 1468 "mcy.tab.c"
    break;

  case 40: /* fmap: token '=' tNUMBER alias  */
#line 204 "mcy.y"
                                  {
		(yyvsp[-3].tok)->token = (yyvsp[-1].num);
		(yyvsp[-3].tok)->alias = (yyvsp[0].str);
		if((yyvsp[-1].num) & (~0xfff))
			xyyerror("Facility value out of range (0x%08x > 0xfff)\n", (yyvsp[-1].num));
		do_add_token(tok_facility, (yyvsp[-3].tok), "facility");
	}
#line 1480 "mcy.tab.c"
    break;

  case 41: /* fmap: token '=' error  */
#line 211 "mcy.y"
                                { xyyerror(err_number); }
#line 1486 "mcy.tab.c"
    break;

  case 42: /* fmap: token error  */
#line 212 "mcy.y"
                                { xyyerror(err_assign); }
#line 1492 "mcy.tab.c"
    break;

  case 43: /* alias: %empty  */
#line 215 "mcy.y"
                        { (yyval.str) = NULL; }
#line 1498 "mcy.tab.c"
    break;

  case 44: /* alias: ':' tIDENT  */
#line 216 "mcy.y"
                        { (yyval.str) = (yyvsp[0].str); }
#line 1504 "mcy.tab.c"
    break;

  case 45: /* alias: ':' error  */
#line 217 "mcy.y"
                        { xyyerror(err_ident); }
#line 1510 "mcy.tab.c"
    break;

  case 48: /* lmaps: error  */
#line 225 "mcy.y"
                        { xyyerror(err_ident); }
#line 1516 "mcy.tab.c"
    break;

  case 49: /* lmap: token '=' tNUMBER setfile ':' tFILE optcp  */
#line 228 "mcy.y"
                                                    {
		(yyvsp[-6].tok)->token = (yyvsp[-4].num);
		(yyvsp[-6].tok)->alias = (yyvsp[-1].str);
		(yyvsp[-6].tok)->codepage = (yyvsp[0].num);
		do_add_token(tok_language, (yyvsp[-6].tok), "language");
		if(!find_language((yyvsp[-4].num)) && !find_cpxlat((yyvsp[-4].num)))
			mcy_warning("Language 0x%x not built-in, using codepage %d; use explicit codepage to override\n", (yyvsp[-4].num), WMC_DEFAULT_CODEPAGE);
	}
#line 1529 "mcy.tab.c"
    break;

  case 50: /* lmap: token '=' tNUMBER setfile ':' error  */
#line 236 "mcy.y"
                                                { xyyerror("Filename expected\n"); }
#line 1535 "mcy.tab.c"
    break;

  case 51: /* lmap: token '=' tNUMBER error  */
#line 237 "mcy.y"
                                                { xyyerror(err_colon); }
#line 1541 "mcy.tab.c"
    break;

  case 52: /* lmap: token '=' error  */
#line 238 "mcy.y"
                                                { xyyerror(err_number); }
#line 1547 "mcy.tab.c"
    break;

  case 53: /* lmap: token error  */
#line 239 "mcy.y"
                                                { xyyerror(err_assign); }
#line 1553 "mcy.tab.c"
    break;

  case 54: /* optcp: %empty  */
#line 242 "mcy.y"
                        { (yyval.num) = 0; }
#line 1559 "mcy.tab.c"
    break;

  case 55: /* optcp: ':' tNUMBER  */
#line 243 "mcy.y"
                        { (yyval.num) = (yyvsp[0].num); }
#line 1565 "mcy.tab.c"
    break;

  case 56: /* optcp: ':' error  */
#line 244 "mcy.y"
                        { xyyerror("Codepage-number expected\n"); }
#line 1571 "mcy.tab.c"
    break;

  case 59: /* cmaps: error  */
#line 252 "mcy.y"
                        { xyyerror(err_ident); }
#line 1577 "mcy.tab.c"
    break;

  case 60: /* cmap: clan '=' tNUMBER ':' tNUMBER  */
#line 255 "mcy.y"
                                       {
		static const char err_nocp[] = "Codepage %d not builtin; cannot convert\n";
		if(find_cpxlat((yyvsp[-4].num)))
			xyyerror("Codepage translation already defined for language 0x%x\n", (yyvsp[-4].num));
		if((yyvsp[-2].num) && !is_valid_codepage((yyvsp[-2].num)))
			xyyerror(err_nocp, (yyvsp[-2].num));
		if((yyvsp[0].num) && !is_valid_codepage((yyvsp[0].num)))
			xyyerror(err_nocp, (yyvsp[0].num));
		add_cpxlat((yyvsp[-4].num), (yyvsp[-2].num), (yyvsp[0].num));
	}
#line 1592 "mcy.tab.c"
    break;

  case 61: /* cmap: clan '=' tNUMBER ':' error  */
#line 265 "mcy.y"
                                        { xyyerror(err_number); }
#line 1598 "mcy.tab.c"
    break;

  case 62: /* cmap: clan '=' tNUMBER error  */
#line 266 "mcy.y"
                                        { xyyerror(err_colon); }
#line 1604 "mcy.tab.c"
    break;

  case 63: /* cmap: clan '=' error  */
#line 267 "mcy.y"
                                        { xyyerror(err_number); }
#line 1610 "mcy.tab.c"
    break;

  case 64: /* cmap: clan error  */
#line 268 "mcy.y"
                                        { xyyerror(err_assign); }
#line 1616 "mcy.tab.c"
    break;

  case 65: /* clan: tNUMBER  */
#line 271 "mcy.y"
                        { (yyval.num) = (yyvsp[0].num); }
#line 1622 "mcy.tab.c"
    break;

  case 66: /* clan: tTOKEN  */
#line 272 "mcy.y"
                        {
		if((yyvsp[0].tok)->type != tok_language)
			xyyerror("Language name or code expected\n");
		(yyval.num) = (yyvsp[0].tok)->token;
	}
#line 1632 "mcy.tab.c"
    break;

  case 67: /* $@1: %empty  */
#line 282 "mcy.y"
                          { test_id((yyvsp[-1].num)); }
#line 1638 "mcy.tab.c"
    break;

  case 68: /* msg: msgid sevfacsym $@1 bodies  */
#line 282 "mcy.y"
                                                        { (yyval.msg) = complete_msg((yyvsp[0].msg), (yyvsp[-3].num)); }
#line 1644 "mcy.tab.c"
    break;

  case 69: /* msgid: tMSGID '=' id  */
#line 285 "mcy.y"
                        {
		if((yyvsp[0].num) & (~0xffff))
			xyyerror("Message ID value out of range (0x%08x > 0xffff)\n", (yyvsp[0].num));
		(yyval.num) = (yyvsp[0].num);
	}
#line 1654 "mcy.tab.c"
    break;

  case 70: /* msgid: tMSGID error  */
#line 290 "mcy.y"
                        { xyyerror(err_assign); }
#line 1660 "mcy.tab.c"
    break;

  case 71: /* id: %empty  */
#line 293 "mcy.y"
                        { (yyval.num) = ++last_id; }
#line 1666 "mcy.tab.c"
    break;

  case 72: /* id: tNUMBER  */
#line 294 "mcy.y"
                        { (yyval.num) = last_id = (yyvsp[0].num); }
#line 1672 "mcy.tab.c"
    break;

  case 73: /* id: '+' tNUMBER  */
#line 295 "mcy.y"
                        { (yyval.num) = last_id += (yyvsp[0].num); }
#line 1678 "mcy.tab.c"
    break;

  case 74: /* id: '+' error  */
#line 296 "mcy.y"
                        { xyyerror(err_number); }
#line 1684 "mcy.tab.c"
    break;

  case 75: /* sevfacsym: %empty  */
#line 299 "mcy.y"
                        { have_sev = have_fac = have_sym = 0; }
#line 1690 "mcy.tab.c"
    break;

  case 76: /* sevfacsym: sevfacsym sev  */
#line 300 "mcy.y"
                        { if(have_sev) xyyerror("Severity already defined\n"); have_sev = 1; }
#line 1696 "mcy.tab.c"
    break;

  case 77: /* sevfacsym: sevfacsym fac  */
#line 301 "mcy.y"
                        { if(have_fac) xyyerror("Facility already defined\n"); have_fac = 1; }
#line 1702 "mcy.tab.c"
    break;

  case 78: /* sevfacsym: sevfacsym sym  */
#line 302 "mcy.y"
                        { if(have_sym) xyyerror("Symbolname already defined\n"); have_sym = 1; }
#line 1708 "mcy.tab.c"
    break;

  case 79: /* sym: tSYMNAME '=' tIDENT  */
#line 305 "mcy.y"
                                { last_sym = (yyvsp[0].str); }
#line 1714 "mcy.tab.c"
    break;

  case 80: /* sym: tSYMNAME '=' error  */
#line 306 "mcy.y"
                                { xyyerror(err_ident); }
#line 1720 "mcy.tab.c"
    break;

  case 81: /* sym: tSYMNAME error  */
#line 307 "mcy.y"
                                { xyyerror(err_assign); }
#line 1726 "mcy.tab.c"
    break;

  case 82: /* sev: tSEVERITY '=' token  */
#line 310 "mcy.y"
                                {
		struct token *tok = lookup_token((yyvsp[0].tok)->name);
		if(!tok)
			xyyerror("Undefined severityname\n");
		if(tok->type != tok_severity)
			xyyerror("Identifier is not of class 'severity'\n");
		last_sev = tok->token;
	}
#line 1739 "mcy.tab.c"
    break;

  case 83: /* sev: tSEVERITY '=' error  */
#line 318 "mcy.y"
                                { xyyerror(err_ident); }
#line 1745 "mcy.tab.c"
    break;

  case 84: /* sev: tSEVERITY error  */
#line 319 "mcy.y"
                                { xyyerror(err_assign); }
#line 1751 "mcy.tab.c"
    break;

  case 85: /* fac: tFACILITY '=' token  */
#line 322 "mcy.y"
                                {
		struct token *tok = lookup_token((yyvsp[0].tok)->name);
		if(!tok)
			xyyerror("Undefined facilityname\n");
		if(tok->type != tok_facility)
			xyyerror("Identifier is not of class 'facility'\n");
		last_fac = tok->token;
	}
#line 1764 "mcy.tab.c"
    break;

  case 86: /* fac: tFACILITY '=' error  */
#line 330 "mcy.y"
                                { xyyerror(err_ident); }
#line 1770 "mcy.tab.c"
    break;

  case 87: /* fac: tFACILITY error  */
#line 331 "mcy.y"
                                { xyyerror(err_assign); }
#line 1776 "mcy.tab.c"
    break;

  case 88: /* bodies: body  */
#line 337 "mcy.y"
                        { (yyval.msg) = add_lanmsg(NULL, (yyvsp[0].lmp)); }
#line 1782 "mcy.tab.c"
    break;

  case 89: /* bodies: bodies body  */
#line 338 "mcy.y"
                        { (yyval.msg) = add_lanmsg((yyvsp[-1].msg), (yyvsp[0].lmp)); }
#line 1788 "mcy.tab.c"
    break;

  case 90: /* bodies: error  */
#line 339 "mcy.y"
                        { xyyerror("'Language=...' (start of message text-definition) expected\n"); }
#line 1794 "mcy.tab.c"
    break;

  case 91: /* body: lang setline lines tMSGEND  */
#line 342 "mcy.y"
                                        { (yyval.lmp) = new_lanmsg(&(yyvsp[-3].lcp), (yyvsp[-1].str)); }
#line 1800 "mcy.tab.c"
    break;

  case 92: /* lang: tLANGUAGE setnl '=' token tNL  */
#line 350 "mcy.y"
                                        {
		struct token *tok = lookup_token((yyvsp[-1].tok)->name);
		struct cp_xlat *cpx;
		if(!tok)
			xyyerror("Undefined language\n");
		if(tok->type != tok_language)
			xyyerror("Identifier is not of class 'language'\n");
		if((cpx = find_cpxlat(tok->token)))
		{
			set_codepage((yyval.lcp).codepage = cpx->cpin);
		}
		else if(!tok->codepage)
		{
			const struct language *lan = find_language(tok->token);
			if(!lan)
			{
				/* Just set default; warning was given while parsing languagenames */
				set_codepage((yyval.lcp).codepage = WMC_DEFAULT_CODEPAGE);
			}
			else
			{
				/* The default seems to be to use the DOS codepage... */
				set_codepage((yyval.lcp).codepage = lan->doscp);
			}
		}
		else
			set_codepage((yyval.lcp).codepage = tok->codepage);
		(yyval.lcp).language = tok->token;
	}
#line 1834 "mcy.tab.c"
    break;

  case 93: /* lang: tLANGUAGE setnl '=' token error  */
#line 379 "mcy.y"
                                                { xyyerror("Missing newline\n"); }
#line 1840 "mcy.tab.c"
    break;

  case 94: /* lang: tLANGUAGE setnl '=' error  */
#line 380 "mcy.y"
                                                { xyyerror(err_ident); }
#line 1846 "mcy.tab.c"
    break;

  case 95: /* lang: tLANGUAGE error  */
#line 381 "mcy.y"
                                                { xyyerror(err_assign); }
#line 1852 "mcy.tab.c"
    break;

  case 96: /* lines: tLINE  */
#line 384 "mcy.y"
                        { (yyval.str) = (yyvsp[0].str); }
#line 1858 "mcy.tab.c"
    break;

  case 97: /* lines: lines tLINE  */
#line 385 "mcy.y"
                        { (yyval.str) = merge((yyvsp[-1].str), (yyvsp[0].str)); }
#line 1864 "mcy.tab.c"
    break;

  case 98: /* lines: error  */
#line 386 "mcy.y"
                        { xyyerror(err_msg); }
#line 1870 "mcy.tab.c"
    break;

  case 99: /* lines: lines error  */
#line 387 "mcy.y"
                        { xyyerror(err_msg); }
#line 1876 "mcy.tab.c"
    break;

  case 100: /* token: tIDENT  */
#line 393 "mcy.y"
                        { (yyval.tok) = xmalloc(sizeof(struct token)); memset((yyval.tok),0,sizeof(*(yyval.tok))); (yyval.tok)->name = (yyvsp[0].str); }
#line 1882 "mcy.tab.c"
    break;

  case 101: /* token: tTOKEN  */
#line 394 "mcy.y"
                        { (yyval.tok) = (yyvsp[0].tok); }
#line 1888 "mcy.tab.c"
    break;

  case 102: /* setnl: %empty  */
#line 397 "mcy.y"
                        { want_nl = 1; }
#line 1894 "mcy.tab.c"
    break;

  case 103: /* setline: %empty  */
#line 400 "mcy.y"
                        { want_line = 1; }
#line 1900 "mcy.tab.c"
    break;

  case 104: /* setfile: %empty  */
#line 403 "mcy.y"
                        { want_file = 1; }
#line 1906 "mcy.tab.c"
    break;


#line 1910 "mcy.tab.c"

      default: break;
    }
  /* User semantic actions sometimes alter yychar, and that requires
     that yytoken be updated with the new translation.  We take the
     approach of translating immediately before every use of yytoken.
     One alternative is translating here after every semantic action,
     but that translation would be missed if the semantic action invokes
     YYABORT, YYACCEPT, or YYERROR immediately after altering yychar or
     if it invokes YYBACKUP.  In the case of YYABORT or YYACCEPT, an
     incorrect destructor might then be invoked immediately.  In the
     case of YYERROR or YYBACKUP, subsequent parser actions might lead
     to an incorrect destructor call or verbose syntax error message
     before the lookahead is translated.  */
  YY_SYMBOL_PRINT ("-> $$ =", YY_CAST (yysymbol_kind_t, yyr1[yyn]), &yyval, &yyloc);

  YYPOPSTACK (yylen);
  yylen = 0;

  *++yyvsp = yyval;

  /* Now 'shift' the result of the reduction.  Determine what state
     that goes to, based on the state we popped back to and the rule
     number reduced by.  */
  {
    const int yylhs = yyr1[yyn] - YYNTOKENS;
    const int yyi = yypgoto[yylhs] + *yyssp;
    yystate = (0 <= yyi && yyi <= YYLAST && yycheck[yyi] == *yyssp
               ? yytable[yyi]
               : yydefgoto[yylhs]);
  }

  goto yynewstate;


/*--------------------------------------.
| yyerrlab -- here on detecting error.  |
`--------------------------------------*/
yyerrlab:
  /* Make sure we have latest lookahead translation.  See comments at
     user semantic actions for why this is necessary.  */
  yytoken = yychar == MCY_EMPTY ? YYSYMBOL_YYEMPTY : YYTRANSLATE (yychar);
  /* If not already recovering from an error, report this error.  */
  if (!yyerrstatus)
    {
      ++yynerrs;
      yyerror (YY_("syntax error"));
    }

  if (yyerrstatus == 3)
    {
      /* If just tried and failed to reuse lookahead token after an
         error, discard it.  */

      if (yychar <= MCY_EOF)
        {
          /* Return failure if at end of input.  */
          if (yychar == MCY_EOF)
            YYABORT;
        }
      else
        {
          yydestruct ("Error: discarding",
                      yytoken, &yylval);
          yychar = MCY_EMPTY;
        }
    }

  /* Else will try to reuse lookahead token after shifting the error
     token.  */
  goto yyerrlab1;


/*---------------------------------------------------.
| yyerrorlab -- error raised explicitly by YYERROR.  |
`---------------------------------------------------*/
yyerrorlab:
  /* Pacify compilers when the user code never invokes YYERROR and the
     label yyerrorlab therefore never appears in user code.  */
  if (0)
    YYERROR;
  ++yynerrs;

  /* Do not reclaim the symbols of the rule whose action triggered
     this YYERROR.  */
  YYPOPSTACK (yylen);
  yylen = 0;
  YY_STACK_PRINT (yyss, yyssp);
  yystate = *yyssp;
  goto yyerrlab1;


/*-------------------------------------------------------------.
| yyerrlab1 -- common code for both syntax error and YYERROR.  |
`-------------------------------------------------------------*/
yyerrlab1:
  yyerrstatus = 3;      /* Each real token shifted decrements this.  */

  /* Pop stack until we find a state that shifts the error token.  */
  for (;;)
    {
      yyn = yypact[yystate];
      if (!yypact_value_is_default (yyn))
        {
          yyn += YYSYMBOL_YYerror;
          if (0 <= yyn && yyn <= YYLAST && yycheck[yyn] == YYSYMBOL_YYerror)
            {
              yyn = yytable[yyn];
              if (0 < yyn)
                break;
            }
        }

      /* Pop the current state because it cannot handle the error token.  */
      if (yyssp == yyss)
        YYABORT;


      yydestruct ("Error: popping",
                  YY_ACCESSING_SYMBOL (yystate), yyvsp);
      YYPOPSTACK (1);
      yystate = *yyssp;
      YY_STACK_PRINT (yyss, yyssp);
    }

  YY_IGNORE_MAYBE_UNINITIALIZED_BEGIN
  *++yyvsp = yylval;
  YY_IGNORE_MAYBE_UNINITIALIZED_END


  /* Shift the error token.  */
  YY_SYMBOL_PRINT ("Shifting", YY_ACCESSING_SYMBOL (yyn), yyvsp, yylsp);

  yystate = yyn;
  goto yynewstate;


/*-------------------------------------.
| yyacceptlab -- YYACCEPT comes here.  |
`-------------------------------------*/
yyacceptlab:
  yyresult = 0;
  goto yyreturnlab;


/*-----------------------------------.
| yyabortlab -- YYABORT comes here.  |
`-----------------------------------*/
yyabortlab:
  yyresult = 1;
  goto yyreturnlab;


/*-----------------------------------------------------------.
| yyexhaustedlab -- YYNOMEM (memory exhaustion) comes here.  |
`-----------------------------------------------------------*/
yyexhaustedlab:
  yyerror (YY_("memory exhausted"));
  yyresult = 2;
  goto yyreturnlab;


/*----------------------------------------------------------.
| yyreturnlab -- parsing is finished, clean up and return.  |
`----------------------------------------------------------*/
yyreturnlab:
  if (yychar != MCY_EMPTY)
    {
      /* Make sure we have latest lookahead translation.  See comments at
         user semantic actions for why this is necessary.  */
      yytoken = YYTRANSLATE (yychar);
      yydestruct ("Cleanup: discarding lookahead",
                  yytoken, &yylval);
    }
  /* Do not reclaim the symbols of the rule whose action triggered
     this YYABORT or YYACCEPT.  */
  YYPOPSTACK (yylen);
  YY_STACK_PRINT (yyss, yyssp);
  while (yyssp != yyss)
    {
      yydestruct ("Cleanup: popping",
                  YY_ACCESSING_SYMBOL (+*yyssp), yyvsp);
      YYPOPSTACK (1);
    }
#ifndef yyoverflow
  if (yyss != yyssa)
    YYSTACK_FREE (yyss);
#endif

  return yyresult;
}

#line 406 "mcy.y"


static WCHAR *merge(WCHAR *s1, WCHAR *s2)
{
	int l1 = unistrlen(s1);
	int l2 = unistrlen(s2);
	s1 = xrealloc(s1, (l1 + l2 + 1) * sizeof(*s1));
	unistrcpy(s1+l1, s2);
	free(s2);
	return s1;
}

static void do_add_token(enum tok_enum type, struct token *tok, const char *code)
{
	struct token *tp = lookup_token(tok->name);
	if(tp)
	{
		if(tok->type != type)
			mcy_warning("Type change in token\n");
		if(tp != tok)
			xyyerror("Overlapping token not the same\n");
		/* else its already defined and changed */
		if(tok->fixed)
			xyyerror("Redefinition of %s\n", code);
		tok->fixed = 1;
	}
	else
	{
		add_token(type, tok->name, tok->token, tok->codepage, tok->alias, 1);
		free(tok);
	}
}

static struct lanmsg *new_lanmsg(struct lan_cp *lcp, WCHAR *msg)
{
	struct lanmsg *lmp = xmalloc(sizeof(*lmp));
	lmp->lan = lcp->language;
	lmp->cp  = lcp->codepage;
	lmp->msg = msg;
	lmp->len = unistrlen(msg) + 1;	/* Include termination */
	lmp->file = input_name;
	lmp->line = line_number;
	if(lmp->len > 4096)
		mcy_warning("Message exceptionally long; might be a missing termination\n");
	return lmp;
}

static struct msg *add_lanmsg(struct msg *msg, struct lanmsg *lanmsg)
{
	int i;
	if(!msg)
	{
		msg = xmalloc(sizeof(*msg));
		memset( msg, 0, sizeof(*msg) );
	}
	msg->msgs = xrealloc(msg->msgs, (msg->nmsgs+1) * sizeof(*(msg->msgs)));
	msg->msgs[msg->nmsgs] = lanmsg;
	msg->nmsgs++;
	for(i = 0; i < msg->nmsgs-1; i++)
	{
		if(msg->msgs[i]->lan == lanmsg->lan)
			xyyerror("Message for language 0x%x already defined\n", lanmsg->lan);
	}
	return msg;
}

static int sort_lanmsg(const void *p1, const void *p2)
{
	return (*(const struct lanmsg * const *)p1)->lan - (*(const struct lanmsg * const*)p2)->lan;
}

static struct msg *complete_msg(struct msg *mp, int id)
{
	assert(mp != NULL);
	mp->id = id;
	if(have_sym)
		mp->sym = last_sym;
	else
		xyyerror("No symbolic name defined for message id %d\n", id);
	mp->sev = last_sev;
	mp->fac = last_fac;
	qsort(mp->msgs, mp->nmsgs, sizeof(*(mp->msgs)), sort_lanmsg);
	mp->realid = id | (last_sev << 30) | (last_fac << 16);
	if(custombit)
		mp->realid |= 1 << 29;
	mp->base = base;
	mp->cast = cast;
	return mp;
}

static void add_node(enum node_type type, void *p)
{
	struct node *ndp = xmalloc(sizeof(*ndp));
	memset( ndp, 0, sizeof(*ndp) );
	ndp->type = type;
	ndp->u.all = p;

	if(nodetail)
	{
		ndp->prev = nodetail;
		nodetail->next = ndp;
		nodetail = ndp;
	}
	else
	{
		nodehead = nodetail = ndp;
	}
}

static void test_id(int id)
{
	struct node *ndp;
	for(ndp = nodehead; ndp; ndp = ndp->next)
	{
		if(ndp->type != nd_msg)
			continue;
		if(ndp->u.msg->id == id && ndp->u.msg->sev == last_sev && ndp->u.msg->fac == last_fac)
			xyyerror("MessageId %d with facility 0x%x and severity 0x%x already defined\n", id, last_fac, last_sev);
	}
}

static int check_languages(struct node *head)
{
	static const char err_missing[] = "Missing definition for language 0x%x; MessageID %d, facility 0x%x, severity 0x%x\n";
	struct node *ndp;
	int nm = 0;
	struct msg *msg = NULL;

	for(ndp = head; ndp; ndp = ndp->next)
	{
		if(ndp->type != nd_msg)
			continue;
		if(!nm)
		{
			msg = ndp->u.msg;
		}
		else
		{
			int i;
			struct msg *m1;
			struct msg *m2;
			if(ndp->u.msg->nmsgs > msg->nmsgs)
			{
				m1 = ndp->u.msg;
				m2 = msg;
			}
			else
			{
				m1 = msg;
				m2 = ndp->u.msg;
			}

			for(i = 0; i < m1->nmsgs; i++)
			{
				if(i > m2->nmsgs)
					error(err_missing, m1->msgs[i]->lan, m2->id, m2->fac, m2->sev);
				else if(m1->msgs[i]->lan < m2->msgs[i]->lan)
					error(err_missing, m1->msgs[i]->lan, m2->id, m2->fac, m2->sev);
				else if(m1->msgs[i]->lan > m2->msgs[i]->lan)
					error(err_missing, m2->msgs[i]->lan, m1->id, m1->fac, m1->sev);
			}
		}
		nm++;
	}
	return nm;
}

#define MSGRID(x)	((*(const struct msg * const*)(x))->realid)
static int sort_msg(const void *p1, const void *p2)
{
	return MSGRID(p1) > MSGRID(p2) ? 1 : (MSGRID(p1) == MSGRID(p2) ? 0 : -1);
}

/*
 * block_messages() basically transposes the messages
 * from ID/language based list to a language/ID
 * based list.
 */
static struct lan_blk *block_messages(struct node *head)
{
	struct lan_blk *lbp;
	struct lan_blk *lblktail = NULL;
	struct lan_blk *lblkhead = NULL;
	struct msg **msgtab = NULL;
	struct node *ndp;
	int nmsg = 0;
	int i;
	int nl;
	int factor = 2;

	for(ndp = head; ndp; ndp = ndp->next)
	{
		if(ndp->type != nd_msg)
			continue;
		msgtab = xrealloc(msgtab, (nmsg+1) * sizeof(*msgtab));
		msgtab[nmsg++] = ndp->u.msg;
	}

	assert(nmsg != 0);
	qsort(msgtab, nmsg, sizeof(*msgtab), sort_msg);

	for(nl = 0; nl < msgtab[0]->nmsgs; nl++)	/* This should be equal for all after check_languages() */
	{
		lbp = xmalloc(sizeof(*lbp));
		memset( lbp, 0, sizeof(*lbp) );
		if(!lblktail)
		{
			lblkhead = lblktail = lbp;
		}
		else
		{
			lblktail->next = lbp;
			lbp->prev = lblktail;
			lblktail = lbp;
		}
		lbp->nblk = 1;
		lbp->blks = xmalloc(sizeof(*lbp->blks));
		lbp->blks[0].idlo = msgtab[0]->realid;
		lbp->blks[0].idhi = msgtab[0]->realid;
		/* The plus 4 is the entry header; (+3)&~3 is DWORD alignment */
		lbp->blks[0].size = ((factor * msgtab[0]->msgs[nl]->len + 3) & ~3) + 4;
		lbp->blks[0].msgs = xmalloc(sizeof(*lbp->blks[0].msgs));
		lbp->blks[0].nmsg = 1;
		lbp->blks[0].msgs[0] = msgtab[0]->msgs[nl];
		lbp->lan = msgtab[0]->msgs[nl]->lan;

		for(i = 1; i < nmsg; i++)
		{
			struct block *blk = &(lbp->blks[lbp->nblk-1]);
			if(msgtab[i]->realid == blk->idhi+1)
			{
				blk->size += ((factor * msgtab[i]->msgs[nl]->len + 3) & ~3) + 4;
				blk->idhi++;
				blk->msgs = xrealloc(blk->msgs, (blk->nmsg+1) * sizeof(*blk->msgs));
				blk->msgs[blk->nmsg++] = msgtab[i]->msgs[nl];
			}
			else
			{
				lbp->nblk++;
				lbp->blks = xrealloc(lbp->blks, lbp->nblk * sizeof(*lbp->blks));
				blk = &(lbp->blks[lbp->nblk-1]);
				blk->idlo = msgtab[i]->realid;
				blk->idhi = msgtab[i]->realid;
				blk->size = ((factor * msgtab[i]->msgs[nl]->len + 3) & ~3) + 4;
				blk->msgs = xmalloc(sizeof(*blk->msgs));
				blk->nmsg = 1;
				blk->msgs[0] = msgtab[i]->msgs[nl];
			}
		}
	}
	free(msgtab);
	return lblkhead;
}

static int sc_xlat(const void *p1, const void *p2)
{
	return ((const struct cp_xlat *)p1)->lan - ((const struct cp_xlat *)p2)->lan;
}

static void add_cpxlat(int lan, int cpin, int cpout)
{
	cpxlattab = xrealloc(cpxlattab, (ncpxlattab+1) * sizeof(*cpxlattab));
	cpxlattab[ncpxlattab].lan   = lan;
	cpxlattab[ncpxlattab].cpin  = cpin;
	cpxlattab[ncpxlattab].cpout = cpout;
	ncpxlattab++;
	qsort(cpxlattab, ncpxlattab, sizeof(*cpxlattab), sc_xlat);
}

static struct cp_xlat *find_cpxlat(int lan)
{
	struct cp_xlat t;

	if(!cpxlattab) return NULL;

	t.lan = lan;
	return (struct cp_xlat *)bsearch(&t, cpxlattab, ncpxlattab, sizeof(*cpxlattab), sc_xlat);
}
