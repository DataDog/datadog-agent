//go:build re2_cgo

/*
  Header  file  for  CRE2, a  C  language  wrapper  for RE2:  a  regular
  expressions library by Google.

  Copyright (c) 2012, 2015 Marco Maggi <mrc.mgg@gmail.com>
  Copyright (c) 2011 Keegan McAllister
  All rights reserved.

  For the license notice see the NOTICE file.
*/

#ifndef CRE2_H
#define CRE2_H 1


/** --------------------------------------------------------------------
 ** Headers.
 ** ----------------------------------------------------------------- */

#ifdef __cplusplus
extern "C" {
#else
#include <stdbool.h>
#endif

#include <stdint.h>

#ifndef cre2_decl
#  define cre2_decl	extern
#endif


/** --------------------------------------------------------------------
 ** Regular expressions configuration options.
 ** ----------------------------------------------------------------- */

typedef void cre2_options_t;

typedef enum cre2_encoding_t {
  CRE2_UNKNOWN	= 0,	/* should never happen */
  CRE2_UTF8	= 1,
  CRE2_Latin1	= 2
} cre2_encoding_t;

cre2_decl cre2_options_t *cre2_opt_new		(void);
cre2_decl void		  cre2_opt_delete	(cre2_options_t *opt);

cre2_decl void cre2_opt_set_posix_syntax	(cre2_options_t *opt, int flag);
cre2_decl void cre2_opt_set_longest_match	(cre2_options_t *opt, int flag);
cre2_decl void cre2_opt_set_log_errors		(cre2_options_t *opt, int flag);
cre2_decl void cre2_opt_set_literal		(cre2_options_t *opt, int flag);
cre2_decl void cre2_opt_set_never_nl		(cre2_options_t *opt, int flag);
cre2_decl void cre2_opt_set_dot_nl		(cre2_options_t *opt, int flag);
cre2_decl void cre2_opt_set_never_capture	(cre2_options_t *opt, int flag);
cre2_decl void cre2_opt_set_case_sensitive	(cre2_options_t *opt, int flag);
cre2_decl void cre2_opt_set_perl_classes	(cre2_options_t *opt, int flag);
cre2_decl void cre2_opt_set_word_boundary	(cre2_options_t *opt, int flag);
cre2_decl void cre2_opt_set_one_line		(cre2_options_t *opt, int flag);
cre2_decl void cre2_opt_set_max_mem		(cre2_options_t *opt, int64_t m);
cre2_decl void cre2_opt_set_encoding		(cre2_options_t *opt, cre2_encoding_t enc);
cre2_decl void cre2_opt_set_latin1_encoding		(cre2_options_t *opt);

cre2_decl int cre2_opt_posix_syntax		(cre2_options_t *opt);
cre2_decl int cre2_opt_longest_match		(cre2_options_t *opt);
cre2_decl int cre2_opt_log_errors		(cre2_options_t *opt);
cre2_decl int cre2_opt_literal			(cre2_options_t *opt);
cre2_decl int cre2_opt_never_nl			(cre2_options_t *opt);
cre2_decl int cre2_opt_dot_nl			(cre2_options_t *opt);
cre2_decl int cre2_opt_never_capture		(cre2_options_t *opt);
cre2_decl int cre2_opt_case_sensitive		(cre2_options_t *opt);
cre2_decl int cre2_opt_perl_classes		(cre2_options_t *opt);
cre2_decl int cre2_opt_word_boundary		(cre2_options_t *opt);
cre2_decl int cre2_opt_one_line			(cre2_options_t *opt);
cre2_decl int64_t cre2_opt_max_mem		(cre2_options_t *opt);
cre2_decl cre2_encoding_t cre2_opt_encoding	(cre2_options_t *opt);


/** --------------------------------------------------------------------
 ** Precompiled regular expressions.
 ** ----------------------------------------------------------------- */

typedef struct cre2_string_t {
  const char *	data;
  int		length;
} cre2_string_t;

typedef void	cre2_regexp_t;

typedef struct cre2_named_groups_iter_t cre2_named_groups_iter_t;

/* This definition  must be  kept in sync  with the definition  of "enum
   ErrorCode" in the file "re2.h" of the original RE2 distribution. */
typedef enum cre2_error_code_t {
  CRE2_NO_ERROR = 0,
  CRE2_ERROR_INTERNAL,		/* unexpected error */
  /* parse errors */
  CRE2_ERROR_BAD_ESCAPE,	/* bad escape sequence */
  CRE2_ERROR_BAD_CHAR_CLASS,	/* bad character class */
  CRE2_ERROR_BAD_CHAR_RANGE,	/* bad character class range */
  CRE2_ERROR_MISSING_BRACKET,	/* missing closing ] */
  CRE2_ERROR_MISSING_PAREN,	/* missing closing ) */
  CRE2_ERROR_TRAILING_BACKSLASH,/* trailing \ at end of regexp */
  CRE2_ERROR_REPEAT_ARGUMENT,	/* repeat argument missing, e.g. "*" */
  CRE2_ERROR_REPEAT_SIZE,	/* bad repetition argument */
  CRE2_ERROR_REPEAT_OP,		/* bad repetition operator */
  CRE2_ERROR_BAD_PERL_OP,	/* bad perl operator */
  CRE2_ERROR_BAD_UTF8,		/* invalid UTF-8 in regexp */
  CRE2_ERROR_BAD_NAMED_CAPTURE,	/* bad named capture group */
  CRE2_ERROR_PATTERN_TOO_LARGE,	/* pattern too large (compile failed) */
} cre2_error_code_t;

/* construction and destruction */
cre2_decl cre2_regexp_t *  cre2_new	(const char *pattern, int pattern_len,
				 const cre2_options_t *opt);
cre2_decl void    cre2_delete	(cre2_regexp_t *re);

/* regular expression inspection */
cre2_decl const char * cre2_pattern	(const cre2_regexp_t *re);
cre2_decl int cre2_error_code		(const cre2_regexp_t *re);
cre2_decl int cre2_num_capturing_groups	(const cre2_regexp_t *re);
cre2_decl int cre2_program_size		(const cre2_regexp_t *re);

/* named capture information */
cre2_decl int cre2_find_named_capturing_groups  (const cre2_regexp_t *re, const char *name);
cre2_decl cre2_named_groups_iter_t * cre2_named_groups_iter_new(const cre2_regexp_t *re);
cre2_decl bool cre2_named_groups_iter_next(cre2_named_groups_iter_t* iter, const char ** name, int *index);
cre2_decl void cre2_named_groups_iter_delete(cre2_named_groups_iter_t *iter);

/* invalidated by further re use */
cre2_decl const char *cre2_error_string(const cre2_regexp_t *re);
cre2_decl const char *cre2_error_arg(const cre2_regexp_t *re);


/** --------------------------------------------------------------------
 ** Main matching functions.
 ** ----------------------------------------------------------------- */

typedef enum cre2_anchor_t {
  CRE2_UNANCHORED   = 1,
  CRE2_ANCHOR_START = 2,
  CRE2_ANCHOR_BOTH  = 3
} cre2_anchor_t;

typedef struct cre2_range_t {
  long	start;	/* inclusive start index for bytevector */
  long	past;	/* exclusive end index for bytevector */
} cre2_range_t;

cre2_decl int cre2_match	(const cre2_regexp_t * re,
				 const char * text, int textlen,
				 int startpos, int endpos, cre2_anchor_t anchor,
				 cre2_string_t * match, int nmatch);

cre2_decl int cre2_easy_match	(const char * pattern, int pattern_len,
				 const char * text, int text_len,
				 cre2_string_t * match, int nmatch);

cre2_decl void cre2_strings_to_ranges (const char * text, cre2_range_t * ranges,
				       cre2_string_t * strings, int nmatch);


/** --------------------------------------------------------------------
 ** Other matching functions.
 ** ----------------------------------------------------------------- */

typedef int cre2_match_stringz_fun_t (const char * pattern, const cre2_string_t * text,
				      cre2_string_t * match, int nmatch);

typedef int cre2_match_stringz2_fun_t (const char * pattern, cre2_string_t * text,
				       cre2_string_t * match, int nmatch);

typedef int cre2_match_rex_fun_t (cre2_regexp_t * rex, const cre2_string_t * text,
				  cre2_string_t * match, int nmatch);

typedef int cre2_match_rex2_fun_t (cre2_regexp_t * rex, cre2_string_t * text,
				   cre2_string_t * match, int nmatch);

cre2_decl cre2_match_stringz_fun_t	cre2_full_match;
cre2_decl cre2_match_stringz_fun_t	cre2_partial_match;
cre2_decl cre2_match_stringz2_fun_t	cre2_consume;
cre2_decl cre2_match_stringz2_fun_t	cre2_find_and_consume;

cre2_decl cre2_match_rex_fun_t		cre2_full_match_re;
cre2_decl cre2_match_rex_fun_t		cre2_partial_match_re;
cre2_decl cre2_match_rex2_fun_t		cre2_consume_re;
cre2_decl cre2_match_rex2_fun_t		cre2_find_and_consume_re;


/** --------------------------------------------------------------------
 ** Problematic functions.
 ** ----------------------------------------------------------------- */

cre2_decl int cre2_replace	(const char * pattern,
				 cre2_string_t * text_and_target,
				 cre2_string_t * rewrite);
cre2_decl int cre2_replace_re	(cre2_regexp_t * rex,
				 cre2_string_t * text_and_target,
				 cre2_string_t * rewrite);

cre2_decl int cre2_global_replace	(const char * pattern,
					 cre2_string_t * text_and_target,
					 cre2_string_t * rewrite);
cre2_decl int cre2_global_replace_re	(cre2_regexp_t * rex,
					 cre2_string_t * text_and_target,
					 cre2_string_t * rewrite);

cre2_decl int cre2_extract		(const char * pattern,
					 cre2_string_t * text,
					 cre2_string_t * rewrite,
					 cre2_string_t * target);

cre2_decl int cre2_extract_re		(cre2_regexp_t * rex,
					 cre2_string_t * text,
					 cre2_string_t * rewrite,
					 cre2_string_t * target);

cre2_decl int cre2_quote_meta (cre2_string_t * quoted, cre2_string_t * original);

cre2_decl int cre2_possible_match_range (cre2_regexp_t * rex,
					 cre2_string_t * min_, cre2_string_t * max_,
					 int maxlen);

cre2_decl int cre2_check_rewrite_string (cre2_regexp_t * rex,
					 cre2_string_t * rewrite, cre2_string_t * errmsg);


/** --------------------------------------------------------------------
 ** Set match.
 ** ----------------------------------------------------------------- */

struct cre2_set;
typedef struct cre2_set cre2_set;

cre2_decl cre2_set *cre2_set_new(cre2_options_t *opt, cre2_anchor_t anchor);
cre2_decl void      cre2_set_delete(cre2_set *set);
cre2_decl char* cre2_set_add(cre2_set *set, const char *pattern, size_t pattern_len);
cre2_decl int cre2_set_add_simple(cre2_set *set, const char *pattern);
cre2_decl int cre2_set_compile(cre2_set *set);
cre2_decl size_t cre2_set_match(cre2_set *set, const char *text, size_t text_len,
					 int *match, size_t match_len);


/** --------------------------------------------------------------------
 ** Done.
 ** ----------------------------------------------------------------- */

#ifdef __cplusplus
} // extern "C"
#endif

#endif /* CRE2_H */

/* end of file */
