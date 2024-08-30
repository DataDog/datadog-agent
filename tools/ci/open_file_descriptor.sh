#!/bin/bash
# Create a file descriptor on a temporary file
# https://unix.stackexchange.com/questions/181937/how-create-a-temporary-file-in-shell-script

# Create the temp file
tmpfile=$(mktemp /tmp/gitlab_store.XXXXXX)
# create file descriptor 3 for writing to a temporary file so that
# echo ... >&3 writes to that file
exec 3>"$tmpfile"

# create file descriptor 4 for reading from the same file so that
# the file seek positions for reading and writing can be different
exec 4<"$tmpfile"

# delete temp file; the directory entry is deleted at once; the reference counter
# of the inode is decremented only after the file descriptor has been closed.
# The file content blocks are deallocated (this is the real deletion) when the
# reference counter drops to zero.
rm "$tmpfile"

# Set a meaningful name to retrieve secrets from the file descriptor
function pop_front() {
    head -n 1 <&4
}
