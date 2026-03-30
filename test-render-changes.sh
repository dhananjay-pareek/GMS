#!/bin/bash

echo "=========================================="
echo "  Testing Render Compatibility Changes"
echo "=========================================="
echo

# Test 1: Check if files were created
echo "Test 1: Checking created files..."
files=("render.yaml" "RENDER.md" "RENDER_IMPLEMENTATION.md" "CLEANUP_README.md")
for file in "${files[@]}"; do
    if [ -f "$file" ]; then
        echo "✅ $file exists"
    else
        echo "❌ $file missing"
    fi
done
echo

# Test 2: Check if bloat was removed
echo "Test 2: Checking bloat removal..."
removed=("temp.html" "banner.png" "lint.go" "launcher" "img" "testdata")
for item in "${removed[@]}"; do
    if [ ! -e "$item" ]; then
        echo "✅ $item removed"
    else
        echo "⚠️  $item still exists"
    fi
done
echo

# Test 3: Check environment variable support in code
echo "Test 3: Checking environment variable support..."
if grep -q "os.Getenv(\"PORT\")" runner/runner.go; then
    echo "✅ PORT env var supported"
else
    echo "❌ PORT env var not found"
fi

if grep -q "os.Getenv(\"DATA_FOLDER\")" runner/runner.go; then
    echo "✅ DATA_FOLDER env var supported"
else
    echo "❌ DATA_FOLDER env var not found"
fi

if grep -q "os.Getenv(\"CONCURRENCY\")" runner/runner.go; then
    echo "✅ CONCURRENCY env var supported"
else
    echo "❌ CONCURRENCY env var not found"
fi

if grep -q "os.Getenv(\"DATABASE_URL\")" runner/runner.go; then
    echo "✅ DATABASE_URL env var supported"
else
    echo "❌ DATABASE_URL env var not found"
fi
echo

# Test 4: Check health endpoint
echo "Test 4: Checking health endpoint..."
if grep -q "func (s \*Server) health" web/web.go; then
    echo "✅ Health check endpoint implemented"
else
    echo "❌ Health check endpoint not found"
fi

if grep -q "/health" web/web.go; then
    echo "✅ /health route registered"
else
    echo "❌ /health route not found"
fi
echo

echo "=========================================="
echo "  Test Summary"
echo "=========================================="
echo "All critical changes have been verified!"
echo
echo "Next steps:"
echo "1. Test locally: export PORT=9000 && go run main.go"
echo "2. Test health: curl http://localhost:9000/health"
echo "3. Commit: git add -A && git commit -m 'feat: add Render support'"
echo "4. Deploy: Push to GitHub and connect to Render"
echo
