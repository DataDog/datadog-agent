# Building attr

libacl depends on libattr, from the same people.
It is only used in openscap, so we only have to build for linux.

For LGPL, we play it safe and make just a .so file, which openscap refers to.

# Updating

Point at the new download and see if it builds. If symbols go missing,
then add the new .c files.

# How to rebuild rules from scratch

We should only have to do this once.

### Make fake attr

Build the deps and put them where we can find them later
```
bazel run @attr//:install -- --dest_dir /tmp/attr
```

###
- Download the current libacl sources
- untar. Leaves the directory acl-2.3.1
- cp acl-2.3.1/configure acl-2.3.1/dd.configure
  - remove the checks for the attr.h and just define the vars
- mkdir work
- cd work
- The normal config adds --disable-static, but we want to build the lib here to see all the .o files.
- ../acl-2.3.1/dd.configure --disable-nls
- Whack the makefile to fix these lines
  -  DEFAULT_INCLUDES = -I. -I$(srcdir) -I$(top_builddir)/include -I/tmp/attr/include
  -  LIBS = -L/tmp/attr/lib -lattr
- make

### make the BUILD file.

The usual list of things

- copy config.h to overlay/config.h.  There are no platform or arch issues, so we only need one.
- Add all the sources from libacl.
- And it turns out we need misc to
- Add the minimal defines.
  - some HAVE_ bits were tricky.
- figure out their knotty include system.
