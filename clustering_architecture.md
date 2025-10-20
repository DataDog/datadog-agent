# Log Pattern Clustering Architecture

## Main Data Flow Pipeline

```mermaid
flowchart TD
    A[Raw Log Messages<br/>GET /api/users 200<br/>POST /api/users 201] --> B[Tokenization]

    B --> C["Token Classification<br/>automaton.TokenizeString()"]
    C --> D["Token List Creation<br/>token.NewTokenList()"]

    D --> E["Signature Generation<br/>.Signature()"]
    E --> F["Hash Computation<br/>computeHash()"]

    F --> G["Cluster Manager<br/>clustering.Add()"]
    G --> H{Hash Bucket<br/>Lookup}

    H -->|Existing Cluster| I["Add to Cluster<br/>cluster.Add()"]
    H -->|New Signature| J["Create New Cluster<br/>NewCluster()"]

    I --> K["Pattern Generation<br/>cluster.GeneratePattern()"]
    J --> K

    K --> L[Wildcard Patterns<br/>* /api/users *<br/>ERROR * failed]

    style A fill:#e1f5fe
    style L fill:#c8e6c9
    style G fill:#fff3e0
```

## Core Function Call Graph

```mermaid
graph TD
    A[automaton.TokenizeString] --> B[NewTokenizer]
    A --> C[Tokenizer.Tokenize]

    C --> D[processNextToken]
    C --> E[consumeWhitespace]
    C --> F[extractWord]
    C --> G[classifyToken]

    G --> H[globalTrie.Match]
    G --> I[GetTerminalRules]

    A --> J[token.NewTokenList]
    J --> K[TokenList.Signature]

    K --> L[PositionSignature]
    K --> M[CountSignature]
    K --> N[computeHash]

    O[clustering.NewClusterManager] --> P[ClusterManager.Add]
    P --> Q[hashBuckets lookup]
    P --> R[cluster.Signature.Equals]
    P --> S[NewCluster]

    S --> T[Cluster.Add]
    T --> U[Cluster.GeneratePattern]

    style A fill:#ffecb3
    style P fill:#f3e5f5
    style U fill:#e8f5e8
```

## Hash Bucket Architecture

```mermaid
graph TB
    A[ClusterManager] --> B["hashBuckets: map[uint64][]*Cluster"]

    B --> C["Hash: 12345"]
    B --> D["Hash: 67890"]

    C --> E["Cluster1<br/>HTTP Requests"]
    C --> F["Cluster2<br/>Hash Collision"]

    E --> G["TokenLists:<br/>GET /api 200<br/>POST /api 201<br/>PUT /api 200"]
    E --> H["Pattern: * /api *<br/>Wildcards: positions 0, 4"]

    D --> I["Cluster3<br/>Error Messages"]
    I --> J["TokenLists:<br/>ERROR DB failed<br/>ERROR Auth failed"]
    I --> K["Pattern: ERROR * failed<br/>Wildcards: position 2"]

    style A fill:#f9f,stroke:#333,stroke-width:2px
    style E fill:#bbf,stroke:#333,stroke-width:2px
    style I fill:#fbb,stroke:#333,stroke-width:2px
```

## Memory Layout and Data Structure

```mermaid
classDiagram
    class ClusterManager {
        +map~uint64~[]Cluster hashBuckets
        +int totalTokenLists
        +int totalClusters
        +Add(tokenList) Cluster
        +GetCluster(signature) Cluster
    }

    class Cluster {
        +Signature signature
        +[]TokenList tokenLists
        +TokenList pattern
        +map~int~bool wildcardMap
        +Add(tokenList) bool
        +GeneratePattern() TokenList
    }

    class TokenList {
        +[]Token tokens
        +Signature() Signature
        +PositionSignature() string
        +CountSignature() string
    }

    class Token {
        +string Value
        +TokenType Type
        +bool IsWildcard
    }

    ClusterManager --> Cluster : contains
    Cluster --> TokenList : groups
    TokenList --> Token : contains
```

## Performance Characteristics

### Algorithm Complexity by Operation

```mermaid
graph LR
    subgraph "Tokenization Pipeline"
        A["Raw Log<br/>O(n) time<br/>O(k) space"] --> B["Token Classification<br/>O(1) per token<br/>Trie + Rules"]
        B --> C["TokenList<br/>O(k) creation<br/>O(k) memory"]
    end

    subgraph "Clustering Pipeline"
        C --> D["Signature Generation<br/>O(k) time<br/>O(1) space"]
        D --> E["Hash Lookup<br/>O(1) avg<br/>O(m) worst"]
        E --> F["Cluster Assignment<br/>O(1) insertion<br/>O(1) space"]
    end

    subgraph "Pattern Pipeline"
        F --> G["Pattern Generation<br/>O(k × c) time<br/>O(k) space"]
        G --> H["Wildcard Detection<br/>O(k × c) comparison<br/>Lazy evaluation"]
    end

    style A fill:#ffecb3
    style E fill:#f3e5f5
    style G fill:#e8f5e8
```

### Performance Analysis

```mermaid
graph TB
    subgraph "Performance Characteristics"
        A["🚀 Tokenization<br/>O(n) always<br/>Single-pass processing"]
        B["📊 Signature<br/>O(k) linear<br/>Cached result"]
        C["🔍 Hash Lookup<br/>O(1) avg, O(m) worst<br/>Rare collisions"]
        D["🎯 Clustering<br/>O(1) typical<br/>Hit existing clusters"]
        E["🎨 Pattern Gen<br/>O(k) single, O(k×c) multiple<br/>Lazy evaluation"]
    end

    A --> B
    B --> C
    C --> D
    D --> E

    style A fill:#e3f2fd
    style B fill:#f3e5f5
    style C fill:#fff3e0
    style D fill:#e8f5e8
    style E fill:#fce4ec
```

### Test Results from Codebase

From the actual test suite (`TestClusteringPerformance`):
- **Input**: 400 similar log messages
- **Output**: 3 clusters created
- **Demonstrates**: Effective pattern consolidation for similar structured logs

### Algorithm Variables

```mermaid
graph LR
    subgraph "Input Variables"
        A["n: String Length<br/>Character count<br/>Linear tokenization cost"]
        B["k: Tokens per Message<br/>After tokenization<br/>Affects signature generation"]
    end

    subgraph "System Variables"
        C["m: Clusters per Bucket<br/>Hash collisions<br/>Usually 1 cluster"]
        D["c: Messages per Cluster<br/>Pattern generation cost<br/>Compression vs speed trade-off"]
    end

    style A fill:#e3f2fd
    style B fill:#e3f2fd
    style C fill:#fff3e0
    style D fill:#fff3e0
```

### Key Optimizations

```mermaid
graph TB
    subgraph "Memory Optimizations"
        A["String Interning<br/>Common tokens cached<br/>GET, POST, ERROR reused"]
        B["Lazy Evaluation<br/>Patterns generated on-demand<br/>Reduces memory footprint"]
    end

    subgraph "CPU Optimizations"
        C["Hash Pre-computation<br/>Signatures include cached hash<br/>Avoids repeated calculations"]
        D["Trie Lookup<br/>O(1) for HTTP methods<br/>O(1) for severity levels"]
    end

    subgraph "Reliability Features"
        E["Collision Handling<br/>Graceful hash collision recovery<br/>Exact signature fallback"]
        F["Input Validation<br/>UTF-8 safety checks<br/>Defensive programming"]
    end

    style A fill:#e8f5e8
    style B fill:#e8f5e8
    style C fill:#fff3e0
    style D fill:#fff3e0
    style E fill:#fce4ec
    style F fill:#fce4ec
```

## Production Data Flow Example

```mermaid
sequenceDiagram
    participant L as Log Message
    participant T as Tokenizer
    participant TL as TokenList
    participant CM as ClusterManager
    participant C as Cluster

    L->>T: "GET /api/users 200"
    T->>T: TokenizeString()
    T->>TL: [HttpMethod(GET), Whitespace( ), AbsolutePath(/api/users), ...]
    TL->>TL: Generate Signature()
    TL->>TL: "HttpMethod,Whitespace,AbsolutePath,Whitespace,HttpStatus"
    TL->>TL: Hash: 0x1a2b3c4d

    TL->>CM: ClusterManager.Add(tokenList)
    CM->>CM: hashBuckets[0x1a2b3c4d] lookup
    CM->>C: Found existing cluster
    C->>C: cluster.Add(tokenList)
    C->>C: GeneratePattern()
    C-->>CM: Pattern: "* /api/users *"
    CM-->>L: Clustered successfully

    Note over C: Wildcards at positions [0, 4]<br/>for HTTP method and status code
```

## Key Production Functions

### Core Pipeline
- `automaton.TokenizeString()` - Entry point
- `ClusterManager.Add()` - Main clustering logic
- `Cluster.GeneratePattern()` - Pattern extraction
- `TokenList.Signature()` - Clustering key generation

### Support Functions
- `NewClusterManager()` - Initialization
- `NewCluster()` - Cluster creation
- `Cluster.Add()` - Add TokenList to existing cluster
- `ClusterManager.GetCluster()` - Retrieve by signature

### Infrastructure
- `globalTrie.Match()` - Fast token classification
- `Signature.Equals()` - Hash collision resolution
- `computeHash()` - Signature hashing for buckets