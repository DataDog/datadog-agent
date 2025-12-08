# Quick Start Guide: Regex to Token Translation

## Important: Run from DevContainer!

The `transformers`, `torch`, and `accelerate` libraries are installed in your **devcontainer environment**, not on your macOS host machine.

### How to Run the Command

**In your DevContainer terminal** (the one that shows `datadog@...` prompt):

```bash
dda inv logs-processor.translate-regex-to-tokens \
  --regex-pattern='\d{3}-\d{2}-\d{4}' \
  --rule-name='ssn' \
  --replacement='[SSN_REDACTED]'
```

Or use the standalone script directly:

```bash
python3 pkg/logs/processor/translate_regex.py \
  --regex '\d{3}-\d{2}-\d{4}' \
  --name 'ssn' \
  --replacement '[SSN_REDACTED]'
```

### Installing Dependencies (Only Needed in DevContainer)

If you haven't already installed the dependencies in your devcontainer:

```bash
pip install transformers torch accelerate
```

### Example Translations

Here are some common patterns to try:

1. **Social Security Number**
   ```bash
   dda inv logs-processor.translate-regex-to-tokens \
     --regex-pattern='\d{3}-\d{2}-\d{4}' \
     --rule-name='auto_redact_ssn' \
     --replacement='[SSN_REDACTED]'
   ```

2. **Credit Card (16 digits)**
   ```bash
   dda inv logs-processor.translate-regex-to-tokens \
     --regex-pattern='\d{4}[\s-]?\d{4}[\s-]?\d{4}[\s-]?\d{4}' \
     --rule-name='auto_redact_cc' \
     --replacement='[CC_REDACTED]'
   ```

3. **IPv4 Address**
   ```bash
   dda inv logs-processor.translate-regex-to-tokens \
     --regex-pattern='\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}' \
     --rule-name='detect_ipv4' \
     --replacement='[IP_REDACTED]'
   ```

4. **Email Address**
   ```bash
   dda inv logs-processor.translate-regex-to-tokens \
     --regex-pattern='[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Z|a-z]{2,}' \
     --rule-name='auto_redact_email' \
     --replacement='[EMAIL_REDACTED]'
   ```

5. **API Key (hex string)**
   ```bash
   dda inv logs-processor.translate-regex-to-tokens \
     --regex-pattern='[0-9a-fA-F]{32}' \
     --rule-name='auto_redact_api_key' \
     --replacement='[API_KEY_REDACTED]'
   ```

### Output

The script will generate Go code that you can add to `pkg/logs/processor/processing_rules.go`:

```go
{
    Name: "auto_redact_ssn",
    Type: RuleTypeToken,
    TokenPattern: []tokens.Token{
        tokens.D3, tokens.Dash, tokens.D2, tokens.Dash, tokens.D4,
    },
    Replacement: []byte("[SSN_REDACTED]"),
    PrefilterKeywords: [][]byte{
        []byte("-"),
    },
}, // Matches SSN format XXX-XX-XXXX
```

### Troubleshooting

**Error: "transformers library not installed"**
- You're running from your macOS terminal instead of the devcontainer
- Switch to the devcontainer terminal (should show `datadog@...` prompt)

**Error: "externally-managed-environment"**
- You're on macOS which prevents system-wide pip installs
- The packages are already installed in your devcontainer - use that instead!

**SyntaxWarning about invalid escape sequence**
- This has been fixed - the docstring now uses `r"""` to mark it as a raw string

### What Happens Behind the Scenes

1. The invoke task calls the standalone Python script
2. The script loads Qwen2.5-Coder-3B (~6.5GB download on first run)
3. The model analyzes your regex and suggests token patterns
4. The script generates ready-to-use Go code with prefilter keywords
5. You copy the generated code into `processing_rules.go`

### Converting Existing Rules

To convert the regex rules from `comp/logs/agent/config/processing_rules.go`:

1. Find the regex pattern in the config
2. Run the translation command with that pattern
3. Review the suggested token pattern
4. Add it to `pkg/logs/processor/processing_rules.go`

See `pkg/logs/processor/TOKEN_RULES_README.md` for complete documentation.

