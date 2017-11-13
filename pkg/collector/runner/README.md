### Testing the number of runner check workers

This package includes a comprehensive test for comparing the efficiency of running checks with differing numbers of check workers.  

To run this test, use the following command:  
`go test {path_to_runner_package} -run TestNumWorkersEfficiency -v -args -efficiency [optional args]`

The test is configured by run-time flags. The available flags are described below.  

| Flag | Description |
|---|---|
| -static     | Run the check with the default number of workers (vs a dynamic number)                             |
| -lazy       | Run the check using a lazy wait (vs a busy wait)                                                   |
| -python     | Use a python check (vs a golang check)                                                             |
| -memory     | Run the memory tests: involves running 5 sets of checks and displaying the memory stats after each |
| -granularity={} | (_For memory test_) How many checks to run in each set - default 100                           |
| -wait={}    | How long each test check will take to complete (ms) - default 100                                  |

_Note: there's a bug in the python checks test causing the tests to crash occasionally_
