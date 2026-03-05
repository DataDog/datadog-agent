# path-p.tst: test of pathname expansion for any POSIX-compliant shell

posix="true"

mkdir -p foo/dir foo/no_read_dir foo/no_search_dir
>foo/dir/file >foo/no_read_dir/file >foo/no_search_dir/file
chmod a-r foo/no_read_dir
chmod a-x foo/no_search_dir

mkdir -p "bar/a[b/c]d"

mkdir -p baz/.dir/.file

test_oE 'expansion with read-and-searchable directory'
echo foo/*dir
echo foo/d*r/f*e
__IN__
foo/dir foo/no_read_dir foo/no_search_dir
foo/dir/file
__OUT__

test_oE '* does not match slash'
echo foo*dir*file
__IN__
foo*dir*file
__OUT__

test_oE '? does not match slash'
echo foo?dir?file
__IN__
foo?dir?file
__OUT__

test_oE '[...] does not match slash'
echo foo[/]dir[/]file
echo bar/a[b/c]d
__IN__
foo[/]dir[/]file
bar/a[b/c]d
__OUT__

test_oE '* does not match initial dot'
echo baz/*dir/*file
__IN__
baz/*dir/*file
__OUT__

test_oE '? does not match initial dot'
echo baz/?dir/?file
__IN__
baz/?dir/?file
__OUT__

test_oE '[!...] does not match initial dot'
echo baz/[!a]dir/[!1-9]file
__IN__
baz/[!a]dir/[!1-9]file
__OUT__

: This is not yet mandatory in POSIX, so it is tested in path-y.sh <<'__OUT__'
test_oE 'no pattern matches . or ..'
echo .*/ # should not print . or ..
__IN__
.dir/
__OUT__

test_oE 'literal . and .. are not filtered out'
echo b*/../.
__IN__
bar/../. baz/../.
__OUT__

(
# Skip if we're root.
if { ls for/dir || <foo/no_search_dir/file; } 2>/dev/null; then
    skip="true"
fi

test_oE 'expansion with unreadable directory'
echo f*o/no_read_dir
echo foo/no_read_d*r
echo foo/no_read_d*r/file
echo foo/no_read_dir/f*e
echo foo/no_read_d*r/f*e
__IN__
foo/no_read_dir
foo/no_read_dir
foo/no_read_dir/file
foo/no_read_dir/f*e
foo/no_read_d*r/f*e
__OUT__

test_oE 'expansion with unsearchable directory'
echo f*o/no_search_dir
echo foo/no_search_d*r
echo foo/no_search_d*r/file
# echo foo/no_search_dir/f*e
echo foo/no_search_d*r/f*e
__IN__
foo/no_search_dir
foo/no_search_dir
foo/no_search_d*r/file
foo/no_search_dir/file
__OUT__
# POSIX says: "Any component, except the last, that does not contain any '*',
# '?' or '[' characters that will be treated as special shall require search
# permission." This statement is not actually correct. If the last component
# contains a pattern character, only read permission is required for its parent
# directory. The expansion of foo/no_search_dir/f*e succeeds in many shells
# despite the POSIX requirement, so the line for this case is commented out.

)

test_oE 'pathnames are sorted according to current collating sequence'
echo [fb]*/
__IN__
bar/ baz/ foo/
__OUT__

# vim: set ft=sh ts=8 sts=4 sw=4 et:
