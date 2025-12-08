# Testing Complex Regex Patterns with the LLM

## Overview

I've created a comprehensive testing framework for the regex-to-token translator. Here's what you can do:

## 1. Mock Testing (No LLM Required)

Run this to see expected outputs for various complexity levels:

```bash
python3 pkg/logs/processor/test_translate.py
```

This shows 13 different patterns from simple (SSN) to complex (UUID, JWT) to unsuitable (alternation patterns).

## 2. Real LLM Testing (In DevContainer)

Switch to your devcontainer terminal and run:

```bash
# Simple pattern
dda inv logs-processor.translate-regex-to-tokens \
  --regex-pattern='\d{3}-\d{2}-\d{4}' \
  --rule-name='auto_redact_ssn'

# Complex pattern - UUID
dda inv logs-processor.translate-regex-to-tokens \
  --regex-pattern='[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}' \
  --rule-name='detect_uuid' \
  --replacement='[UUID_REDACTED]'

# Very complex - AWS Access Key
dda inv logs-processor.translate-regex-to-tokens \
  --regex-pattern='AKIA[0-9A-Z]{16}' \
  --rule-name='auto_redact_aws_key' \
  --description='AWS IAM access key ID'
```

## 3. Test Cases Document

See `COMPLEX_TEST_CASES.md` for 12 detailed test cases with:
- Full command to run
- Expected token output
- Prefilter keywords
- Limitations and notes
- Suitability assessment

## What the Mock Test Shows

The mock test demonstrates the translator's behavior across different complexity levels:

### ✅ Simple Patterns (Ideal)
- **SSN**: `\d{3}-\d{2}-\d{4}` → `[D3, Dash, D2, Dash, D4]`
- **Date**: `\d{4}-\d{2}-\d{2}` → `[D4, Dash, D2, Dash, D2]`

### ⚠️ Medium Complexity (Workable)
- **IPv4**: `\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}` → `[D1, Period, D1, Period, D1, Period, D1]`
  - Note: D1 matches 1-10+ digits, will overmatch
- **Credit Card**: Needs 3 separate rules for different separator formats

### 🔧 Complex (Possible but Limited)
- **Email**: `[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Z|a-z]{2,}` → `[C1, At, C1, Period, C1]`
  - Note: Will produce false positives
- **JWT**: `[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+` → `[C1, Period, C1, Period, C1]`
  - Note: Cannot verify base64 encoding
- **UUID**: `[0-9a-fA-F]{8}-...` → `[C8, Dash, C4, ...]`
  - Note: C tokens match any chars, not just hex

### ❌ Not Suitable
- **Alternation**: `(ERROR|WARN|INFO)` - Cannot express with tokens
- **Variable quantifiers**: `[a-z]+` - Cannot express `+` or `*`
- **Range validation**: `(25[0-5]|2[0-4][0-9]|...)` - Too complex

## Key Insights from Testing

1. **Prefilter keywords are crucial** - They enable fast rejection of non-matching logs
2. **Fixed-length patterns are ideal** - No ambiguity in matching
3. **Variable-length patterns overmatch** - `C1` and `D1` match 1-10+ chars/digits
4. **Character class restrictions are lost** - `[0-9a-fA-F]` becomes generic `C`
5. **Optional elements need multiple rules** - `[\s-]?` requires separate rules for each case

## Recommended Test Sequence

1. **Start with the mock test** to understand expected behavior
2. **Try a simple pattern** in devcontainer (SSN or date)
3. **Test a medium pattern** (UUID or AWS key)
4. **Try a complex pattern** (email or JWT) to see limitations
5. **Review the generated code** and note any warnings from the LLM

## Files Created

1. **`test_translate.py`** - Mock translator with 13 test cases
2. **`COMPLEX_TEST_CASES.md`** - Detailed test documentation
3. **`QUICKSTART.md`** - Quick start guide for users
4. **`TOKEN_RULES_README.md`** - Complete reference documentation
5. **`translate_regex.py`** - Standalone LLM-based translator
6. **`tasks/logs_processor.py`** - Invoke task integration

## Next Steps

To convert existing regex rules from `comp/logs/agent/config/processing_rules.go`:

1. Find a regex pattern you want to convert
2. Run it through the translator (in devcontainer)
3. Review the LLM's suggestions and warnings
4. Add the generated Go code to `pkg/logs/processor/processing_rules.go`
5. Test with real log data to verify accuracy

## Performance Considerations

The LLM (Qwen2.5-Coder-3B) will:
- Download ~6.5GB on first run
- Take 10-30 seconds per translation
- Require GPU/CPU resources

Consider batch translating multiple patterns in one session to amortize the model loading time.

## Evaluating Results

When reviewing LLM output, check:
1. **Are the tokens correct?** (D for digits, C for chars, etc.)
2. **Are prefilter keywords present?** (Speeds up matching significantly)
3. **Are there warnings/notes?** (LLM will note limitations)
4. **Will it overmatch?** (Generic tokens like C1 match any chars)
5. **Do you need multiple rules?** (For optional elements)

If the LLM suggests a pattern is too complex, consider keeping it as a regex rule in `comp/logs/agent/config/processing_rules.go`.

