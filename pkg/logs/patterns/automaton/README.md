# Automaton Engine

The automaton engine provides log tokenization using a trie-based state machine with pattern matching rules.

## Components
- **Tokenizer**: Streams log messages character by character and classifies tokens
  - Scans through log text character by character
  - Groups characters into meaningful tokens (words, numbers, etc..)
  - Classifies each token by type (IP address, date, URL, HTTP method, etc.)
- **Trie**: Prebuilt data structure for efficient token classification
  - Contains pre-defined patterns like "GET", "POST", "ERROR" for fast lookup
  - Helps tokenizer quickly identify token types during character-by-character scanning
- **Rules**: Regex patterns for token classification
  - Handles tokens that don't have exact matches in the Trie
  - Uses regex patterns to classify complex tokens (IP addresses, emails, URLs, dates,etc...)

## How It Works

```
Raw Log → Tokenizer → Token Sequence
  ↓           ↓
  |      1. Trie Lookup (exact match)
  |           ↓
  |      2. Rules (if no match)
  |           ↓
  └──────→ Classified Tokens
```

### Tokenization Flow

1. **Character scanning**: Process input character by character
2. **State transitions**: Move between states (word, number, whitespace, etc.)
3. **Token creation**: Generate initial tokens with basic types
4. **Classification**: For each token:
   - First, check **trie** for exact pattern match (e.g., "GET" → HttpMethod)
   - If no match, apply **terminal rules** (regex) for classification (e.g., "192.168.1.1" → IPv4)
5. **Result**: Fully classified tokens with metadata

### Example

```go
// Initialize tokenizer with predefined rules
tokenizer := automaton.NewTokenizer()

// Tokenize a log message
input := "GET /api/users 200"
tokens := tokenizer.Tokenize(input)

// Result:
// [HttpMethod(GET), Whitespace( ), AbsolutePath(/api/users), Whitespace( ), HttpStatus(200)]
```

## Rule Categories

- **HTTP**: Methods (GET, POST), status codes (200, 404)
- **Network**: IPv4, IPv6, email addresses, URLs
- **DateTime**: ISO timestamps, common date formats
- **Severity**: Log levels (ERROR, WARN, INFO)
- **Delimiters**: Brackets, quotes, punctuation

## Design Goals

1. **Performance**: Trie-based matching is O(k) for pattern length k (with upfront memory cost for building the tree)
2. **Accuracy**: Regex rules provide precise classification (e.g., "2024-01-01" matches date pattern)
3. **Extensibility**: Easy to add new rules and categories
4. **Context-aware**: `PossiblyWildcard` flag guides pattern generation (In Progress)
