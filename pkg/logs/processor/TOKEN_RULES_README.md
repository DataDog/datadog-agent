# Token-Based Processing Rules Translation

This directory contains tools for translating regex-based processing rules to token-based rules using the [Qwen2.5-Coder-3B](https://huggingface.co/Qwen/Qwen2.5-Coder-3B) Small Language Model.

## Why Token-Based Rules?

Token-based rules are significantly faster than regex for pattern matching:
- No backtracking
- Structural matching (e.g., "3 digits, dash, 2 digits")
- Built-in prefiltering for quick rejection

## Setup

### Install Dependencies

```bash
pip install transformers torch accelerate
```

### Download Model (First Use)

The Qwen2.5-Coder-3B model will be automatically downloaded on first use (~6.5GB).

## Usage

### Method 1: Using Invoke Task

```bash
# Translate a single pattern
dda inv logs-processor.translate-regex-to-tokens \
  --regex-pattern='\d{3}-\d{2}-\d{4}' \
  --rule-name='ssn' \
  --replacement='[SSN_REDACTED]' \
  --description='Social Security Number'
```

### Method 2: Using Standalone Script

```bash
# Navigate to processor directory
cd pkg/logs/processor

# Translate SSN pattern
python translate_regex.py \
  --regex '\d{3}-\d{2}-\d{4}' \
  --name 'auto_redact_ssn' \
  --replacement '[SSN_REDACTED]' \
  --description 'Social Security Number'

# Translate IPv4 pattern
python translate_regex.py \
  --regex '\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}' \
  --name 'detect_ipv4' \
  --replacement '[IP_REDACTED]' \
  --description 'IPv4 address'

# Credit card pattern
python translate_regex.py \
  --regex '\d{4}[\s-]?\d{4}[\s-]?\d{4}[\s-]?\d{4}' \
  --name 'auto_redact_credit_card' \
  --replacement '[CC_REDACTED]'
```

## Token Types Reference

### Digit Runs
- `D1` through `D10`: Runs of 1-10+ consecutive digits

### Character Runs
- `C1` through `C10`: Runs of 1-10+ consecutive letters

### Special Characters
- `Dash`, `Period`, `Colon`, `Underscore`, `Fslash`, `Comma`
- `At`, `Space`, `Plus`, `Equal`
- `Parenopen`, `Parenclose`
- And more...

### Special Tokens
- `Month`: JAN, FEB, MAR, etc.
- `Day`: MON, TUE, WED, etc.
- `Zone`: UTC, GMT, EST, PST, etc.
- `T`: Time separator

## Example Translations

### SSN Pattern
```
Regex: \d{3}-\d{2}-\d{4}
Tokens: [D3, Dash, D2, Dash, D4]
Prefilter: ["-"]
Go Code:
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
},
```

### Date Pattern
```
Regex: \d{4}-\d{2}-\d{2}
Tokens: [D4, Dash, D2, Dash, D2]
Prefilter: ["-"]
```

### UUID Pattern
```
Regex: [0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}
Tokens: [C8, Dash, C4, Dash, C4, Dash, C4, Dash, C10, C2]
Prefilter: ["-"]
Note: Hex digits are treated as characters (C tokens)
```

## Limitations

### Token patterns cannot handle:
- Variable quantifiers: `{1,3}`, `+`, `*`, `?`
- Alternations: `(a|b)`
- Lookaheads/lookbehinds
- Character classes with ranges: `[0-9a-f]` (treated as generic characters)

### Workarounds:
- **Variable length**: Choose minimum case (e.g., `\d{1,3}` → `D1`)
- **Alternations**: Create separate rules for each alternative
- **Complex patterns**: Keep as regex-based rules

## Adding Prefilter Keywords

Prefilter keywords are literal strings that must be present for the pattern to possibly match. They enable fast rejection without tokenization.

### Good Prefilter Keywords:
- Distinctive punctuation: `"-"` for SSN, `"@"` for email
- Multiple keywords: `["@", "."]` for email
- Rare character sequences

### Bad Prefilter Keywords:
- Common characters: `" "` (space)
- Empty arrays when pattern has distinctive literals
- Too many keywords (diminishing returns)

## Integration

After generating token rules:

1. Add to `processing_rules.go`:
```go
var tokenRules = []*TokenBasedProcessingRule{
    // ... paste generated rules here
}
```

2. The `ProcessingRuleApplicator` will automatically use them with prefiltering

3. Test with unit tests

## Model Information

- Model: [Qwen2.5-Coder-3B](https://huggingface.co/Qwen/Qwen2.5-Coder-3B)
- Size: 3.09B parameters
- Context: 32,768 tokens
- Specialization: Code generation and understanding
- License: Qwen Research License

## See Also

- `processing_rule_applicator.go`: Token rule application logic
- `processing_rules.go`: Token rule definitions
- `tokenizer.go`: Tokenization implementation
- `tokens/tokens.go`: Token type definitions

