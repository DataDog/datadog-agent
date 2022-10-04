from lib.cspm.finding import is_subset


def expect_findings(test_case, findings, expected_findings):
    findings_by_rule = {}
    for agent_rule_id, rule_findings in findings.items():
        findings_by_rule.setdefault(agent_rule_id, []).extend(rule_findings)
        for finding in rule_findings:
            print(f"finding {agent_rule_id} {finding['result']} {finding['resource_id']}")

    for rule_id, expected_rule_findings in expected_findings.items():
        for expected_rule_finding in expected_rule_findings:
            test_case.assertIn(rule_id, findings_by_rule)
            found = False
            rule_findings = findings_by_rule.get(rule_id, [])
            for finding in rule_findings:
                if is_subset(expected_rule_finding, finding):
                    found = True
                    break

            test_case.assertTrue(found, f"unexpected finding {finding} for rule {rule_id}")
            del findings_by_rule[rule_id]

    for rule_id, rule_findings in findings_by_rule.items():
        for finding in rule_findings:
            result = finding["result"]
            print(f"finding {rule_id} {result}")

    for rule_id, rule_findings in findings_by_rule.items():
        for finding in rule_findings:
            result = finding["result"]
            test_case.assertNotIn(
                result, ("failed", "error"), f"finding for rule {rule_id} not expected to be in failed or error state"
            )
