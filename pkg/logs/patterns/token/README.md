## Token System
- **Token**: Represents a single word of a log message after it has been identified and classified.
    - Each Token holds the actual text value.
    - It records what type of data it is (such as HttpMethod, IPv4, Email, etc.).
    - It also marks whether this part of the log should be treated as a wildcard
- **TokenList**: Container for tokens representing a complete log message
  - Acts as a safe transition package between tokenization and clustering
  - Provides basic token management (add, access, length) for other packages to process
- **Signature**: Creates a unique identifier for log patterns
  - Generates a hash-based identifier from token types and token positions
    - **Position-based signature**: Captures the structural pattern of token types
    - **Count-based signature**: Captures the frequency of each token type

## Design

The token system is designed to:
1. **Normalize** log messages into structured token sequences
2. **Enable pattern matching** through token type comparison
3. **Support clustering** via signature-based grouping
4. **Allow merging** of similar patterns through mergeability rules
    - The feature currently needs a remediation to avoid overclustering. eg:
    ```
    "login failed" and "logout succeeded" would incorrectly merge to "* *" 
    ```

