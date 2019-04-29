resource "local_file" "hosts" {
  content     = "${local.hostsfile}"
  filename = "${path.root}/hosts"
}

resource "local_file" "makefile" {
  content     = "${local.Makefile}"
  filename = "${path.root}/Makefile"
}
