# Running tests

* Running all tests:

```bash
deva inv -e security-agent.functional-tests --verbose --skip-linters --testflags "-test.run '.*'"
```

* Running a single test:

```bash
deva inv -e security-agent.functional-tests --verbose --skip-linters --testflags "-test.run 'TestConnect'"
```

* Running ebpfless tests:

```bash
deva inv -e security-agent.ebpfless-functional-tests --verbose --skip-linters --testflags "-test.run '.*'"
```
