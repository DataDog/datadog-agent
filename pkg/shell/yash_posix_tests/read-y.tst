# read-y.tst: yash-specific test of the read built-in

setup -d

test_oE 'input ending with backslash - not raw mode'
printf '%s' 'A\' | {
read a
echo $?
typeset -p a
}
__IN__
1
typeset a=A
__OUT__

test_oE 'input ending with backslash - raw mode'
printf '%s' 'A\' | {
read --raw-mode a
echo $?
typeset -p a
}
__IN__
1
typeset a='A\'
__OUT__

test_oE 'input containing null byte'
printf 'A\0B\n' | { read a; printf '%s\n' "$a"; }
__IN__
A
__OUT__

# Regardless of the -d option, only backslash-newline is treated as line continuation.
test_oE 'line continuation with non-default delimiter'
read -d : a <<'END'
A\
B:C
END
echoraw $? "[${a-unset}]"
__IN__
0 [AB]
__OUT__

# When the delimiter is backslash, no escape sequence is recognized.
test_oE 'backslash as delimiter'
read -d \\ a <<'END'
A\
B
END
echoraw $? "[${a-unset}]"
__IN__
0 [A]
__OUT__

(
setup 'set --empty-last-field'

test_oE 'exact number of fields with non-whitespace IFS'
IFS=' -' read a b c <<\END
A-B-C - 
END
echoraw $? "[${a-unset}]" "[${b-unset}]" "[${c-unset}]"
__IN__
0 [A] [B] [C -]
__OUT__

test_oE 'too many fields are joined with trailing whitespaces removed'
IFS=' -' read a b c <<\END
A B C-C C\\C\
C   
END
echoraw $? "[${a-unset}]" "[${b-unset}]" "[${c-unset}]"
__IN__
0 [A] [B] [C-C C\CC]
__OUT__

test_oE 'too many fields are joined, ending with non-whitespace delimiter'
IFS=' -' read a b c <<\END
A B C-C C\\C\
C -  
END
echoraw $? "[${a-unset}]" "[${b-unset}]" "[${c-unset}]"
__IN__
0 [A] [B] [C-C C\CC -]
__OUT__

)

# Many other shells behave this way, too.
test_oE 'too many fields are joined with leading whitespaces removed'
IFS=' -' read a b <<\END
 - -
END
echoraw $? "[${a-unset}]" "[${b-unset}]"
IFS=' -' read a b <<\END
 - - -
END
echoraw $? "[${a-unset}]" "[${b-unset}]"
IFS=' -' read a b <<\END
 - -  -   -
END
echoraw $? "[${a-unset}]" "[${b-unset}]"
__IN__
0 [] []
0 [] [- -]
0 [] [-  -   -]
__OUT__

test_oE 'array - single operand - single field'
read -A a <<\END
A
END
echo $?
typeset -p a
__IN__
0
a=(A)
typeset a
__OUT__

test_oE 'array - single operand - no field'
read -A a <<\END

END
echo $?
typeset -p a
__IN__
0
a=()
typeset a
__OUT__

test_oE 'array - many operands'
read -A a b c <<\END
A B C
END
echo $?
typeset -p a b c
__IN__
0
typeset a=A
typeset b=B
c=(C)
typeset c
__OUT__

test_oE 'array - too many fields'
IFS=' -' read -A a b c <<\END
A B C-D E\\E\
E   
END
echo $?
typeset -p a b c
__IN__
0
typeset a=A
typeset b=B
c=(C D 'E\EE')
typeset c
__OUT__

test_oE 'array - too many variables'
read -A a b c d <<\END
A B
END
echo $?
typeset -p a b c d
__IN__
0
typeset a=A
typeset b=B
typeset c=''
d=()
typeset d
__OUT__

test_oE 'array - long option'
read --array a b c <<\END
A B C
END
echo $?
typeset -p a b c
__IN__
0
typeset a=A
typeset b=B
c=(C)
typeset c
__OUT__

test_oE 'array - set -o allexport'
set -a
read -A a b <<\END
A B C D
END
sh -u -c 'echo "[$a]" "[$b]"'
__IN__
[A] [B:C:D]
__OUT__

test_o -d 'assignment to read-only variable'
readonly a
echo A | {
read a
echo $? [$a]
}
__IN__
2 []
__OUT__

test_O -d -e 3 'reading from closed stream'
read foo <&-
__IN__

test_Oe -e 4 'specifying -P and -p both'
read -P -p X foo
__IN__
read: the -P option cannot be used with the -p option
__ERR__

test_Oe -e 4 'multi-character delimiter'
read -d AB foo
__IN__
read: multi-character delimiter is not supported
__ERR__

test_Oe -e 4 'missing operand'
read
__IN__
read: this command requires an operand
__ERR__

test_Oe -e 4 'invalid variable name'
read a=b
__IN__
read: `a=b' is not a valid variable name
__ERR__
#'
#`

# Empty variable name is supported, though it may seem counterintuitive...
test_oE -e 0 'empty variable name'
echo foo | { read ''; readonly ''; readonly; }
__IN__
readonly ''=foo
__OUT__

test_Oe -e 4 'invalid option'
read --no-such-option foo
__IN__
read: `--no-such-option' is not a valid option
__ERR__
#'
#`

# vim: set ft=sh ts=8 sts=4 sw=4 et:
