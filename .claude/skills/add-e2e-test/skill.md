---
name: add-e2e-test
description: Guide developers through adding a new E2E test in the new-e2e framework, helping them choose the right environment and setup
allowed-tools: Read, Glob, Grep, AskUserQuestion, Write, Edit
argument-hint: "<test-name-or-description>"
---

Guide developers through creating a new E2E test in `test/new-e2e/tests/`. This skill helps developers choose the right environment setup (existing provisioner vs custom) and walks them through the implementation based on best practices from the agent-health E2E test development.

## Instructions

### 1. Understand the Test Requirements

Parse `$ARGUMENTS` to understand what the developer wants to test. If unclear, ask:
- What feature/behavior are they testing?
- What infrastructure do they need (VM, containers, Kubernetes, etc.)?
- Does it need special setup (Docker, specific OS, networking)?

### 2. Choose Environment Type

Ask the developer to choose their environment approach using `AskUserQuestion`:

**Question**: "Which environment setup fits your test needs?"

**Options**:
1. **Existing AWS Host** (Simple) — Use `awshost.Provisioner` for basic VM + Agent
   - Description: "Basic EC2 VM with agent. Good for simple agent behavior tests."

2. **AWS Host + Docker** (Moderate) — Use `awshost.Provisioner` + `scenec2.WithDocker()`
   - Description: "EC2 VM with Docker installed. Agent runs on host, Docker available for testing."

3. **Kubernetes** (Complex) — Use existing k8s provisioners
   - Description: "Full Kubernetes cluster. Good for cluster-agent, DCA, or k8s-specific tests."

4. **Custom Pulumi Provisioner** (Recommended for complex setups)
   - Description: "Full control over infrastructure. Use when existing provisioners don't fit your needs (example: agent-health test with Docker Compose)."

### 3. Implementation Path

Based on their choice:

#### Path A: Using Existing Provisioners (Options 1-3)

1. **Find similar tests** using Glob/Grep in `test/new-e2e/tests/`
2. **Show examples** of tests using that provisioner type
3. **Guide them** to copy the pattern and adapt it

#### Path B: Custom Provisioner (Option 4) — RECOMMENDED FOR COMPLEX CASES

Follow the agent-health test pattern from `test/new-e2e/tests/agent-health/DEVELOPMENT_LOG.md`:

1. **Create test package structure**:
   ```
   test/new-e2e/tests/<test-name>/
   ├── provisioner.go              # Infrastructure code
   ├── <test-name>_test.go         # Test logic
   └── fixtures/
       ├── agent_config.yaml       # Agent configuration
       └── <other-fixtures>        # Docker Compose, scripts, etc.
   ```

2. **Implement provisioner.go**:
   - Create custom Pulumi provisioner extending base components
   - Use `awshost.Provisioner` as base
   - Add required components (Docker, FakeIntake, containers, etc.)
   - See `test/new-e2e/tests/agent-health/provisioner.go` as reference

3. **Implement test file**:
   - Create test suite struct
   - Embed custom environment
   - Use `//go:embed` for fixtures
   - Write test methods
   - Use FakeIntake client APIs to validate agent behavior

4. **Key patterns to follow**:
   - **Separate concerns**: Provisioner vs test logic in different files
   - **Use FakeIntake**: Validate agent output through FakeIntake client
   - **Fixture files**: Store configs externally with `//go:embed`
   - **Docker Compose**: Use for declarative container management if needed
   - **Official APIs**: Use client helpers instead of custom HTTP calls

### 4. Essential Files to Reference

Point the developer to:
- **Example**: `test/new-e2e/examples/customenv_with_docker_app_test.go`
- **Agent-health test**: `test/new-e2e/tests/agent-health/` (full custom provisioner example)
- **Framework docs**: `test/new-e2e/README.md`
- **FakeIntake aggregators**: `test/fakeintake/aggregator/`

### 5. Implementation Steps

Walk through these steps with the developer:

1. **Create directory structure** under `test/new-e2e/tests/<test-name>/`
2. **Implement provisioner** (if custom) or select existing one
3. **Create test file** with test suite and methods
4. **Add fixtures** (agent config, docker-compose, etc.)
5. **Implement validation logic** using FakeIntake or other assertions
6. **Add unit tests** for any custom aggregators or parsers
7. **Test locally** using `/run-e2e <test-name>`

### 6. Key Design Decisions to Highlight

Based on agent-health development experience:

- ✅ **Start simple**: Begin with basic setup, add complexity as needed
- ✅ **Use official APIs**: Prefer framework helpers over custom code
- ✅ **Separate concerns**: Keep provisioner and test logic in separate files
- ✅ **Follow examples**: Copy patterns from working tests
- ✅ **Defensive coding**: Add bounds checks and clear error messages
- ✅ **FakeIntake validation**: Primary way to verify agent behavior
- ✅ **Embedded fixtures**: Use `//go:embed` for configuration files

### 7. Common Pitfalls to Avoid

- ❌ Don't manually parse agent output when FakeIntake clients exist
- ❌ Don't inline large configs; use fixture files
- ❌ Don't mix provisioner and test logic in one file
- ❌ Don't forget to use `//go:embed` for fixtures
- ❌ Don't create custom HTTP helpers when official APIs exist

### 8. Testing the Test

Once implemented, help them run it:
```bash
# Run the test
/run-e2e tests/<test-name>

# Debug with stack kept alive
/run-e2e tests/<test-name> --keep-stack

# Run specific test function
/run-e2e tests/<test-name> --run TestSpecificCase
```

## Example Interaction Flow

1. Developer invokes: `/add-e2e-test docker permission check`
2. Skill asks: "What infrastructure do you need?" → User selects "Custom Provisioner"
3. Skill creates directory structure
4. Skill guides through provisioner implementation
5. Skill helps create test file with FakeIntake validation
6. Skill adds fixtures
7. Skill suggests running `/run-e2e` to test

## Output

- Create all necessary files with proper structure
- Provide explanations for each component
- Reference relevant examples and documentation
- Offer to run the test when ready

## References

- Development log: `test/new-e2e/tests/agent-health/DEVELOPMENT_LOG.md`
- Custom env example: `test/new-e2e/examples/customenv_with_docker_app_test.go`
- Framework README: `test/new-e2e/README.md`
- Agent-health implementation: `test/new-e2e/tests/agent-health/`
