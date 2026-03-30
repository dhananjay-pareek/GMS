#!/bin/bash

echo "========================================"
echo "  Cleaning up bloat from repository"
echo "========================================"
echo

# Root directory junk
rm -f "temp.html" && echo "[DELETED] temp.html"
rm -f "command to rebuild and start.txt" && echo "[DELETED] command to rebuild and start.txt"
rm -f "START_APP.bat" && echo "[DELETED] START_APP.bat"
rm -f "banner.png" && echo "[DELETED] banner.png"
rm -f "gmaps-extractor.md" && echo "[DELETED] gmaps-extractor.md"
rm -f "lint.go" && echo "[DELETED] lint.go"
rm -f "MacOS instructions.md" && echo "[DELETED] MacOS instructions.md"
rm -f "AGENTS.md" && echo "[DELETED] AGENTS.md"

# Compiled binaries
rm -f "google-maps-scraper" && echo "[DELETED] google-maps-scraper"
rm -f "google-maps-scraper.exe" && echo "[DELETED] google-maps-scraper.exe"
rm -rf "bin" && echo "[DELETED] bin/ directory"

# Marketing images
rm -rf "img" && echo "[DELETED] img/ directory"

# Test/debug data
rm -rf "testdata" && echo "[DELETED] testdata/ directory"

# Example plugins
rm -rf "examples/plugins" && echo "[DELETED] examples/plugins/ directory"

# Launcher folder (not working/not used)
rm -rf "launcher" && echo "[DELETED] launcher/ directory"

# Remove go.work files if present (optional - only if not using workspace)
rm -f "go.work" && echo "[DELETED] go.work"
rm -f "go.work.sum" && echo "[DELETED] go.work.sum"

echo
echo "========================================"
echo "  Cleanup complete!"
echo "========================================"
echo
echo "Next steps:"
echo "1. Review the changes with: git status"
echo "2. Update .gitignore to prevent these from coming back"
echo "3. Commit the cleanup: git add -A && git commit -m 'chore: remove bloat and unused files'"
echo
