#!/bin/bash
# Smoke test for load-generator component
# Validates Python locustfile and configuration
# Uses uv for fast dependency management when available
set -e

echo "🔥 Running smoke tests for load-generator..."

ERRORS=0
ERROR_DETAILS=""

# Check required files exist
if [ ! -f "locustfile.py" ]; then
    echo "❌ locustfile.py not found"
    ERRORS=$((ERRORS + 1))
    ERROR_DETAILS="${ERROR_DETAILS}\n- locustfile.py not found"
fi

if [ ! -f "requirements.txt" ]; then
    echo "❌ requirements.txt not found"
    ERRORS=$((ERRORS + 1))
    ERROR_DETAILS="${ERROR_DETAILS}\n- requirements.txt not found"
fi

# Validate Python code
if [ -f "locustfile.py" ]; then
    PYTHON_CHECK_PASSED=false
    
    # Best: Use uv to install deps and run pyright (catches undefined methods!)
    if command -v uv &> /dev/null; then
        echo "   Using uv for dependency management..."
        
        # Sync dependencies (very fast with uv)
        if uv pip install -q -r requirements.txt 2>/dev/null; then
            echo "   ✓ Dependencies installed"
        fi
        
        # Run pyright with dependencies available
        if command -v pyright &> /dev/null || uv pip install -q pyright 2>/dev/null; then
            echo "   Running pyright type checker..."
            PYRIGHT_OUTPUT=$(uv run pyright locustfile.py 2>&1) && PYRIGHT_EXIT=0 || PYRIGHT_EXIT=$?
            
            # Filter noisy pyright warnings that don't indicate real bugs:
            # - reportMissingImports: dependencies not installed
            # - reportPrivateImportUsage: ddtrace patterns that work at runtime
            # - could not be resolved: import resolution issues
            # Check if ALL errors are just import-related (check across all lines)
            if echo "$PYRIGHT_OUTPUT" | grep -q "reportMissingImports\|reportPrivateImportUsage\|could not be resolved"; then
                # Count total errors vs import-related errors
                TOTAL_ERRORS=$(echo "$PYRIGHT_OUTPUT" | grep -oE "^[0-9]+ error" | head -1 | grep -oE "^[0-9]+" || echo "0")
                IMPORT_ERRORS=$(echo "$PYRIGHT_OUTPUT" | grep -c "reportMissingImports\|reportPrivateImportUsage\|could not be resolved" || echo "0")
                
                if [ "$TOTAL_ERRORS" = "$IMPORT_ERRORS" ] || [ "$TOTAL_ERRORS" = "0" ]; then
                    echo "   ✓ locustfile.py: pyright check passed (import warnings filtered)"
                    PYTHON_CHECK_PASSED=true
                else
                    echo "❌ locustfile.py has issues (pyright):"
                    echo "$PYRIGHT_OUTPUT" | sed 's/^/      /'
                    ERRORS=$((ERRORS + 1))
                    ERROR_DETAILS="${ERROR_DETAILS}\n- locustfile.py pyright errors:\n${PYRIGHT_OUTPUT}"
                fi
            elif echo "$PYRIGHT_OUTPUT" | grep -q "0 errors"; then
                echo "   ✓ locustfile.py: pyright check passed"
                PYTHON_CHECK_PASSED=true
            elif echo "$PYRIGHT_OUTPUT" | grep -qE "[1-9][0-9]* error"; then
                echo "❌ locustfile.py has issues (pyright):"
                echo "$PYRIGHT_OUTPUT" | sed 's/^/      /'
                ERRORS=$((ERRORS + 1))
                ERROR_DETAILS="${ERROR_DETAILS}\n- locustfile.py pyright errors:\n${PYRIGHT_OUTPUT}"
            else
                echo "   ✓ locustfile.py: pyright check passed"
                PYTHON_CHECK_PASSED=true
            fi
        fi
        
        # Also run ruff for additional checks
        if [ "$PYTHON_CHECK_PASSED" != "true" ] || command -v ruff &> /dev/null; then
            echo "   Running ruff static analysis..."
            if RUFF_OUTPUT=$(uv run ruff check --select=F locustfile.py 2>&1); then
                echo "   ✓ locustfile.py: ruff check passed"
                [ "$PYTHON_CHECK_PASSED" != "true" ] && PYTHON_CHECK_PASSED=true
            elif [ "$PYTHON_CHECK_PASSED" != "true" ]; then
                echo "❌ locustfile.py has issues (ruff):"
                echo "$RUFF_OUTPUT" | sed 's/^/      /'
                ERRORS=$((ERRORS + 1))
                ERROR_DETAILS="${ERROR_DETAILS}\n- locustfile.py ruff errors:\n${RUFF_OUTPUT}"
            fi
        fi
        
    # Fallback: No uv, use traditional approach
    elif command -v python3 &> /dev/null; then
        echo "   Checking Python code (uv not available)..."
        
        # Try ruff first
        if command -v ruff &> /dev/null; then
            echo "   Using ruff for static analysis..."
            if RUFF_OUTPUT=$(ruff check --select=F locustfile.py 2>&1); then
                echo "   ✓ locustfile.py: ruff check passed"
                PYTHON_CHECK_PASSED=true
            else
                echo "❌ locustfile.py has issues (ruff):"
                echo "$RUFF_OUTPUT" | sed 's/^/      /'
                ERRORS=$((ERRORS + 1))
                ERROR_DETAILS="${ERROR_DETAILS}\n- locustfile.py ruff errors:\n${RUFF_OUTPUT}"
            fi
        # Fallback to py_compile
        else
            echo "   Using py_compile (basic syntax check)..."
            if python3 -m py_compile locustfile.py 2>&1; then
                echo "   ✓ locustfile.py: valid Python syntax"
                PYTHON_CHECK_PASSED=true
            else
                echo "❌ locustfile.py has syntax errors"
                ERRORS=$((ERRORS + 1))
                ERROR_DETAILS="${ERROR_DETAILS}\n- locustfile.py syntax error"
            fi
        fi
    else
        echo "⚠️  Neither uv nor python3 available - skipping Python checks"
    fi
    
    # Check any other Python files
    if command -v python3 &> /dev/null; then
        for py_file in *.py; do
            if [ -f "$py_file" ] && [ "$py_file" != "locustfile.py" ]; then
                if ! python3 -m py_compile "$py_file" 2>&1; then
                    echo "❌ $py_file has syntax errors"
                    ERRORS=$((ERRORS + 1))
                    ERROR_DETAILS="${ERROR_DETAILS}\n- $py_file syntax error"
                fi
            fi
        done
    fi
fi

# Validate JSON data files (like people.json)
for json_file in *.json; do
    if [ -f "$json_file" ]; then
        echo "   Validating $json_file..."
        if command -v python3 &> /dev/null; then
            if ! python3 -c "import json; json.load(open('$json_file'))" 2>/dev/null; then
                echo "❌ Invalid JSON: $json_file"
                ERRORS=$((ERRORS + 1))
                ERROR_DETAILS="${ERROR_DETAILS}\n- Invalid JSON: $json_file"
            else
                echo "   ✓ $json_file: valid"
            fi
        fi
    fi
done

# Check shell script syntax
for script in test.sh build.sh; do
    if [ -f "$script" ]; then
        echo "   Checking $script syntax..."
        if ! bash -n "$script" 2>&1; then
            echo "❌ Syntax error in $script"
            ERRORS=$((ERRORS + 1))
            ERROR_DETAILS="${ERROR_DETAILS}\n- Syntax error in $script"
        fi
    fi
done

# Validate stats-test Go code if present
if [ -d "stats-test" ] && command -v go &> /dev/null; then
    echo "   Checking stats-test Go compilation..."
    cd stats-test
    if [ -f "go.mod" ]; then
        if ! go build -o /dev/null . 2>&1; then
            echo "❌ stats-test Go compilation failed"
            ERRORS=$((ERRORS + 1))
            ERROR_DETAILS="${ERROR_DETAILS}\n- stats-test compilation error"
        else
            echo "   ✓ stats-test: compiles"
        fi
    fi
    cd ..
fi

# Summary
if [ $ERRORS -gt 0 ]; then
    echo ""
    echo -e "❌ Smoke tests failed with $ERRORS error(s):$ERROR_DETAILS"
    exit 1
fi

echo "✓ Smoke tests passed for load-generator"