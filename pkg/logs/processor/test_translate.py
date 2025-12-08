#!/usr/bin/env python3
"""
Test script for the regex to token translator.

This script demonstrates the translation without requiring the full model download.
For actual translations, use translate_regex.py with the Qwen2.5-Coder-3B model.
"""

import sys


def mock_translate_ssn():
    """Mock translation result for SSN pattern"""
    return {
        "tokens": ["D3", "Dash", "D2", "Dash", "D4"],
        "prefilter_keywords": ["-"],
        "explanation": "Matches SSN format XXX-XX-XXXX (e.g., 123-45-6789)",
        "notes": "Fixed-length pattern, ideal for token matching"
    }


def mock_translate_ipv4():
    """Mock translation result for IPv4 pattern"""
    return {
        "tokens": ["D1", "Period", "D1", "Period", "D1", "Period", "D1"],
        "prefilter_keywords": ["."],
        "explanation": "Matches IPv4 address (minimum case, 1-3 digits per octet)",
        "notes": "Uses D1 which matches 1-10+ digits. May overmatch but catches all valid IPs."
    }


def mock_translate_date():
    """Mock translation result for ISO date pattern"""
    return {
        "tokens": ["D4", "Dash", "D2", "Dash", "D2"],
        "prefilter_keywords": ["-"],
        "explanation": "Matches ISO date format YYYY-MM-DD (e.g., 2024-12-05)",
        "notes": "Fixed-length pattern, excellent for token matching"
    }


def mock_translate_api_key():
    """Mock translation result for hex API key"""
    return {
        "tokens": ["C32"],
        "prefilter_keywords": [],
        "explanation": "Matches 32-character hex string (e.g., API keys, tokens)",
        "notes": "C32 matches any 32 characters, not just hex. May need additional validation."
    }


def mock_translate_credit_card():
    """Mock translation result for credit card with spaces/dashes"""
    return {
        "tokens": ["D4", "Space", "D4", "Space", "D4", "Space", "D4"],
        "prefilter_keywords": [],
        "explanation": "Matches credit card with spaces: XXXX XXXX XXXX XXXX",
        "notes": "This pattern only matches space-separated format. Need multiple rules for dash-separated or no separator."
    }


def mock_translate_email():
    """Mock translation result for simple email pattern"""
    return {
        "tokens": ["C1", "At", "C1", "Period", "C1"],
        "prefilter_keywords": ["@", "."],
        "explanation": "Matches basic email: user@domain.com (minimum length)",
        "notes": "Very simplified pattern. May produce false positives. C1 matches 1-10+ chars."
    }


def mock_translate_jwt():
    """Mock translation result for JWT token"""
    return {
        "tokens": ["C1", "Period", "C1", "Period", "C1"],
        "prefilter_keywords": ["."],
        "explanation": "Matches JWT structure: header.payload.signature",
        "notes": "Cannot verify base64 encoding with tokens. May match other dot-separated strings."
    }


def mock_translate_aws_access_key():
    """Mock translation result for AWS access key ID"""
    return {
        "tokens": ["NewLiteralToken('AKIA')", "C16"],
        "prefilter_keywords": ["AKIA"],
        "explanation": "Matches AWS access key ID starting with AKIA followed by 16 chars",
        "notes": "Uses literal token for exact prefix matching. Good candidate for token matching."
    }


def mock_translate_uuid():
    """Mock translation result for UUID"""
    return {
        "tokens": ["C8", "Dash", "C4", "Dash", "C4", "Dash", "C4", "Dash", "C12"],
        "prefilter_keywords": ["-"],
        "explanation": "Matches UUID format: xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx",
        "notes": "Fixed structure, ideal for tokens. Matches hex UUIDs and other similar formats."
    }


def mock_translate_timestamp():
    """Mock translation result for ISO8601 timestamp"""
    return {
        "tokens": ["D4", "Dash", "D2", "Dash", "D2", "T", "D2", "Colon", "D2", "Colon", "D2"],
        "prefilter_keywords": ["T"],
        "explanation": "Matches ISO8601 timestamp: YYYY-MM-DDTHH:MM:SS",
        "notes": "Uses special T token. Doesn't match timezone suffix - would need separate rule."
    }


def mock_translate_phone():
    """Mock translation result for US phone number"""
    return {
        "tokens": ["Parenopen", "D3", "Parenclose", "Space", "D3", "Dash", "D4"],
        "prefilter_keywords": ["(", ")"],
        "explanation": "Matches US phone: (123) 456-7890",
        "notes": "Specific format only. Other formats would need separate rules."
    }


def mock_translate_log_level_complex():
    """Mock translation result for complex log level pattern"""
    return {
        "tokens": ["LBracket", "NewLiteralToken('ERROR')", "RBracket"],
        "prefilter_keywords": ["[ERROR]"],
        "explanation": "Matches [ERROR] log level prefix",
        "notes": "Uses literal token for exact matching. Very efficient with prefilter."
    }


def mock_translate_too_complex():
    """Mock translation for a pattern that's too complex for tokens"""
    return {
        "tokens": None,
        "prefilter_keywords": [],
        "explanation": "This pattern uses alternation and quantifiers that cannot be expressed with tokens",
        "notes": "LIMITATION: Token patterns don't support regex features like |, *, +, ?, {n,m}. Recommend keeping as regex."
    }


def generate_go_code(translation, rule_name, replacement):
    """Generate Go code for the token rule"""
    
    tokens_list = ",\n\t\t".join([f"tokens.{t}" for t in translation.get("tokens", [])])
    
    go_code = f'''\t{{
\t\tName: "{rule_name}",
\t\tType: RuleTypeToken,
\t\tTokenPattern: []tokens.Token{{
\t\t\t{tokens_list},
\t\t}},
\t\tReplacement: []byte("{replacement}"),'''
    
    if "prefilter_keywords" in translation and translation["prefilter_keywords"]:
        keywords = ",\n\t\t\t".join([f'[]byte("{k}")' for k in translation["prefilter_keywords"]])
        go_code += f'''
\t\tPrefilterKeywords: [][]byte{{
\t\t\t{keywords},
\t\t}},'''
    
    explanation = translation.get("explanation", "")
    notes = translation.get("notes", "")
    comment = explanation
    if notes:
        comment += f" - Note: {notes}"
    
    go_code += f'''
\t}}, // {comment}'''
    
    return go_code


def main():
    """Run mock translations"""
    print("="*80)
    print("Mock Regex to Token Translation Examples - Complex Patterns")
    print("="*80)
    print()
    print("This script demonstrates expected outputs without requiring the")
    print("Qwen2.5-Coder-3B model. For actual translations, use:")
    print("  python translate_regex.py --regex <pattern> --name <name>")
    print()
    
    examples = [
        # Basic patterns
        ("\\d{3}-\\d{2}-\\d{4}", "auto_redact_ssn", "[SSN_REDACTED]", mock_translate_ssn, "SIMPLE"),
        ("\\d{4}-\\d{2}-\\d{2}", "detect_date", "[DATE_REDACTED]", mock_translate_date, "SIMPLE"),
        
        # Medium complexity
        ("\\d{1,3}\\.\\d{1,3}\\.\\d{1,3}\\.\\d{1,3}", "detect_ipv4", "[IP_REDACTED]", mock_translate_ipv4, "MEDIUM"),
        ("[0-9a-fA-F]{32}", "auto_redact_api_key", "[API_KEY_REDACTED]", mock_translate_api_key, "MEDIUM"),
        ("\\d{4}[\\s-]?\\d{4}[\\s-]?\\d{4}[\\s-]?\\d{4}", "auto_redact_cc", "[CC_REDACTED]", mock_translate_credit_card, "MEDIUM"),
        
        # Complex patterns
        ("[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\\.[A-Z|a-z]{2,}", "auto_redact_email", "[EMAIL_REDACTED]", mock_translate_email, "COMPLEX"),
        ("[A-Za-z0-9_-]+\\.[A-Za-z0-9_-]+\\.[A-Za-z0-9_-]+", "auto_redact_jwt", "[JWT_REDACTED]", mock_translate_jwt, "COMPLEX"),
        ("AKIA[0-9A-Z]{16}", "auto_redact_aws_key", "[AWS_KEY_REDACTED]", mock_translate_aws_access_key, "COMPLEX"),
        
        # Very complex patterns
        ("[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}", "detect_uuid", "[UUID_REDACTED]", mock_translate_uuid, "COMPLEX"),
        ("\\d{4}-\\d{2}-\\d{2}T\\d{2}:\\d{2}:\\d{2}", "detect_timestamp", "[TS_REDACTED]", mock_translate_timestamp, "COMPLEX"),
        ("\\(\\d{3}\\)\\s\\d{3}-\\d{4}", "auto_redact_phone", "[PHONE_REDACTED]", mock_translate_phone, "COMPLEX"),
        ("\\[ERROR\\]", "detect_error_level", "", mock_translate_log_level_complex, "COMPLEX"),
        
        # Too complex for tokens
        ("(ERROR|WARN|INFO|DEBUG)", "detect_log_level_any", "", mock_translate_too_complex, "TOO_COMPLEX"),
    ]
    
    current_category = None
    for regex, name, replacement, translator, category in examples:
        if current_category != category:
            current_category = category
            print("\n" + "="*80)
            if category == "SIMPLE":
                print("SIMPLE PATTERNS - Ideal for token matching")
            elif category == "MEDIUM":
                print("MEDIUM COMPLEXITY - Good candidates with some limitations")
            elif category == "COMPLEX":
                print("COMPLEX PATTERNS - Possible but may need multiple rules")
            elif category == "TOO_COMPLEX":
                print("TOO COMPLEX - Keep as regex, tokens cannot express these")
            print("="*80)
        
        print()
        print("-"*80)
        print(f"Regex: {regex}")
        print(f"Rule: {name}")
        print("-"*80)
        
        result = translator()
        
        if result.get("tokens") is None:
            print(f"❌ NOT SUITABLE FOR TOKENS")
            print(f"Explanation: {result['explanation']}")
            print(f"Notes: {result['notes']}")
        else:
            print(f"Tokens: {result['tokens']}")
            print(f"Prefilter: {result['prefilter_keywords']}")
            print(f"Explanation: {result['explanation']}")
            if result.get("notes"):
                print(f"Notes: {result['notes']}")
            
            print()
            print("Generated Go Code:")
            print(generate_go_code(result, name, replacement) if replacement else f"// Detection only: {name}")
    
    print()
    print("="*80)
    print("Key Takeaways:")
    print("="*80)
    print()
    print("✅ GOOD FOR TOKENS:")
    print("  - Fixed-length patterns (SSN, dates, UUIDs)")
    print("  - Patterns with distinctive prefilter keywords")
    print("  - Known prefixes/suffixes (AWS keys, log levels)")
    print()
    print("⚠️  LIMITED SUPPORT:")
    print("  - Variable-length patterns (may overmatch)")
    print("  - Character class restrictions ([a-zA-Z0-9] becomes C1)")
    print("  - Optional elements (need multiple rules)")
    print()
    print("❌ NOT SUITABLE:")
    print("  - Alternation (|)")
    print("  - Repetition quantifiers (*, +, ?)")
    print("  - Lookaheads/lookbehinds")
    print("  - Backreferences")
    print()
    print("="*80)
    print("To run actual translations with Qwen2.5-Coder-3B:")
    print("  1. Install: pip install transformers torch accelerate")
    print("  2. Run: python translate_regex.py --regex '<pattern>' --name '<name>'")
    print("  3. Or: dda inv logs-processor.translate-regex-to-tokens \\")
    print("           --regex-pattern='<pattern>' --rule-name='<name>'")
    print("="*80)


if __name__ == "__main__":
    main()

