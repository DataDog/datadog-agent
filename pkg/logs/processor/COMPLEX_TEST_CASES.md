# Complex Regex Test Cases for Token Translation

This document provides test cases for the regex-to-token translator, showing expected behavior for various complexity levels.

## Test Commands

Run these in your devcontainer terminal:

### 1. Simple Fixed-Length Pattern (SSN)

```bash
dda inv logs-processor.translate-regex-to-tokens \
  --regex-pattern='\d{3}-\d{2}-\d{4}' \
  --rule-name='auto_redact_ssn' \
  --replacement='[SSN_REDACTED]' \
  --description='Social Security Number'
```

**Expected Output:**
- Tokens: `[D3, Dash, D2, Dash, D4]`
- Prefilter: `["-"]`
- Excellent candidate - fixed length, distinctive separator

### 2. UUID Pattern

```bash
dda inv logs-processor.translate-regex-to-tokens \
  --regex-pattern='[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}' \
  --rule-name='detect_uuid' \
  --replacement='[UUID_REDACTED]' \
  --description='UUID format'
```

**Expected Output:**
- Tokens: `[C8, Dash, C4, Dash, C4, Dash, C4, Dash, C12]`
- Prefilter: `["-"]`
- Good candidate - fixed structure, C tokens will match hex chars

**Note:** C tokens match any characters, not just hex. May have false positives.

### 3. AWS Access Key ID

```bash
dda inv logs-processor.translate-regex-to-tokens \
  --regex-pattern='AKIA[0-9A-Z]{16}' \
  --rule-name='auto_redact_aws_access_key' \
  --replacement='[AWS_KEY_REDACTED]' \
  --description='AWS access key ID starting with AKIA'
```

**Expected Output:**
- Tokens: `[NewLiteralToken("AKIA"), C16]`
- Prefilter: `["AKIA"]`
- Excellent candidate - distinctive prefix for prefiltering

### 4. ISO8601 Timestamp

```bash
dda inv logs-processor.translate-regex-to-tokens \
  --regex-pattern='\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}' \
  --rule-name='detect_iso_timestamp' \
  --replacement='[TIMESTAMP_REDACTED]' \
  --description='ISO8601 timestamp without timezone'
```

**Expected Output:**
- Tokens: `[D4, Dash, D2, Dash, D2, T, D2, Colon, D2, Colon, D2]`
- Prefilter: `["T"]`
- Good candidate - uses special T token, fixed length

**Note:** Doesn't match timezone suffixes (Z, +00:00) - would need separate rules.

### 5. Credit Card with Optional Separators

```bash
dda inv logs-processor.translate-regex-to-tokens \
  --regex-pattern='\d{4}[\s-]?\d{4}[\s-]?\d{4}[\s-]?\d{4}' \
  --rule-name='auto_redact_credit_card' \
  --replacement='[CC_REDACTED]' \
  --description='16-digit credit card with optional spaces or dashes'
```

**Expected Output:**
- LLM should recognize this needs **multiple rules**:
  1. Space-separated: `[D4, Space, D4, Space, D4, Space, D4]`
  2. Dash-separated: `[D4, Dash, D4, Dash, D4, Dash, D4]`
  3. No separator: `[D16]`
- Prefilter: Empty (no reliable keyword across all formats)

**Note:** Tokens don't support optional patterns. Need 3 separate rules.

### 6. Email Address (Complex)

```bash
dda inv logs-processor.translate-regex-to-tokens \
  --regex-pattern='[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Z|a-z]{2,}' \
  --rule-name='auto_redact_email' \
  --replacement='[EMAIL_REDACTED]' \
  --description='Email address'
```

**Expected Output:**
- Tokens: `[C1, At, C1, Period, C1]` (minimum length)
- Prefilter: `["@", "."]`
- **Significant limitations:**
  - `C1` matches 1-10+ chars (not just alphanumeric)
  - Will match `a@b.c` but also match longer emails
  - May produce false positives on strings like `word@word.word`

**Recommendation:** Email regex is complex - consider keeping as regex or using more specific token patterns for known email domains.

### 7. IPv4 Address with Validation

```bash
dda inv logs-processor.translate-regex-to-tokens \
  --regex-pattern='\b((25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.){3}(25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\b' \
  --rule-name='detect_ipv4_validated' \
  --replacement='[IP_REDACTED]' \
  --description='IPv4 with range validation (0-255 per octet)'
```

**Expected Output:**
- LLM should recognize this is **TOO COMPLEX** for tokens
- Tokens can do: `[D1, Period, D1, Period, D1, Period, D1]`
- But cannot validate ranges (0-255)

**Recommendation:** Keep as regex. Tokens cannot express alternation or range validation.

### 8. JWT Token

```bash
dda inv logs-processor.translate-regex-to-tokens \
  --regex-pattern='[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+' \
  --rule-name='auto_redact_jwt' \
  --replacement='[JWT_REDACTED]' \
  --description='JWT token (header.payload.signature)'
```

**Expected Output:**
- Tokens: `[C1, Period, C1, Period, C1]`
- Prefilter: `["."]`
- **Limitations:**
  - Cannot verify base64url encoding
  - Will match any `word.word.word` pattern
  - May need additional context-based filtering

### 9. Log Level with Alternation (TOO COMPLEX)

```bash
dda inv logs-processor.translate-regex-to-tokens \
  --regex-pattern='(ERROR|WARN|WARNING|INFO|DEBUG|TRACE)' \
  --rule-name='detect_log_level' \
  --description='Log level keywords'
```

**Expected Output:**
- **NOT SUITABLE FOR TOKENS**
- Tokens cannot express alternation `|`
- **Alternative:** Create separate rules with literal tokens:
  ```go
  tokens.NewLiteralToken("ERROR")
  tokens.NewLiteralToken("WARN")
  tokens.NewLiteralToken("WARNING")
  // etc...
  ```

### 10. Phone Number (US Format)

```bash
dda inv logs-processor.translate-regex-to-tokens \
  --regex-pattern='\(\d{3}\)\s\d{3}-\d{4}' \
  --rule-name='auto_redact_phone_us' \
  --replacement='[PHONE_REDACTED]' \
  --description='US phone: (123) 456-7890'
```

**Expected Output:**
- Tokens: `[Parenopen, D3, Parenclose, Space, D3, Dash, D4]`
- Prefilter: `["(", ")"]`
- Good candidate - very specific format

**Note:** Only matches this exact format. Other formats (123-456-7890, 1234567890) need separate rules.

### 11. Hex String (32 chars)

```bash
dda inv logs-processor.translate-regex-to-tokens \
  --regex-pattern='[0-9a-fA-F]{32}' \
  --rule-name='auto_redact_hex32' \
  --replacement='[HEX_REDACTED]' \
  --description='32-character hexadecimal string (MD5, API keys)'
```

**Expected Output:**
- Tokens: `[C32]`
- Prefilter: `[]` (empty - no distinctive keyword)
- **Limitation:** `C32` matches ANY 32 characters, not just hex

**Recommendation:** May produce false positives on any 32-char string. Consider if this is acceptable or keep as regex.

### 12. SQL Connection String (COMPLEX)

```bash
dda inv logs-processor.translate-regex-to-tokens \
  --regex-pattern='(mysql|postgresql|mongodb)://[^:]+:[^@]+@[^/]+/?' \
  --rule-name='auto_redact_db_connection' \
  --replacement='[DB_CONNECTION_REDACTED]' \
  --description='Database connection string with credentials'
```

**Expected Output:**
- **PARTIALLY SUITABLE**
- Can match the structure after the protocol:
  - `[Colon, Fslash, Fslash, C1, Colon, C1, At, C1, Fslash]`
- But cannot express:
  - Protocol alternation (mysql|postgresql|mongodb)
  - Character class negation ([^:])

**Recommendation:** Either:
1. Create separate rules for each protocol with literal tokens
2. Keep as regex for full validation

## Summary Table

| Pattern | Complexity | Token Suitable? | Notes |
|---------|-----------|----------------|-------|
| SSN | Simple | ✅ Excellent | Fixed length, good prefilter |
| UUID | Simple | ✅ Good | Fixed structure, may overmatch non-hex |
| AWS Key | Simple | ✅ Excellent | Distinctive prefix |
| ISO Timestamp | Medium | ✅ Good | Fixed format, special T token |
| Credit Card | Medium | ⚠️ Needs 3 rules | Optional separators require multiple rules |
| Email | Complex | ⚠️ Limited | Will overmatch, many false positives |
| IPv4 (validated) | Complex | ❌ No | Cannot validate ranges |
| JWT | Complex | ⚠️ Limited | Cannot verify base64, will overmatch |
| Log Level (alternation) | Complex | ❌ No | Cannot express alternation |
| Phone Number | Medium | ✅ Good | Specific format only |
| Hex String | Simple | ⚠️ Limited | Overmatches (all chars, not just hex) |
| DB Connection | Complex | ❌ No | Alternation and negation not supported |

## Key Insights

### When to Use Tokens:
1. **Fixed-length patterns** (SSN, UUID, dates)
2. **Known prefixes/suffixes** (AWS keys, log levels)
3. **Simple structure** with distinctive separators

### When to Keep Regex:
1. **Alternation** (`|`)
2. **Quantifiers** (`*`, `+`, `?`, `{n,m}`)
3. **Character classes** with specific ranges (`[0-9]` vs `[A-Z]`)
4. **Lookaheads/lookbehinds**
5. **Validation logic** (ranges, checksums)

### Hybrid Approach:
For many patterns, use **prefilter keywords** to quickly eliminate non-matching logs, then apply regex only to potential matches. This gives you the speed benefit of tokens with the accuracy of regex.

