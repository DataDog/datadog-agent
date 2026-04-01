"""Rules for managing proto-generated source files."""

load("@bazel_lib//lib:write_source_files.bzl", "write_source_files")
load("@bazel_skylib//rules:select_file.bzl", "select_file")

def update_proto_pb_go(name, go_proto_library, srcs):
    """Updates checked-in .pb.go files from a go_proto_library target."""
    gen_srcs = "{}_srcs".format(name)
    native.filegroup(name = gen_srcs, srcs = [go_proto_library], output_group = "go_generated_srcs")
    files = {}
    for src in srcs:
        gen_src = "{}_{}".format(name, src)
        select_file(name = gen_src, srcs = gen_srcs, subpath = src)
        files[src] = gen_src
    write_source_files(name = name, files = files)
