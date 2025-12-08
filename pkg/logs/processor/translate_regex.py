#!/usr/bin/env python3
r"""
Translate regex-based processing rules to token-based rules using Qwen2.5-Coder-3B.

This script uses the Qwen2.5-Coder-3B SLM to intelligently convert regex patterns
to efficient token-based patterns for the Datadog Agent logs processor.

Usage:
    python translate_regex.py --regex '\d{3}-\d{2}-\d{4}' --name 'ssn' --replacement '[SSN_REDACTED]'
    
Requirements:
    pip install transformers torch accelerate
"""

import argparse
import json
import re
import sys


TOKEN_REFERENCE = """
Available Token Types in pkg/logs/internal/decoder/auto_multiline_detection/tokens:

Digit Runs:
- D1: 1 digit, D2: 2 digits, D3: 3 digits, ..., D10: 10 or more digits

Character Runs:
- C1: 1 char, C2: 2 chars, C3: 3 chars, ..., C10: 10 or more chars

Special Characters:
- Dash: '-', Period: '.', Colon: ':', Underscore: '_'
- Fslash: '/', Bslash: '\\', Comma: ','
- At: '@', Space: ' ', Plus: '+', Equal: '='
- Parenopen: '(', Parenclose: ')'
- Bracketopen: '[', Bracketclose: ']'
- Braceopen: '{', Braceclose: '}'
- And more (see tokens/tokens.go)

Special Tokens:
- Month: JAN, FEB, MAR, APR, MAY, JUN, JUL, AUG, SEP, OCT, NOV, DEC
- Day: MON, TUE, WED, THU, FRI, SAT, SUN
- Zone: UTC, GMT, EST, EDT, PST, PDT, etc.
- T: Time separator 'T'
- Apm: AM or PM

Example Conversions:

1. SSN Format:
   Regex: \\d{3}-\\d{2}-\\d{4}
   Tokens: [D3, Dash, D2, Dash, D4]
   Prefilter: ["-"]
   Explanation: Matches 123-45-6789

2. Date YYYY-MM-DD:
   Regex: \\d{4}-\\d{2}-\\d{2}
   Tokens: [D4, Dash, D2, Dash, D2]
   Prefilter: ["-"]
   Explanation: Matches 2024-12-05

3. IPv4 Address:
   Regex: \\d{1,3}\\.\\d{1,3}\\.\\d{1,3}\\.\\d{1,3}
   Tokens: [D1, Period, D1, Period, D1, Period, D1] (minimum case)
   Prefilter: ["."]
   Note: This matches 1.2.3.4 but also matches longer digit runs

4. Email Pattern:
   Regex: [A-Za-z]+@[A-Za-z]+\\.[A-Za-z]+
   Tokens: [C1, At, C1, Period, C1]
   Prefilter: ["@", "."]
   Explanation: Matches a@b.c minimum

5. Credit Card (16 digits):
   Regex: \\d{16}
   Tokens: [D10, D6] (10 digits + 6 digits)
   Prefilter: []
   Note: Token runs are capped at D10, so 16 digits = D10 + D6

Important Limitations:
- Token patterns are EXACT sequences, no quantifiers like {1,3}, +, *, ?
- For variable-length patterns, choose most restrictive or common case
- Complex alternations (a|b) require separate rules
- Prefilters improve performance but must be present in ALL matches
"""


def create_prompt(regex_pattern, description=""):
    """Create a prompt for the model to translate regex to tokens"""
    
    prompt = f"""You are a regex to token pattern translator for the Datadog Agent logs processor.

{TOKEN_REFERENCE}

Convert this regex pattern to a token sequence:

Regex Pattern: {regex_pattern}
Description: {description}

Provide the conversion as valid JSON with these exact fields:
{{
  "tokens": ["D3", "Dash", "D2", "Dash", "D4"],
  "prefilter_keywords": ["-"],
  "explanation": "Matches SSN format 123-45-6789",
  "notes": "Any caveats or limitations"
}}

Critical Requirements:
1. Only use token names from the list above (D1-D10, C1-C10, Dash, Period, etc.)
2. Token patterns must be exact sequences - no regex quantifiers
3. For variable-length regex, pick the minimum or most common case
4. Prefilter keywords must be literal strings present in ALL matches
5. If pattern is too complex for tokens, explain in "notes"

Output ONLY valid JSON, no additional text:"""
    
    return prompt


def translate_regex(regex_pattern, description=""):
    """Translate a regex pattern to tokens using Qwen2.5-Coder-3B"""
    
    try:
        from transformers import AutoModelForCausalLM, AutoTokenizer
        import torch
    except ImportError:
        print("Error: transformers library not installed", file=sys.stderr)
        print("Install with: pip install transformers torch accelerate", file=sys.stderr)
        sys.exit(1)
    
    print("Loading Qwen2.5-Coder-3B model...", file=sys.stderr)
    model_name = "Qwen/Qwen2.5-Coder-3B"
    
    try:
        tokenizer = AutoTokenizer.from_pretrained(model_name)
        model = AutoModelForCausalLM.from_pretrained(
            model_name,
            torch_dtype="auto",
            device_map="auto"
        )
    except Exception as e:
        print(f"Error loading model: {e}", file=sys.stderr)
        sys.exit(1)
    
    prompt = create_prompt(regex_pattern, description)
    
    messages = [
        {"role": "system", "content": "You are a helpful coding assistant that translates regex patterns to token sequences. Output only valid JSON."},
        {"role": "user", "content": prompt}
    ]
    
    text = tokenizer.apply_chat_template(
        messages,
        tokenize=False,
        add_generation_prompt=True
    )
    
    model_inputs = tokenizer([text], return_tensors="pt").to(model.device)
    
    print(f"Translating regex: {regex_pattern}", file=sys.stderr)
    generated_ids = model.generate(
        **model_inputs,
        max_new_tokens=512,
        temperature=0.3,
        top_p=0.9,
    )
    
    generated_ids = [
        output_ids[len(input_ids):] for input_ids, output_ids in zip(model_inputs.input_ids, generated_ids)
    ]
    
    response = tokenizer.batch_decode(generated_ids, skip_special_tokens=True)[0]
    
    # Extract JSON from response
    try:
        # Try to find JSON block in response
        json_match = re.search(r'\{[^{}]*(?:\{[^{}]*\}[^{}]*)*\}', response, re.DOTALL)
        if json_match:
            result = json.loads(json_match.group())
            return result
        else:
            print(f"Warning: Could not parse JSON from response", file=sys.stderr)
            print(f"Raw response: {response}", file=sys.stderr)
            return {"error": "Could not parse JSON", "raw": response}
    except json.JSONDecodeError as e:
        print(f"JSON decode error: {e}", file=sys.stderr)
        print(f"Raw response: {response}", file=sys.stderr)
        return {"error": str(e), "raw": response}


def generate_go_code(translation, rule_name, replacement):
    """Generate Go code for the token rule"""
    
    if "error" in translation:
        return f"// Error translating pattern: {translation['error']}"
    
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
    """Main entry point"""
    parser = argparse.ArgumentParser(
        description='Translate regex patterns to token-based rules using Qwen2.5-Coder-3B',
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
  # Translate SSN pattern
  python translate_regex.py --regex '\\d{3}-\\d{2}-\\d{4}' --name 'ssn' --replacement '[SSN_REDACTED]'
  
  # Translate credit card pattern
  python translate_regex.py --regex '\\d{4}[\\s-]?\\d{4}[\\s-]?\\d{4}[\\s-]?\\d{4}' --name 'cc' --replacement '[CC_REDACTED]'
  
  # With description
  python translate_regex.py --regex '\\d{4}-\\d{2}-\\d{2}' --name 'date' --description 'ISO date format'
"""
    )
    
    parser.add_argument('--regex', required=True, help='Regex pattern to translate')
    parser.add_argument('--name', required=True, help='Rule name')
    parser.add_argument('--replacement', default='[REDACTED]', help='Replacement text')
    parser.add_argument('--description', default='', help='Pattern description')
    parser.add_argument('--json-only', action='store_true', help='Output only JSON result')
    
    args = parser.parse_args()
    
    result = translate_regex(args.regex, args.description)
    
    if args.json_only:
        print(json.dumps(result, indent=2))
    else:
        print("\n" + "="*60, file=sys.stderr)
        print("Translation Result:", file=sys.stderr)
        print("="*60, file=sys.stderr)
        print(json.dumps(result, indent=2))
        
        print("\n" + "="*60, file=sys.stderr)
        print("Generated Go Code:", file=sys.stderr)
        print("="*60, file=sys.stderr)
        go_code = generate_go_code(result, args.name, args.replacement)
        print(go_code)
        print("="*60, file=sys.stderr)
        
        if "notes" in result and result["notes"]:
            print(f"\n⚠️  Note: {result['notes']}", file=sys.stderr)


if __name__ == "__main__":
    main()

