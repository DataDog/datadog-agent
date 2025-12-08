# Regex-to-Token Translation System - Documentation Index

Welcome to the regex-to-token translation system for the Datadog Agent logs processor!

## 📖 Start Here

**New to this system?** Start with these documents in order:

1. **[QUICK_REFERENCE.md](QUICK_REFERENCE.md)** ⭐ **START HERE**
   - One-page reference card
   - Quick commands and examples
   - Suitability matrix
   - Common issues

2. **[QUICKSTART.md](QUICKSTART.md)**
   - Installation instructions
   - Basic usage examples
   - Troubleshooting guide
   - Running your first translation

3. **[IMPLEMENTATION_SUMMARY.md](IMPLEMENTATION_SUMMARY.md)**
   - Complete system overview
   - Architecture and workflow
   - All files explained
   - Performance characteristics

## 📚 Detailed Documentation

### For Testing
- **[TESTING_GUIDE.md](TESTING_GUIDE.md)**
  - Testing workflow
  - Mock vs real LLM testing
  - How to evaluate results
  - Test sequence recommendations

- **[COMPLEX_TEST_CASES.md](COMPLEX_TEST_CASES.md)**
  - 12 detailed test cases
  - Full commands to run
  - Expected outputs
  - Suitability assessments

### For Reference
- **[TOKEN_RULES_README.md](TOKEN_RULES_README.md)**
  - Complete technical reference
  - Token types and semantics
  - Performance benchmarks
  - Integration guide
  - Limitations and workarounds

## 🛠️ Code Files

### Python Implementation
```
pkg/logs/processor/
├── translate_regex.py          # Standalone LLM-based translator
├── test_translate.py           # Mock translator (no LLM needed)
└── processing_rules.go         # Generated token rules (Go)

tasks/
└── logs_processor.py           # Invoke task integration
```

### Go Integration
```
pkg/logs/processor/
├── processing_rule_applicator.go   # Token rule matching logic
├── processing_rules.go             # Token rules definitions
└── processor.go                    # Main processor with tokenizer

pkg/logs/internal/decoder/auto_multiline_detection/
├── tokenizer.go                    # Tokenization engine
└── tokens/tokens.go                # Token types and definitions
```

## 🎯 Quick Navigation

### I want to...

**...understand what this system does**
→ Read [QUICK_REFERENCE.md](QUICK_REFERENCE.md) (1 page)

**...translate my first regex pattern**
→ Read [QUICKSTART.md](QUICKSTART.md) then run commands in devcontainer

**...see examples without installing anything**
→ Run `python3 pkg/logs/processor/test_translate.py`

**...understand complex patterns**
→ Read [COMPLEX_TEST_CASES.md](COMPLEX_TEST_CASES.md)

**...know if my pattern is suitable for tokens**
→ Check suitability matrix in [QUICK_REFERENCE.md](QUICK_REFERENCE.md)

**...understand how it works internally**
→ Read [IMPLEMENTATION_SUMMARY.md](IMPLEMENTATION_SUMMARY.md)

**...integrate this into my workflow**
→ Read [TESTING_GUIDE.md](TESTING_GUIDE.md)

**...learn all token types**
→ Read [TOKEN_RULES_README.md](TOKEN_RULES_README.md)

**...troubleshoot an issue**
→ Check troubleshooting sections in [QUICKSTART.md](QUICKSTART.md) or [QUICK_REFERENCE.md](QUICK_REFERENCE.md)

## 📊 Document Comparison

| Document | Length | Best For | Skill Level |
|----------|--------|----------|-------------|
| QUICK_REFERENCE | 1 page | Quick lookup | All levels |
| QUICKSTART | Short | First time users | Beginner |
| COMPLEX_TEST_CASES | Medium | Testing patterns | Intermediate |
| TESTING_GUIDE | Medium | Systematic testing | Intermediate |
| TOKEN_RULES_README | Long | Complete reference | Advanced |
| IMPLEMENTATION_SUMMARY | Long | Understanding system | Advanced |

## 🚀 Common Workflows

### Workflow 1: First Time User
```
1. Read QUICK_REFERENCE.md (5 min)
2. Read QUICKSTART.md (10 min)
3. Run test_translate.py (2 min)
4. Try simple pattern in devcontainer (5 min)
5. Review generated code (2 min)
```

### Workflow 2: Converting Existing Regex
```
1. Check QUICK_REFERENCE.md suitability matrix
2. If suitable: Run translation command
3. Review LLM output and warnings
4. Check COMPLEX_TEST_CASES.md for similar patterns
5. Add to processing_rules.go
6. Test with real logs
```

### Workflow 3: Learning the System
```
1. Read IMPLEMENTATION_SUMMARY.md (overview)
2. Read TOKEN_RULES_README.md (deep dive)
3. Read COMPLEX_TEST_CASES.md (examples)
4. Experiment with test_translate.py
5. Try increasingly complex patterns
```

### Workflow 4: Troubleshooting
```
1. Check error message
2. Look in QUICKSTART.md troubleshooting section
3. Check QUICK_REFERENCE.md common issues
4. Review pattern in COMPLEX_TEST_CASES.md
5. Read LLM warnings in output
```

## 🎓 Learning Path

### Level 1: Basics (30 minutes)
- [ ] Read QUICK_REFERENCE.md
- [ ] Read QUICKSTART.md
- [ ] Run test_translate.py
- [ ] Understand token types (D1-D10, C1-C10)
- [ ] Understand prefilter keywords

### Level 2: Usage (1 hour)
- [ ] Set up devcontainer environment
- [ ] Translate simple pattern (SSN)
- [ ] Translate medium pattern (UUID)
- [ ] Read TESTING_GUIDE.md
- [ ] Understand when tokens are suitable

### Level 3: Advanced (2-3 hours)
- [ ] Read TOKEN_RULES_README.md
- [ ] Read COMPLEX_TEST_CASES.md
- [ ] Try all 12 test cases
- [ ] Understand performance implications
- [ ] Learn integration points

### Level 4: Expert (4+ hours)
- [ ] Read IMPLEMENTATION_SUMMARY.md
- [ ] Study tokenizer.go implementation
- [ ] Study processing_rule_applicator.go
- [ ] Benchmark token rules vs regex
- [ ] Contribute new token types

## 📞 Need Help?

### Common Questions

**Q: Which document should I read first?**
A: Start with [QUICK_REFERENCE.md](QUICK_REFERENCE.md) - it's a one-page overview.

**Q: My pattern has alternation (|), what do I do?**
A: Check [COMPLEX_TEST_CASES.md](COMPLEX_TEST_CASES.md) test case #9. Tokens don't support alternation - keep as regex.

**Q: How do I know if my pattern is suitable?**
A: Check the suitability matrix in [QUICK_REFERENCE.md](QUICK_REFERENCE.md) or run the mock test.

**Q: The translation seems wrong, is the LLM broken?**
A: No - tokens have limitations. Check the LLM's notes/warnings in the output. Read [COMPLEX_TEST_CASES.md](COMPLEX_TEST_CASES.md) for context.

**Q: Can I see examples without installing the LLM?**
A: Yes! Run `python3 pkg/logs/processor/test_translate.py` - it shows 13 mock examples.

**Q: Where do I add the generated token rules?**
A: Add them to `pkg/logs/processor/processing_rules.go` in the `tokenRules` slice.

## 🔗 External Resources

- **Qwen2.5-Coder**: https://huggingface.co/Qwen/Qwen2.5-Coder-3B
- **Datadog Agent Docs**: https://docs.datadoghq.com/agent/
- **Log Processing Rules**: https://docs.datadoghq.com/agent/logs/advanced_log_collection/

## 📝 Quick Command Reference

```bash
# Mock test (no LLM, instant)
python3 pkg/logs/processor/test_translate.py

# Real translation (in devcontainer)
dda inv logs-processor.translate-regex-to-tokens \
  --regex-pattern='YOUR_PATTERN' \
  --rule-name='YOUR_RULE'

# Standalone script (in devcontainer)
python3 pkg/logs/processor/translate_regex.py \
  --regex 'YOUR_PATTERN' \
  --name 'YOUR_RULE'
```

## 🎯 TL;DR

**System**: Translates regex patterns → token sequences using AI
**Goal**: Faster log processing with prefilter optimization
**Best for**: Fixed-length patterns with distinctive keywords
**Not for**: Alternation, quantifiers, complex validation
**Start**: Read [QUICK_REFERENCE.md](QUICK_REFERENCE.md) → Run mock test → Try in devcontainer

---

**Ready to start?** → [QUICK_REFERENCE.md](QUICK_REFERENCE.md) ⭐

