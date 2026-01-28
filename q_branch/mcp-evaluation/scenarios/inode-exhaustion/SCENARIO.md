# Inode Exhaustion

## Difficulty
Hard

## Problem Statement
Filesystem running out of inodes despite having free disk space, preventing new file creation due to creating millions of tiny files.

## Symptoms
Observable symptoms that an SRE would notice:
- "No space left on device" error despite df showing free space
- `df -i` shows 100% inode usage
- Cannot create new files or directories
- Disk space available but inode count at maximum
- Millions of small files in directory

## Root Cause
Process creating massive numbers of small files (1-byte each), exhausting filesystem inodes before disk space runs out. Each file consumes one inode regardless of size.

## Investigation Steps
Recommended approach to diagnose:
1. **Check disk space**: Use `get_disk_usage` - shows space available
2. **Check inodes**: Use `bash_execute` with `df -i` - shows 100% inode usage
3. **Find file-heavy directory**: Use `bash_execute` with `find /tmp -type f | wc -l`
4. **Identify process**: Use `bash_execute` with `lsof` or check recent file creation
5. **Confirm small files**: Use `bash_execute` with `ls -lh` to see tiny files

## Acceptable Diagnoses
For the LLM autograder, list acceptable diagnostic conclusions:

### Primary Diagnosis (Full Credit)
- Identified inode exhaustion (100% inode usage)
- Found millions of small files
- Root cause: Too many small files exhausting inodes

### Alternative Valid Diagnoses (Partial Credit)
- "Out of inodes"
- "Too many small files"
- "Inode limit reached"

### Key Terms (should be mentioned)
- "inode", "exhaustion", "100%"
- "small files" or "many files"
- df -i output

### Common Errors (deductions)
- Confusing with disk space issue (-30%)
- Not checking inode usage (-40%)
- Not understanding inode concept (-25%)

## Setup Instructions
```bash
cd /mcp/scenarios/inode-exhaustion
python3 workload.py > /tmp/inode-exhaustion.log 2>&1 &
echo $! > /tmp/inode-exhaustion.pid
```

## Cleanup Instructions
```bash
if [ -f /tmp/inode-exhaustion.pid ]; then
    kill $(cat /tmp/inode-exhaustion.pid)
    rm /tmp/inode-exhaustion.pid
fi
rm -f /tmp/inode-exhaustion.log
# WARNING: This may take a while if many files created
rm -rf /tmp/cache_files/
```

## Expected Timeline
- **Medium term** (5-15 minutes)
- Creates 1000 files/second
- Time depends on inode limit (typically 1-10 million)

## Success Criteria
- [x] Identified inode exhaustion
- [x] Found directory with millions of files
- [x] Understood inode vs disk space difference
- [x] Proposed mitigation: Delete files, use different storage approach

## Autograder Scoring Rubric

### Inode Identification (25 points)
- 25pts: Found 100% inode usage with df -i
- 10pts: Mentioned inode without verification
- 0pts: Did not check inodes

### File Discovery (25 points)
- 25pts: Found directory with massive file count
- 10pts: Mentioned many files without location
- 0pts: Did not find files

### Root Cause Analysis (30 points)
- 30pts: Explained inode exhaustion from many small files
- 20pts: Mentioned too many files without inode explanation
- 0pts: No analysis

### Mitigation Proposal (20 points)
- 20pts: Specific solution (delete files, restructure storage)
- 10pts: Generic solution
- 0pts: No mitigation
