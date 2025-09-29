# Log Pattern Extraction Architecture

## Overview
The feature extracts log pattern from a stream of logs while keeping the position and values of wildcards assoicated with those pattern.

Example:
```
Input Logs:
- "GET /api/users/123 200"
- "GET /api/users/456 200" 
- "GET /api/users/789 200"

Extracted Pattern (Could be converted to string):
- Pattern TokenList: [
    {Type: HttpMethod, Value: "GET", IsWildcard: false},
    {Type: Whitespace, Value: " ", IsWildcard: false},
    {Type: AbsolutePath, Value: "/*/*/*", IsWildcard: true},
    {Type: Whitespace, Value: " ", IsWildcard: false},
    {Type: HttpStatus, Value: "200", IsWildcard: false}
  ]
- Wildcard Values: ["123"], ["456"], ["789"]

Result: Instead of sending 3 full logs, send 1 pattern + 3 wildcard values
```

```
Input Logs:
- "ERROR Database connection to 192.168.1.100 failed"
- "ERROR Database connection to 192.168.1.101 failed"
- "ERROR Database connection to 192.168.1.102 failed"

Extracted Pattern (Could be converted to string):
- Pattern TokenList: [
    {Type: SeverityLevel, Value: "ERROR", IsWildcard: false},
    {Type: Whitespace, Value: " ", IsWildcard: false},
    {Type: Word, Value: "Database", IsWildcard: false},
    {Type: Word, Value: "connection", IsWildcard: false},
    {Type: Word, Value: "to", IsWildcard: false},
    {Type: Whitespace, Value: " ", IsWildcard: false},
    {Type: IPv4, Value: "*", IsWildcard: true},
    {Type: Whitespace, Value: " ", IsWildcard: false},
    {Type: Word, Value: "failed", IsWildcard: false}
  ]
- Wildcard Values: ["192.168.1.100"], ["192.168.1.101"], ["192.168.1.102"]

Result: Instead of sending 3 full logs, send 1 pattern + 3 wildcard values
```


## Architecture Components
## Data Flow

```
Log Message → Tokenizer → TokenList → Signature → ClusterManager → Cluster → Pattern → Stream
```

### 1. Token Package (`pkg/logs/patterns/token/`)
- **Token**: Represents a single word of a log message after it has been identified and classified.
    - Each Token holds the actual text value.
    - It records what type of data it is (such as HttpMethod, IPv4, Email, etc.).
    - It also marks whether this part of the log should be treated as a wildcard
- **TokenList**: Container for tokens representing a complete log message
  - Acts as a safe transition package between tokenization and clustering
  - Provides basic token management (add, access, length) for other packages to process
- **Signature**: Creates a unique identifier for log patterns
  - Generates a hash-based identifier from token types and token positions

### 2. Automaton Package (`pkg/logs/patterns/automaton/`)
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

### 3. Clustering Package (`pkg/logs/patterns/clustering/`)
- **ClusterManager**: Manages hash buckets and statistics for clustering
  - Uses signature hashes to route TokenLists to the right bucket
  - Tracks statistics (total clusters, TokenLists, average sizes)
  - Provides lookup and management functions for clusters for debugging
- **Cluster**: Manages a single group of similar TokenLists
  - Contains TokenLists that have the same signature
  - Differentitate the TokenLists 
  - Generates wildcard patterns showing the common structure
  - Identifies wildcards position
  - Assigns unique PatternIDs for streaming

## Current State
- ✅ Pattern extraction system
- ✅ Wildcard value extraction
- ❌ Memory management (infinite growth)
- ❌ Pattern is not connected to processor
- ❌ Processor is not connected to the GRPC stream 
  - Include sending 1st state of the pattern (and update it as it goes)
  - Resending ALL states of all patterns when the connection is reset (aka resending the hash bucket)
- ❌ Need to figure out a way to efficiently send a pattern ID along with wildcards ?

## Next Step
1. On delivery of the logs
- Build a new stream strategy to deliver logs to the batcher
- Build a new batcher than can ship the logs to a blackhole/gRPC stream

2. On eviction strategy for the tokenlists
- Before Patternize for a sample of logs:
    - Store all TokenList objects in clusters
    - Memory grows with each new log

- After Patternize:
    - Keep: Pattern string (for a limited time?), pattern ID (both intake and agent keep a record of it?), wildcard positions
    - Keep: Wildcard values for new logs
    - Remove: All original TokenList objects
    - Remove: All raw log messages associated with the pattern
