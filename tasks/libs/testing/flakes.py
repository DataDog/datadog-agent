def get_tests_family_if_failing_tests(test_name_list, failing_tests: set):
    """
    Get the parent tests of a list of tests only if the marked test is failing
    For example with the test ["TestEKSSuite/TestCPU/TestCPUUtilization", "TestKindSuite/TestCPU"]
    this method should return the set{"TestEKSSuite/TestCPU/TestCPUUtilization", "TestEKSSuite/TestCPU", "TestEKSSuite", "TestKindSuite/TestCPU", "TestKindSuite"}
    if TestKindSuite/TestCPU and TestEKSSuite/TestCPU/TestCPUUtilization are failing
    Another example, with the test ["TestEKSSuite/TestCPU/TestCPUUtilization", "TestKindSuite/TestCPU"]
    if only TestKindSuite/TestCPU is failing, the method should return the set{"TestKindSuite/TestCPU", "TestKindSuite"}
    """
    test_name_set = set(test_name_list)
    marked_tests_failing = failing_tests.intersection(test_name_set)
    return get_tests_family(list(marked_tests_failing))


def get_tests_family(test_name_list):
    """
    Get the parent tests of a list of tests
    For example with the test ["TestEKSSuite/TestCPU/TestCPUUtilization", "TestKindSuite/TestCPU"]
    this method should return the set{"TestEKSSuite/TestCPU/TestCPUUtilization", "TestEKSSuite/TestCPU", "TestEKSSuite", "TestKindSuite/TestCPU", "TestKindSuite"}
    """
    test_family = set(test_name_list)
    for test_name in test_name_list:
        while test_name.count('/') > 0:
            test_name = test_name.rsplit('/', 1)[0]
            test_family.add(test_name)
    return test_family


def is_known_flaky_test(failing_test, known_flaky_tests, known_flaky_tests_parents):
    """
    Check if a test is known to be flaky
    The method should be called with the following arguments:
    - failing_test: the test that is failing
    - known_flaky_tests: the set of tests that are known to be flaky
    - known_flaky_tests_parents: the set of tests that are ancestors of a known flaky test, thus would fail when the flaky leaf test fails
    If a test is a parent of a test that is known to be flaky, the test should be considered flaky
    For example:
    - if TestEKSSuite/TestCPU is known to be flaky, TestEKSSuite/TestCPU/TestCPUUtilization should be considered flaky
    - if TestEKSSuite/TestCPU is known to be flaky, TestEKSSuite should be considered flaky unless TestEKSSuite/TestCPU is not failing
    - if TestEKSSuite/TestCPU is known to be flaky, TestEKSSuite/TestMemory should not be considered flaky
    """

    failing_test_parents = get_tests_family([failing_test])

    if any(parent in known_flaky_tests for parent in failing_test_parents):
        return True

    return failing_test in known_flaky_tests_parents
