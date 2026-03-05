# param-y.tst: yash-specific test of parameter expansion

setup -d

test_oE '${#*}'
set 1 22 '3  3'
bracket ${#*}
bracket "${#*}"
__IN__
[1][2][4]
[9]
__OUT__

test_oE '${#@}'
set 1 22 '3  3'
bracket ${#@}
bracket "${#@}"
__IN__
[1][2][4]
[1][2][4]
__OUT__

test_oE '$ followed by non-special character'
bracket $% $+ $, $. $/ $: $= $[ $] $^ $} $~
__IN__
[$%][$+][$,][$.][$/][$:][$=][$[][$]][$^][$}][$~]
__OUT__

test_oE '$ followed by space'
bracket $	$ $
__IN__
[$][$][$]
__OUT__

test_oE '$"'
bracket $"x"
__IN__
[$x]
__OUT__

test_oE '$&'
bracket $&& echo x
__IN__
[$]
x
__OUT__

test_oE '$)'
(bracket $)
__IN__
[$]
__OUT__

test_oE '$;'
bracket $;
__IN__
[$]
__OUT__

test_oE '$<'
bracket $</dev/null
__IN__
[$]
__OUT__

test_oE '$>'
bracket $>&1
__IN__
[$]
__OUT__

test_oE '$\'
bracket $\x
__IN__
[$x]
__OUT__

test_oE '$`'
bracket $`echo x`
__IN__
[$x]
__OUT__

test_oE '$|'
bracket $| cat
__IN__
[$]
__OUT__

test_Oe -e 2 '${}'
bracket ${}
__IN__
syntax error: the parameter name is missing or invalid
__ERR__

test_Oe -e 2 '${}}'
bracket ${}}
__IN__
syntax error: the parameter name is missing or invalid
__ERR__

test_Oe -e 2 '${&}'
bracket ${&}
__IN__
syntax error: the parameter name is missing or invalid
__ERR__

test_Oe -e 2 'missing index'
bracket ${foo[]}
__IN__
syntax error: the index is missing
__ERR__

test_Oe -e 2 'missing start index'
bracket ${foo[,2]}
__IN__
syntax error: the index is missing
__ERR__

test_Oe -e 2 'missing end index'
bracket ${foo[1,]}
__IN__
syntax error: the index is missing
__ERR__

test_Oe -e 2 'missing index and closing bracket'
bracket ${foo[}
__IN__
syntax error: `]' is missing
syntax error: `}' is missing
__ERR__
#'
#`
#'
#`
#]
#}

test_Oe -e 2 'missing end index and closing bracket'
bracket ${foo[1}
__IN__
syntax error: `]' is missing
syntax error: `}' is missing
__ERR__
#'
#`
#'
#`
#]
#}

test_Oe -e 2 'missing closing bracket'
bracket ${foo[1,2}
__IN__
syntax error: `]' is missing
syntax error: `}' is missing
__ERR__
#'
#`
#'
#`
#]
#}

test_Oe -e 2 'missing closing brace'
bracket ${foo
__IN__
syntax error: `}' is missing
__ERR__
#'
#`
#}

test_Oe -e 2 'unexpected colon in parameter expansion'
bracket ${foo:}
__IN__
syntax error: invalid use of `:' in parameter expansion
__ERR__
#'
#`

test_Oe -e 2 'unexpected colon in unclosed parameter expansion'
bracket ${foo:
__IN__
syntax error: invalid use of `:' in parameter expansion
syntax error: `}' is missing
__ERR__
#'
#}
#`
#'
#`

test_Oe -e 2 'colon followed by hash in parameter expansion'
bracket ${foo:#bar}
__IN__
syntax error: invalid use of `:' in parameter expansion
__ERR__
#'
#`

test_Oe -e 2 'colon followed by percent in parameter expansion'
bracket ${foo:%bar}
__IN__
syntax error: invalid use of `:' in parameter expansion
__ERR__
#'
#`

test_Oe -e 2 'unexpected exclamation in unclosed parameter expansion'
bracket ${foo!
__IN__
syntax error: invalid character `!' in parameter expansion
__ERR__
#'
#`
#}

test_Oe -e 2 'missing closing brace after hash'
bracket ${foo#bar
__IN__
syntax error: `}' is missing
__ERR__
#'
#`
#}

test_Oe -e 2 'missing closing brace after percent'
bracket ${foo%bar
__IN__
syntax error: `}' is missing
__ERR__
#'
#`
#}

test_Oe -e 2 'missing closing brace after one slash'
bracket ${foo/bar
__IN__
syntax error: `}' is missing
__ERR__
#'
#`
#}

test_Oe -e 2 'missing closing brace after two slashes'
bracket ${foo/bar/baz
__IN__
syntax error: `}' is missing
__ERR__
#'
#`
#}

test_Oe -e 2 '${#foo-bar}'
bracket ${#foo-bar}
__IN__
syntax error: invalid use of `#' in parameter expansion
__ERR__
#'
#`

test_Oe -e 2 '${#foo+bar}'
bracket ${#foo+bar}
__IN__
syntax error: invalid use of `#' in parameter expansion
__ERR__
#'
#`

test_Oe -e 2 '${#foo?bar}'
bracket ${#foo?bar}
__IN__
syntax error: invalid use of `#' in parameter expansion
__ERR__
#'
#`

test_Oe -e 2 '${#foo=bar}'
bracket ${#foo=bar}
__IN__
syntax error: invalid use of `#' in parameter expansion
__ERR__
#'
#`

test_Oe -e 2 '${#foo#bar}'
bracket ${#foo#bar}
__IN__
syntax error: invalid use of `#' in parameter expansion
__ERR__
#'
#`

test_Oe -e 2 '${#foo%bar}'
bracket ${#foo%bar}
__IN__
syntax error: invalid use of `#' in parameter expansion
__ERR__
#'
#`

test_Oe -e 2 '${#foo/bar}'
bracket ${#foo/bar}
__IN__
syntax error: invalid use of `#' in parameter expansion
__ERR__
#'
#`

test_Oe -e 2 '${#foo/bar/baz}'
bracket ${#foo/bar/baz}
__IN__
syntax error: invalid use of `#' in parameter expansion
__ERR__
#'
#`

test_oE '${#+}'
bracket "${#+}" # substitute empty string if # is set
__IN__
[]
__OUT__

test_oE '${#=}'
bracket ${#=} # assign and substitute empty string if # is unset
__IN__
[0]
__OUT__

test_oE '${##}'
bracket ${##} # length of $#
__IN__
[1]
__OUT__

test_oE '${##x}'
bracket "${##0}" # actually unambiguous, but POSIXly unspecified behavior
__IN__
[]
__OUT__

test_oE '${#%x}'
bracket "${#%0}" # actually unambiguous, but POSIXly unspecified behavior
__IN__
[]
__OUT__

test_oE '${#/...}'
bracket "${#/x/y}" "${#:/x/y}" "${#//x/y}"
__IN__
[0][0][0]
__OUT__

test_oE '${*#x}'
# The matching prefix is removed from each positional parameter and then all
# the parameters are concatenated.
set '1-1-1' 2 '3  -  3'
bracket "${*#*-}"
__IN__
[1-1 2   3]
__OUT__

test_oE '${*%x}'
# The matching suffix is removed from each positional parameter and then all
# the parameters are concatenated.
set '1-1-1' 2 '3  -  3'
bracket "${*%-*}"
__IN__
[1-1 2 3  ]
__OUT__

test_oE '${@#x}'
# The matching suffix is removed from each positional parameter.
set '1-1-1' 2 '3  -  3'
bracket "${@#*-}"
__IN__
[1-1][2][  3]
__OUT__

test_oE '${@%x}'
# The matching suffix is removed from each positional parameter.
set '1-1-1' 2 '3  -  3'
bracket "${@%-*}"
__IN__
[1-1][2][3  ]
__OUT__

test_oE 'yash-specific behaviors about $@'
e=
bracket "$@$@" - "$e$@" - "$@$e" - "$e$@$e"
bracket "${e:-$@}"
__IN__
[-][-][-]

__OUT__

test_oE 'nested expansion'
a='..x..'
bracket ${{a#..}%..} ${${a#..}%..}
bracket ${$(echo 123)%3} ${`echo 123`%3} ${$((3*4))%2}
__IN__
[x][x]
[12][12][1]
__OUT__

test_oE 'quoting in nested expansion'
a='*' bs='\' bs2='\\'
> nested_expansion
bracket nested${{a}}expansion nested${{bs}}*expansion ${{bs2}} ${{u-\\'*'"?"}}
__IN__
[nested_expansion][nested\*expansion][\\][\*?]
__OUT__

test_oE '$@ in nested expansion'
bracket ${@} - ${{@}} - "${@}" - "${{@}}"
set ''
bracket ${@} - ${{@}} - "${@}" - "${{@}}"
set ' '
bracket ${@} - ${{@}} - "${@}" - "${{@}}"
set '' ''
bracket ${@} - ${{@}} - "${@}" - "${{@}}"
__IN__
[-][-][-]
[-][-][][-][]
[-][-][ ][-][ ]
[-][-][][][-][][]
__OUT__

test_O -d -e 2 'error in nested expansion'
: ${${a-${b?}}}
__IN__

test_oE 'disambiguation of ${$...' # None of below are nested expansions
bracket ${$++} ${$:++}
[ "${$--}" = "$$" ] || echoraw - [ "${$--}" = "$$" ]
[ "${$==}" = "$$" ] || echoraw = [ "${$==}" = "$$" ]
[ "${$??}" = "$$" ] || echoraw ? [ "${$??}" = "$$" ]
[ "${$:--}" = "$$" ] || echoraw :- [ "${$:--}" = "$$" ]
[ "${$:==}" = "$$" ] || echoraw := [ "${$:==}" = "$$" ]
[ "${$:??}" = "$$" ] || echoraw :? [ "${$:??}" = "$$" ]
[ "${$#\#}" = "$$" ] || echoraw \# [ "${$#\#}" = "$$" ]
[ "${$%\%}" = "$$" ] || echoraw \% [ "${$%\%}" = "$$" ]
[ "${$###}" = "$$" ] || echoraw \## [ "${$###}" = "$$" ]
[ "${$%%%}" = "$$" ] || echoraw \%% [ "${$%%%}" = "$$" ]
[ "${$/x/y}" = "$$" ] || echoraw / [ "${$/x/y}" = "$$" ]
[ "${$:/x/y}" = "$$" ] || echoraw :/ [ "${$:/x/y}" = "$$" ]
[ "${$//x/y}" = "$$" ] || echoraw // [ "${$//x/y}" = "$$" ]
__IN__
[+][+]
__OUT__

test_oE '${a/b/c}'
a='123/456/789' b='1*2?3' HOME=/
bracket ${a/4*6/x} ${a/\//y} ${a/\//} ${a/\/}
bracket ${a/#*3/x} ${a/#456/y}
bracket ${a/%7*/x} ${a/%456/y}
bracket ${a//4*6/x} ${a//\//y} ${a//\//} ${a//\/}
bracket ${a:/1*9/x} ${a:/2*9/x} ${a:/1*8/x}
bracket ${b/\**\?/x} ${b/"*"?'?'/x}
bracket ${a/5/~/\*'*'"*"} ${a//~}
> 1_2_3
bracket ${a/*/$b} ${a/*/"$b"}
__IN__
[123/x/789][123y456/789][123456/789][123456/789]
[x/456/789][123/456/789]
[123/456/x][123/456/789]
[123/x/789][123y456y789][123456789][123456789]
[x][123/456/789][123/456/789]
[1x3][1x3]
[123/4/***6/789][123456789]
[1_2_3][1_2_3]
__OUT__
# XXX: Should the last one (${a/*/"$b"}) expand to 1*2?3 rather than 1_2_3?

test_oE 'scalar parameter index'
a='1-2-3'
bracket @ "${a[@]}"
bracket \* "${a[*]}"
bracket \# "${a[#]}"
for i in -6 -5 -2 -1 0 1 2 5 6; do
    bracket $i "${a[i]}"
done
for i in -6 -5 -1 0 1 5 6; do
    for j in -6 -5 -1 0 1 5 6; do
        bracket $i,$j "${a[i,j]}"
    done
done
__IN__
[@][1-2-3]
[*][1-2-3]
[#][5]
[-6][]
[-5][1]
[-2][-]
[-1][3]
[0][]
[1][1]
[2][-]
[5][3]
[6][]
[-6,-6][]
[-6,-5][1]
[-6,-1][1-2-3]
[-6,0][]
[-6,1][1]
[-6,5][1-2-3]
[-6,6][1-2-3]
[-5,-6][]
[-5,-5][1]
[-5,-1][1-2-3]
[-5,0][]
[-5,1][1]
[-5,5][1-2-3]
[-5,6][1-2-3]
[-1,-6][]
[-1,-5][]
[-1,-1][3]
[-1,0][]
[-1,1][]
[-1,5][3]
[-1,6][3]
[0,-6][]
[0,-5][]
[0,-1][]
[0,0][]
[0,1][]
[0,5][]
[0,6][]
[1,-6][]
[1,-5][1]
[1,-1][1-2-3]
[1,0][]
[1,1][1]
[1,5][1-2-3]
[1,6][1-2-3]
[5,-6][]
[5,-5][]
[5,-1][3]
[5,0][]
[5,1][]
[5,5][3]
[5,6][3]
[6,-6][]
[6,-5][]
[6,-1][]
[6,0][]
[6,1][]
[6,5][]
[6,6][]
__OUT__

test_oE 'array variable index'
a=(1 22 '3  3' 4"   "4 '')
bracket @ "${a[@]}"
bracket \* "${a[*]}"
bracket \# "${a[#]}"
for i in -6 -5 -2 -1 0 1 2 5 6; do
    bracket $i "${a[i]}"
done
for i in -6 -5 -1 0 1 5 6; do
    for j in -6 -5 -1 0 1 5 6; do
        bracket $i,$j "${a[i,j]}"
    done
done
__IN__
[@][1][22][3  3][4   4][]
[*][1 22 3  3 4   4 ]
[#][5]
[-6]
[-5][1]
[-2][4   4]
[-1][]
[0]
[1][1]
[2][22]
[5][]
[6]
[-6,-6]
[-6,-5][1]
[-6,-1][1][22][3  3][4   4][]
[-6,0]
[-6,1][1]
[-6,5][1][22][3  3][4   4][]
[-6,6][1][22][3  3][4   4][]
[-5,-6]
[-5,-5][1]
[-5,-1][1][22][3  3][4   4][]
[-5,0]
[-5,1][1]
[-5,5][1][22][3  3][4   4][]
[-5,6][1][22][3  3][4   4][]
[-1,-6]
[-1,-5]
[-1,-1][]
[-1,0]
[-1,1]
[-1,5][]
[-1,6][]
[0,-6]
[0,-5]
[0,-1]
[0,0]
[0,1]
[0,5]
[0,6]
[1,-6]
[1,-5][1]
[1,-1][1][22][3  3][4   4][]
[1,0]
[1,1][1]
[1,5][1][22][3  3][4   4][]
[1,6][1][22][3  3][4   4][]
[5,-6]
[5,-5]
[5,-1][]
[5,0]
[5,1]
[5,5][]
[5,6][]
[6,-6]
[6,-5]
[6,-1]
[6,0]
[6,1]
[6,5]
[6,6]
__OUT__

test_oE 'positional parameter index (*)'
set 1 22 '3  3' 4"   "4 ''
bracket @ "${*[@]}"
bracket \* "${*[*]}"
bracket \# "${*[#]}"
for i in -6 -5 -2 -1 0 1 2 5 6; do
    bracket $i "${*[i]}"
done
for i in -6 -5 -1 0 1 5 6; do
    for j in -6 -5 -1 0 1 5 6; do
        bracket $i,$j "${*[i,j]}"
    done
done
__IN__
[@][1 22 3  3 4   4 ]
[*][1 22 3  3 4   4 ]
[#][5]
[-6][]
[-5][1]
[-2][4   4]
[-1][]
[0][]
[1][1]
[2][22]
[5][]
[6][]
[-6,-6][]
[-6,-5][1]
[-6,-1][1 22 3  3 4   4 ]
[-6,0][]
[-6,1][1]
[-6,5][1 22 3  3 4   4 ]
[-6,6][1 22 3  3 4   4 ]
[-5,-6][]
[-5,-5][1]
[-5,-1][1 22 3  3 4   4 ]
[-5,0][]
[-5,1][1]
[-5,5][1 22 3  3 4   4 ]
[-5,6][1 22 3  3 4   4 ]
[-1,-6][]
[-1,-5][]
[-1,-1][]
[-1,0][]
[-1,1][]
[-1,5][]
[-1,6][]
[0,-6][]
[0,-5][]
[0,-1][]
[0,0][]
[0,1][]
[0,5][]
[0,6][]
[1,-6][]
[1,-5][1]
[1,-1][1 22 3  3 4   4 ]
[1,0][]
[1,1][1]
[1,5][1 22 3  3 4   4 ]
[1,6][1 22 3  3 4   4 ]
[5,-6][]
[5,-5][]
[5,-1][]
[5,0][]
[5,1][]
[5,5][]
[5,6][]
[6,-6][]
[6,-5][]
[6,-1][]
[6,0][]
[6,1][]
[6,5][]
[6,6][]
__OUT__

test_oE 'positional parameter index (@)'
set 1 22 '3  3' 4"   "4 ''
bracket @ "${@[@]}"
bracket \* "${@[*]}"
bracket \# "${@[#]}"
for i in -6 -5 -2 -1 0 1 2 5 6; do
    bracket $i "${@[i]}"
done
for i in -6 -5 -1 0 1 5 6; do
    for j in -6 -5 -1 0 1 5 6; do
        bracket $i,$j "${@[i,j]}"
    done
done
__IN__
[@][1][22][3  3][4   4][]
[*][1 22 3  3 4   4 ]
[#][5]
[-6]
[-5][1]
[-2][4   4]
[-1][]
[0]
[1][1]
[2][22]
[5][]
[6]
[-6,-6]
[-6,-5][1]
[-6,-1][1][22][3  3][4   4][]
[-6,0]
[-6,1][1]
[-6,5][1][22][3  3][4   4][]
[-6,6][1][22][3  3][4   4][]
[-5,-6]
[-5,-5][1]
[-5,-1][1][22][3  3][4   4][]
[-5,0]
[-5,1][1]
[-5,5][1][22][3  3][4   4][]
[-5,6][1][22][3  3][4   4][]
[-1,-6]
[-1,-5]
[-1,-1][]
[-1,0]
[-1,1]
[-1,5][]
[-1,6][]
[0,-6]
[0,-5]
[0,-1]
[0,0]
[0,1]
[0,5]
[0,6]
[1,-6]
[1,-5][1]
[1,-1][1][22][3  3][4   4][]
[1,0]
[1,1][1]
[1,5][1][22][3  3][4   4][]
[1,6][1][22][3  3][4   4][]
[5,-6]
[5,-5]
[5,-1][]
[5,0]
[5,1]
[5,5][]
[5,6][]
[6,-6]
[6,-5]
[6,-1]
[6,0]
[6,1]
[6,5]
[6,6]
__OUT__

test_oE 'arithmetics in parameter index'
a=12345 b='* 2'
echo ${a[1 + 1, (1 + 1) $b]}
__IN__
234
__OUT__

test_oE 'backslash in parameter index'
a=abcde
echo ${a[1\+1,2\*2]}
echo ${a[\@]}
__IN__
bcd
abcde
__OUT__

test_oE 'valid assignment to array element in expansion'
a=('' '' '')
bracket "${a[1]=x}" "${a[2]=y}" "${a[3]=z}"
bracket ${a[1]:=1}
bracket "${a}"
__IN__
[][][]
[1]
[1][][]
__OUT__

test_oE 'ignored assignment to array element in expansion'
a=(1)
bracket ${a[1]:=x}
bracket "${a}"
__IN__
[1]
[1]
__OUT__

test_Oe -e 2 'out-of-range assignment to array element in expansion'
a=(1)
eval 'bracket ${a[2]:=x}'
__IN__
eval: index 2 is out of range (the actual size of array $a is 1)
__ERR__

test_Oe 'assignment to read-only array element in expansion'
a=('')
readonly a
eval 'bracket "${a[1]:=X}"'
__IN__
eval: $a is read-only
__ERR__

test_oE 'exportation of array'
a=() b=(1) c=(1 2) d=(1 ':  :' 3)
export a b c d
sh -c 'printf "[%s]\n" "$a" "$b" "$c" "$d"'
__IN__
[]
[1]
[1:2]
[1::  ::3]
__OUT__

(
posix="true"

test_Oe -e 2 'nested expansion (with $) unavailable in POSIX mode'
echo ${${a}}
__IN__
syntax error: invalid character `{' in parameter expansion
__ERR__
#'
#`

test_Oe -e 2 'nested expansion (w/o $) unavailable in POSIX mode'
echo ${{a}}
__IN__
syntax error: the parameter name is missing or invalid
__ERR__

test_Oe -e 2 'index unavailable in POSIX mode'
a=123
echo ${a[1]}
__IN__
syntax error: invalid character `[' in parameter expansion
__ERR__
#'
#`

test_Oe -e 2 '${foo/bar} unavailable in POSIX mode'
a=123
echo ${a/2}
__IN__
syntax error: invalid character `/' in parameter expansion
__ERR__
#'
#`

)

# vim: set ft=sh ts=8 sts=4 sw=4 et:
