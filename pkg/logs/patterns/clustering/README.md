# Clustering System

The clustering system groups similar log messages and generates patterns by identifying wildcard positions.

## Components

- **ClusterManager**: Manages hash buckets and statistics for clustering
  - Uses signature hashes to route TokenLists to the right bucket
  - Tracks statistics (total clusters, TokenLists, average sizes)
  - Provides lookup and management functions for clusters for debugging
  
- **Cluster**: Manages a single group of similar TokenLists
  - Contains TokenLists that have the same signature
  - Differentitate the TokenLists 
  - Generates wildcard patterns showing the common structure
  - Identifies wildcard positions
  - Assigns unique PatternIDs for streaming

## How It Works

### Step-by-Step Example

```
Log 1: "user123 logged in"
  → TokenList[Word(user123), Word(logged), Word(in)]
  → Signature{Hash: 0xABC, Position: "Word|Word|Word"}
  → hashBuckets[0xABC] = [Cluster1]
  → Cluster1.TokenLists = [TokenList1]

Log 2: "user456 logged in"
  → TokenList[Word(user456), Word(logged), Word(in)]
  → Signature{Hash: 0xABC, Position: "Word|Word|Word"}  (same signature!)
  → hashBuckets[0xABC] → found Cluster1
  → Signature matches → Add to existing Cluster1
  → Cluster1.TokenLists = [TokenList1, TokenList2]

Cluster1:
  TokenLists = [
    TokenList1: [Word(user123), Word(logged), Word(in)]
    TokenList2: [Word(user456), Word(logged), Word(in)]
    TokenList3: [Word(user789), Word(logged), Word(in)]
  ]

Pattern Generation:
  Cluster1.GeneratePattern()
  → Position 0: "user123" ≠ "user456" → Wildcard(*)
  → Position 1: "logged" = "logged" → Word(logged)
  → Position 2: "in" = "in" → Word(in)
  → Result: [Wildcard(*), Word(logged), Word(in)]
```

### Two-Level Lookup

The ClusterManager uses a two-level structure for efficiency:

1. **Hash bucket lookup** (O(1)): `hashBuckets[signature.Hash]` returns array of clusters
2. **Signature comparison** (O(n) where n = clusters in bucket): Find exact signature match

1. **Tokenization**: Log message → TokenList (via automaton)
2. **Signature generation**: TokenList → Signature (position + count hash)
3. **Hash bucket lookup**: Use signature.Hash to find bucket (O(1))
4. **Signature matching**: Within bucket, compare full signatures to find exact match
5. **Add to cluster**: If match found, add TokenList; otherwise create new cluster
6. **Pattern generation**: Analyze cluster TokenLists to identify wildcard positions

### Pattern Generation
// Still need more work here to avoid overclustering
When a cluster has multiple TokenLists:
- Compare token values at each position
- If all values match → keep literal token
- If values differ → create wildcard token
- Result: Pattern with wildcards at variable positions

### Example

```go
// Create cluster manager
manager := clustering.NewClusterManager()

// Add log messages (as TokenLists)
tokenList1 := tokenizer.Tokenize("user123 logged in")
tokenList2 := tokenizer.Tokenize("user456 logged in")
tokenList3 := tokenizer.Tokenize("user789 logged in")

manager.Add(tokenList1)  // Creates Cluster1
manager.Add(tokenList2)  // Adds to Cluster1 (same signature)
manager.Add(tokenList3)  // Adds to Cluster1 (same signature)

// Get cluster and generate pattern
signature := token.NewSignature(tokenList1)
cluster := manager.GetCluster(signature)
pattern := cluster.GeneratePattern()

// Result: [Wildcard(*), Word(logged), Word(in)]
```

## Timestamp Tracking (In Progress)

Clusters track timestamps for stateful encoding:
- `CreatedAt`: When pattern was first discovered
- `UpdatedAt`: When pattern was last modified
- `LastSentAt`: When pattern was last sent to gRPC (for incremental updates)
