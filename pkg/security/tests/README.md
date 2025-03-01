# Running tests

* Running all tests:

```bash
inv -e security-agent.functional-tests --verbose --skip-linters --testflags "-test.run '.*'"
```

* Running a single test:

```bash
inv -e security-agent.functional-tests --verbose --skip-linters --testflags "-test.run 'TestConnect'"
```

* Running ebpfless tests:

```bash
inv -e security-agent.ebpfless-functional-tests --verbose --skip-linters --testflags "-test.run '.*'"
```
