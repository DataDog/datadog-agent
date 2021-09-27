package demo_import

valid_container(c) {
  c.inspect.HostConfig.Memory != 0
}
