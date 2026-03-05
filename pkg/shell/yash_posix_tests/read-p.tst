# read-p.tst: test of the read built-in for any POSIX-compliant shell

posix="true"
setup -d

test_oE 'single operand - without IFS'
read a <<\END
A
END
echoraw $? "[${a-unset}]"
__IN__
0 [A]
__OUT__

test_oE 'single operand - with IFS whitespace'
read a <<\END
  A  
END
echoraw $? "[${a-unset}]"
__IN__
0 [A]
__OUT__

test_oE 'single operand - with IFS non-whitespace'
read a <<\END
 - A - 
END
echoraw $? "[${a-unset}]"
__IN__
0 [- A -]
__OUT__

test_oE 'EOF fails read'
! read a </dev/null
echoraw $? "[${a-unset}]"
__IN__
0 []
__OUT__

test_oE 'read does not read more than needed'
{
    read a
    echo B
    cat
} <<\END
\
A
C
END
__IN__
B
C
__OUT__

test_oE 'variables are assigned even if EOF is reached without newline'
printf 'foo bar baz' | {
read a b
echo $? [$a] [$b]
}
__IN__
1 [foo] [bar baz]
__OUT__

test_oE 'orphan backslash is ignored'
printf 'foo\' | {
read a
printf '[%s]\n' "$a"
}
__IN__
[foo]
__OUT__

test_oE 'set -o allexport'
(
set -a
read a b <<\END
A B
END
sh -u -c 'echo "[$a]" "[$b]"'
)
__IN__
[A] [B]
__OUT__

test_oE 'line continuation - followed by normal line'
read a b <<\END
A\
A B\
B
END
echoraw $? "[${a-unset}]" "[${b-unset}]"
__IN__
0 [AA] [BB]
__OUT__

test_oE 'line continuation - followed by EOF'
! read a b <<\END
A\
END
echoraw $? "[${a-unset}]" "[${b-unset}]"
__IN__
0 [A] []
__OUT__

test_oE 'field splitting - 1'
IFS=' -' read a b c <<\END
 AA B CC 
END
echoraw $? "[${a-unset}]" "[${b-unset}]" "[${c-unset}]"
__IN__
0 [AA] [B] [CC]
__OUT__

test_oE 'field splitting - 2-1'
IFS=' -' read a b c d e <<\END
-BB-C-DD-
END
echoraw $? "[${a-unset}]" "[${b-unset}]" "[${c-unset}]" \
    "[${d-unset}]" "[${e-unset}]"
__IN__
0 [] [BB] [C] [DD] []
__OUT__

test_oE 'field splitting - 2-2'
IFS=' -' read a b c d e <<\END
- BB- C- DD- 
END
echoraw $? "[${a-unset}]" "[${b-unset}]" "[${c-unset}]" \
    "[${d-unset}]" "[${e-unset}]"
__IN__
0 [] [BB] [C] [DD] []
__OUT__

test_oE 'field splitting - 2-3'
IFS=' -' read a b c d e <<\END
 -BB -C -DD -
END
echoraw $? "[${a-unset}]" "[${b-unset}]" "[${c-unset}]" \
    "[${d-unset}]" "[${e-unset}]"
__IN__
0 [] [BB] [C] [DD] []
__OUT__

test_oE 'field splitting - 2-4'
IFS=' -' read a b c d e <<\END
 - BB - C - DD - 
END
echoraw $? "[${a-unset}]" "[${b-unset}]" "[${c-unset}]" \
    "[${d-unset}]" "[${e-unset}]"
__IN__
0 [] [BB] [C] [DD] []
__OUT__

test_oE 'field splitting - 3-1'
IFS=' -' read a b c d e <<\END
--CC--
END
echoraw $? "[${a-unset}]" "[${b-unset}]" "[${c-unset}]" \
    "[${d-unset}]" "[${e-unset}]"
__IN__
0 [] [] [CC] [] []
__OUT__

test_oE 'field splitting - 3-2'
IFS=' -' read a b c d e <<\END
  --CC  --
END
echoraw $? "[${a-unset}]" "[${b-unset}]" "[${c-unset}]" \
    "[${d-unset}]" "[${e-unset}]"
__IN__
0 [] [] [CC] [] []
__OUT__

test_oE 'field splitting - 3-3'
IFS=' -' read a b c d e <<\END
-  -CC-  -
END
echoraw $? "[${a-unset}]" "[${b-unset}]" "[${c-unset}]" \
    "[${d-unset}]" "[${e-unset}]"
__IN__
0 [] [] [CC] [] []
__OUT__

test_oE 'field splitting - 3-4'
IFS=' -' read a b c d e <<\END
--  CC--  
END
echoraw $? "[${a-unset}]" "[${b-unset}]" "[${c-unset}]" \
    "[${d-unset}]" "[${e-unset}]"
__IN__
0 [] [] [CC] [] []
__OUT__

test_oE 'backslash prevents field splitting - backslash not in IFS'
IFS=' -' read a b c d <<\END
A\ A \ \B\  C\\C\-C\\-D
END
echoraw $? "[${a-unset}]" "[${b-unset}]" "[${c-unset}]" "[${d-unset}]"
__IN__
0 [A A] [ B ] [C\C-C\] [D]
__OUT__

test_oE 'backslash prevents field splitting - backslash in IFS'
IFS=' -\' read a b c d <<\END
A\ A \ \B\  C\\C\-C\\-D
END
echoraw $? "[${a-unset}]" "[${b-unset}]" "[${c-unset}]" "[${d-unset}]"
__IN__
0 [A A] [ B ] [C\C-C\] [D]
__OUT__

test_oE 'line continuation and newline as IFS'
IFS='
' read a b <<\END
A\
B
C
END
echoraw $? "[${a-unset}]" "[${b-unset}]"
__IN__
0 [AB] []
__OUT__

test_oE 'variables are assigned empty string for missing fields'
read a b c d <<\END
A B
END
echoraw $? "[${a-unset}]" "[${b-unset}]" "[${c-unset}]" "[${d-unset}]"
__IN__
0 [A] [B] [] []
__OUT__

test_oE 'exact number of fields with non-whitespace IFS'
IFS=' -' read a b c <<\END
A-B-C - 
END
echoraw $? "[${a-unset}]" "[${b-unset}]" "[${c-unset}]"
__IN__
0 [A] [B] [C]
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

test_oE 'no field splitting with empty IFS'
IFS= read a b c d <<\END
 A\ B \ \C\  D\\E\-F\\-G 
END
echoraw $? "[${a-unset}]" "[${b-unset}]" "[${c-unset}]" "[${d-unset}]"
__IN__
0 [ A B  C  D\E-F\-G ] [] [] []
__OUT__

test_oE 'non-default delimiters'
{
read -d : a b
read -d x c d
} <<\END
A B:C D ExF
END
echoraw $? "[${a-unset}]" "[${b-unset}]" "[${c-unset}]" "[${d-unset}]"
__IN__
0 [A] [B] [C] [D E]
__OUT__

test_oE 'raw mode - backslash not in IFS'
IFS=' -' read -r a b c d <<\END
A\A\\ B-C\- D\
X
END
echoraw $? "[${a-unset}]" "[${b-unset}]" "[${c-unset}]" "[${d-unset}]"
__IN__
0 [A\A\\] [B] [C\] [D\]
__OUT__

test_oE 'raw mode - backslash in IFS'
IFS=' -\' read -r a b c d e f <<\END
A\B\\ D-E\- F\
X
END
echoraw $? "[${a-unset}]" "[${b-unset}]" "[${c-unset}]" "[${d-unset}]" \
    "[${e-unset}]" "[${f-unset}]"
__IN__
0 [A] [B] [] [D] [E] [- F\]
__OUT__

test_oE 'in subshell'
(echo A | read a)
echoraw $? "[${a-unset}]"
__IN__
0 [unset]
__OUT__

test_O -e n 'failure by readonly variable'
echo B | (readonly a=A; read a)
__IN__

# vim: set ft=sh ts=8 sts=4 sw=4 et:
