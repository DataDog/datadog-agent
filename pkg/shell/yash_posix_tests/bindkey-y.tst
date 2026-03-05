# bindkey-y.tst: yash-specific test of the bindkey built-in

if ! testee -c 'command -bv bindkey' >/dev/null; then
    skip="true"
fi

test_oE -e 0 'bindkey is an elective built-in'
command -V bindkey
__IN__
bindkey: an elective built-in
__OUT__

sort -k 1 >commands <<\__END__
noop
alert
self-insert
insert-tab
expect-verbatim
digit-argument
bol-or-digit
accept-line
abort-line
eof
eof-if-empty
eof-or-delete
accept-with-hash
accept-prediction
setmode-viinsert
setmode-vicommand
setmode-emacs
expect-char
abort-expect-char
redraw-all
clear-and-redraw-all
forward-char
backward-char
forward-bigword
end-of-bigword
backward-bigword
forward-semiword
end-of-semiword
backward-semiword
forward-viword
end-of-viword
backward-viword
forward-emacsword
backward-emacsword
beginning-of-line
end-of-line
go-to-column
first-nonblank
find-char
find-char-rev
till-char
till-char-rev
refind-char
refind-char-rev
delete-char
delete-bigword
delete-semiword
delete-viword
delete-emacsword
backward-delete-char
backward-delete-bigword
backward-delete-semiword
backward-delete-viword
backward-delete-emacsword
delete-line
forward-delete-line
backward-delete-line
kill-char
kill-bigword
kill-semiword
kill-viword
kill-emacsword
backward-kill-char
backward-kill-bigword
backward-kill-semiword
backward-kill-viword
backward-kill-emacsword
kill-line
forward-kill-line
backward-kill-line
put-before
put
put-left
put-pop
undo
undo-all
cancel-undo
cancel-undo-all
redo
complete
complete-next-candidate
complete-prev-candidate
complete-next-column
complete-prev-column
complete-next-page
complete-prev-page
complete-list
complete-all
complete-max
complete-max-then-list
complete-max-then-next-candidate
complete-max-then-prev-candidate
clear-candidates
vi-replace-char
vi-insert-beginning
vi-append
vi-append-to-eol
vi-replace
vi-switch-case
vi-switch-case-char
vi-yank
vi-yank-to-eol
vi-delete
vi-delete-to-eol
vi-change
vi-change-to-eol
vi-change-line
vi-yank-and-change
vi-yank-and-change-to-eol
vi-yank-and-change-line
vi-substitute
vi-append-last-bigword
vi-exec-alias
vi-edit-and-accept
vi-complete-list
vi-complete-all
vi-complete-max
vi-search-forward
vi-search-backward
emacs-transpose-chars
emacs-transpose-words
emacs-upcase-word
emacs-downcase-word
emacs-capitalize-word
emacs-delete-horizontal-space
emacs-just-one-space
emacs-search-forward
emacs-search-backward
emacs-search-forward-current
emacs-search-backward-current
oldest-history
newest-history
return-history
oldest-history-bol
newest-history-bol
return-history-bol
oldest-history-eol
newest-history-eol
return-history-eol
next-history
prev-history
next-history-bol
prev-history-bol
next-history-eol
prev-history-eol
srch-self-insert
srch-backward-delete-char
srch-backward-delete-line
srch-continue-forward
srch-continue-backward
srch-accept-search
srch-abort-search
search-again
search-again-rev
search-again-forward
search-again-backward
beginning-search-forward
beginning-search-backward
__END__

testcase "$LINENO" -e 0 'printing commands' \
    4<commands 5</dev/null 3<<\__IN__
bindkey -l
__IN__

sort >vi_insert <<\__END__
bindkey -v '\\' self-insert
bindkey -v '\^V' expect-verbatim
bindkey -v '\et' accept-line
bindkey -v '\^J' accept-line
bindkey -v '\^M' accept-line
bindkey -v '\!' abort-line
bindkey -v '\^C' abort-line
bindkey -v '\#' eof-if-empty
bindkey -v '\^D' eof-if-empty
bindkey -v '\^[' setmode-vicommand
bindkey -v '\^L' redraw-all
bindkey -v '\R' forward-char
bindkey -v '\L' backward-char
bindkey -v '\H' beginning-of-line
bindkey -v '\E' end-of-line
bindkey -v '\X' delete-char
bindkey -v '\B' backward-delete-char
bindkey -v '\?' backward-delete-char
bindkey -v '\^H' backward-delete-char
bindkey -v '\^W' backward-delete-semiword
bindkey -v '\$' backward-delete-line
bindkey -v '\^U' backward-delete-line
bindkey -v '\D' next-history-eol
bindkey -v '\^N' next-history-eol
bindkey -v '\U' prev-history-eol
bindkey -v '\^P' prev-history-eol
bindkey -v '\^I' complete-next-candidate
bindkey -v '\bt' complete-prev-candidate
__END__

testcase "$LINENO" 'printing default vi-insert bindings: output' \
    4<vi_insert 5</dev/null 3<<\__IN__
bindkey -v | sort
__IN__

test_x -e 0 'printing default vi-insert bindings: exit status'
bindkey -v
__IN__

test_OE -e 0 'binding key (vi-insert)'
bindkey -v 'a' self-insert
__IN__

test_oE -e 0 'printing bound key (vi-insert)'
bindkey -v a self-insert
bindkey -v a
__IN__
bindkey -v a self-insert
__OUT__

test_OE -e 0 'unbinding key (vi-insert)'
bindkey -v '\!' -
__IN__

test_Oe -e 1 'printing unbound key (vi-insert)'
bindkey -v '\!' -
bindkey -v '\!'
__IN__
bindkey: key sequence `\!' is not bound
__ERR__
#`

sort >vi_command <<\__END__
bindkey -a '\^[' noop
bindkey -a 1 digit-argument
bindkey -a 2 digit-argument
bindkey -a 3 digit-argument
bindkey -a 4 digit-argument
bindkey -a 5 digit-argument
bindkey -a 6 digit-argument
bindkey -a 7 digit-argument
bindkey -a 8 digit-argument
bindkey -a 9 digit-argument
bindkey -a 0 bol-or-digit
bindkey -a '\et' accept-line
bindkey -a '\^J' accept-line
bindkey -a '\^M' accept-line
bindkey -a '\!' abort-line
bindkey -a '\^C' abort-line
bindkey -a '\#' eof-if-empty
bindkey -a '\^D' eof-if-empty
bindkey -a '#' accept-with-hash
bindkey -a i setmode-viinsert
bindkey -a '\I' setmode-viinsert
bindkey -a '\^L' redraw-all
bindkey -a l forward-char
bindkey -a ' ' forward-char
bindkey -a '\R' forward-char
bindkey -a h backward-char
bindkey -a '\L' backward-char
bindkey -a '\B' backward-char
bindkey -a '\?' backward-char
bindkey -a '\^H' backward-char
bindkey -a W forward-bigword
bindkey -a E end-of-bigword
bindkey -a B backward-bigword
bindkey -a w forward-viword
bindkey -a e end-of-viword
bindkey -a b backward-viword
bindkey -a '\H' beginning-of-line
bindkey -a '$' end-of-line
bindkey -a '\E' end-of-line
bindkey -a '|' go-to-column
bindkey -a '^' first-nonblank
bindkey -a f find-char
bindkey -a F find-char-rev
bindkey -a t till-char
bindkey -a T till-char-rev
bindkey -a ';' refind-char
bindkey -a ',' refind-char-rev
bindkey -a x kill-char
bindkey -a '\X' kill-char
bindkey -a X backward-kill-char
bindkey -a P put-before
bindkey -a p put
bindkey -a u undo
bindkey -a U undo-all
bindkey -a '\^R' cancel-undo
bindkey -a . redo
bindkey -a r vi-replace-char
bindkey -a I vi-insert-beginning
bindkey -a a vi-append
bindkey -a A vi-append-to-eol
bindkey -a R vi-replace
bindkey -a '~' vi-switch-case-char
bindkey -a y vi-yank
bindkey -a Y vi-yank-to-eol
bindkey -a d vi-delete
bindkey -a D forward-kill-line
bindkey -a c vi-change
bindkey -a C vi-change-to-eol
bindkey -a S vi-change-line
bindkey -a s vi-substitute
bindkey -a _ vi-append-last-bigword
bindkey -a '@' vi-exec-alias
bindkey -a v vi-edit-and-accept
bindkey -a '=' vi-complete-list
bindkey -a '*' vi-complete-all
bindkey -a '\\' vi-complete-max
bindkey -a '?' vi-search-forward
bindkey -a / vi-search-backward
bindkey -a G oldest-history-bol
bindkey -a g return-history-bol
bindkey -a j next-history-bol
bindkey -a '+' next-history-bol
bindkey -a '\D' next-history-bol
bindkey -a '\^N' next-history-bol
bindkey -a k prev-history-bol
bindkey -a -- - prev-history-bol
bindkey -a '\U' prev-history-bol
bindkey -a '\^P' prev-history-bol
bindkey -a n search-again
bindkey -a N search-again-rev
__END__

testcase "$LINENO" 'printing default vi-command bindings: output' \
    4<vi_command 5</dev/null 3<<\__IN__
bindkey -a | sort
__IN__

test_x -e 0 'printing default vi-command bindings: exit status'
bindkey -a
__IN__

test_OE -e 0 'binding key (vi-command)'
bindkey -a a self-insert
__IN__

test_oE -e 0 'printing bound key (vi-command)'
bindkey -a a self-insert
bindkey -a a
__IN__
bindkey -a a self-insert
__OUT__

test_OE -e 0 'unbinding key (vi-command)'
bindkey -a f -
__IN__

test_Oe -e 1 'printing unbound key (vi-command)'
bindkey -a f -
bindkey -a f
__IN__
bindkey: key sequence `f' is not bound
__ERR__
#`

sort >emacs <<\__END__
bindkey -e '\\' self-insert
bindkey -e '\^[\^I' insert-tab
bindkey -e '\^Q' expect-verbatim
bindkey -e '\^V' expect-verbatim
bindkey -e '\^[0' digit-argument
bindkey -e '\^[1' digit-argument
bindkey -e '\^[2' digit-argument
bindkey -e '\^[3' digit-argument
bindkey -e '\^[4' digit-argument
bindkey -e '\^[5' digit-argument
bindkey -e '\^[6' digit-argument
bindkey -e '\^[7' digit-argument
bindkey -e '\^[8' digit-argument
bindkey -e '\^[9' digit-argument
bindkey -e '\^[-' digit-argument
bindkey -e '\et' accept-line
bindkey -e '\^J' accept-line
bindkey -e '\^M' accept-line
bindkey -e '\!' abort-line
bindkey -e '\^C' abort-line
bindkey -e '\#' eof-or-delete
bindkey -e '\^D' eof-or-delete
bindkey -e '\^[#' accept-with-hash
bindkey -e '\^L' redraw-all
bindkey -e '\R' forward-char
bindkey -e '\^F' forward-char
bindkey -e '\L' backward-char
bindkey -e '\^B' backward-char
bindkey -e '\^[f' forward-emacsword
bindkey -e '\^[F' forward-emacsword
bindkey -e '\^[b' backward-emacsword
bindkey -e '\^[B' backward-emacsword
bindkey -e '\H' beginning-of-line
bindkey -e '\^A' beginning-of-line
bindkey -e '\E' end-of-line
bindkey -e '\^E' end-of-line
bindkey -e '\^]' find-char
bindkey -e '\^[\^]' find-char-rev
bindkey -e '\X' delete-char
bindkey -e '\B' backward-delete-char
bindkey -e '\?' backward-delete-char
bindkey -e '\^H' backward-delete-char
bindkey -e '\^[d' kill-emacsword
bindkey -e '\^[D' kill-emacsword
bindkey -e '\^W' backward-kill-bigword
bindkey -e '\^[\B' backward-kill-emacsword
bindkey -e '\^[\?' backward-kill-emacsword
bindkey -e '\^[\^H' backward-kill-emacsword
bindkey -e '\^K' forward-kill-line
bindkey -e '\$' backward-kill-line
bindkey -e '\^U' backward-kill-line
bindkey -e '\^X\B' backward-kill-line
bindkey -e '\^X\?' backward-kill-line
bindkey -e '\^Y' put-left
bindkey -e '\^[y' put-pop
bindkey -e '\^[Y' put-pop
bindkey -e '\^_' undo
bindkey -e '\^X\$' undo
bindkey -e '\^X\^U' undo
bindkey -e '\^[\^R' undo-all
bindkey -e '\^[r' undo-all
bindkey -e '\^[R' undo-all
bindkey -e '\^I' complete-next-candidate
bindkey -e '\bt' complete-prev-candidate
bindkey -e '\^[=' complete-list
bindkey -e '\^[?' complete-list
bindkey -e '\^[*' complete-all
bindkey -e '\^T' emacs-transpose-chars
bindkey -e '\^[t' emacs-transpose-words
bindkey -e '\^[T' emacs-transpose-words
bindkey -e '\^[l' emacs-downcase-word
bindkey -e '\^[L' emacs-downcase-word
bindkey -e '\^[u' emacs-upcase-word
bindkey -e '\^[U' emacs-upcase-word
bindkey -e '\^[c' emacs-capitalize-word
bindkey -e '\^[C' emacs-capitalize-word
bindkey -e '\^[\\' emacs-delete-horizontal-space
bindkey -e '\^[ ' emacs-just-one-space
bindkey -e '\^S' emacs-search-forward
bindkey -e '\^R' emacs-search-backward
bindkey -e '\^[<' oldest-history-eol
bindkey -e '\^[>' return-history-eol
bindkey -e '\D' next-history-eol
bindkey -e '\^N' next-history-eol
bindkey -e '\U' prev-history-eol
bindkey -e '\^P' prev-history-eol
__END__

testcase "$LINENO" 'printing default emacs bindings: output' \
    4<emacs 5</dev/null 3<<\__IN__
bindkey -e | sort
__IN__

test_x -e 0 'printing default emacs bindings: exit status'
bindkey -e
__IN__

test_OE -e 0 'binding key (emacs)'
bindkey -e '\^N' search-again
__IN__

test_oE -e 0 'printing bound key (emacs)'
bindkey -e '\^N' search-again
bindkey -e '\^N'
__IN__
bindkey -e '\^N' search-again
__OUT__

test_OE -e 0 'unbinding key (emacs)'
bindkey -e '\D' -
__IN__

test_Oe -e 1 'printing unbound key (emacs)'
bindkey -e '\D' -
bindkey -e '\D'
__IN__
bindkey: key sequence `\D' is not bound
__ERR__
#`

while read -r _ _ key _; do
    printf 'bindkey -v %s -\n' "$key"
done <vi_insert >vi_insert_x

test_OE -e 0 'removing all default vi-insert bindings'
. ./vi_insert_x
bindkey -v
__IN__

testcase "$LINENO" 'restoring bindings from previous output' \
    4<vi_insert 5</dev/null 3<<\__IN__
default=$(bindkey -v)
. ./vi_insert_x
eval "$default"
bindkey -v | sort
__IN__

test_OE 'all commands are bindable' -e
for cmd in $(bindkey -l)
do
    bindkey -v !!! $cmd
    bindkey -a !!! $cmd
    bindkey -e !!! $cmd
done
bindkey -v !!! -
bindkey -a !!! -
bindkey -e !!! -
__IN__

test_Oe -e 1 'binding empty sequence'
bindkey -v '' self-insert
__IN__
bindkey: cannot bind an empty key sequence
__ERR__

test_Oe -e 1 'printing empty sequence'
bindkey -v ''
__IN__
bindkey: key sequence `' is not bound
__ERR__
#`

test_Oe -e 1 'binding to non-existing command'
bindkey -a '\\' no-such-command
__IN__
bindkey: no such editing command `no-such-command'
__ERR__
#`

test_Oe -e 2 'invalid option'
bindkey --no-such-option
__IN__
bindkey: `--no-such-option' is not a valid option
__ERR__
#`

test_O -e 2 'ambiguous long option, exit status and standard output'
bindkey --vi-
__IN__

test_o 'ambiguous long option, standard error'
bindkey --vi- 2>&1 | head -n 1
__IN__
bindkey: option `--vi-' is ambiguous
__OUT__
#`

test_Oe -e 2 'missing argument'
bindkey
__IN__
bindkey: no option is specified
__ERR__

test_Oe -e 2 'too many operands with -a'
bindkey -a X Y Z
__IN__
bindkey: too many operands are specified
__ERR__

test_Oe -e 2 'operand with -l'
bindkey -l X
__IN__
bindkey: no operand is expected
__ERR__

test_O -d -e 1 'printing to closed stream (-l)'
bindkey -l >&-
__IN__

test_O -d -e 1 'printing to closed stream (-a)'
bindkey -a >&-
__IN__

test_O -d -e 127 'bindkey built-in is unavailable in POSIX mode' --posix
echo echo not reached > bindkey
chmod a+x bindkey
PATH=$PWD:$PATH
bindkey --help
__IN__

# vim: set ft=sh ts=8 sts=4 sw=4 et:
