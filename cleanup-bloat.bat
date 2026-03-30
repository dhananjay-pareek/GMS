@echo off
echo ========================================
echo   Cleaning up bloat from repository
echo ========================================
echo.

REM Root directory junk
if exist "temp.html" (
    del /f /q "temp.html"
    echo [DELETED] temp.html
)

if exist "command to rebuild and start.txt" (
    del /f /q "command to rebuild and start.txt"
    echo [DELETED] command to rebuild and start.txt
)

if exist "START_APP.bat" (
    del /f /q "START_APP.bat"
    echo [DELETED] START_APP.bat
)

if exist "banner.png" (
    del /f /q "banner.png"
    echo [DELETED] banner.png
)

if exist "gmaps-extractor.md" (
    del /f /q "gmaps-extractor.md"
    echo [DELETED] gmaps-extractor.md
)

if exist "lint.go" (
    del /f /q "lint.go"
    echo [DELETED] lint.go
)

if exist "MacOS instructions.md" (
    del /f /q "MacOS instructions.md"
    echo [DELETED] MacOS instructions.md
)

if exist "AGENTS.md" (
    del /f /q "AGENTS.md"
    echo [DELETED] AGENTS.md
)

REM Compiled binaries
if exist "google-maps-scraper" (
    del /f /q "google-maps-scraper"
    echo [DELETED] google-maps-scraper
)

if exist "google-maps-scraper.exe" (
    del /f /q "google-maps-scraper.exe"
    echo [DELETED] google-maps-scraper.exe
)

if exist "bin" (
    rd /s /q "bin"
    echo [DELETED] bin/ directory
)

REM Marketing images
if exist "img" (
    rd /s /q "img"
    echo [DELETED] img/ directory
)

REM Test/debug data
if exist "testdata" (
    rd /s /q "testdata"
    echo [DELETED] testdata/ directory
)

REM Example plugins
if exist "examples\plugins" (
    rd /s /q "examples\plugins"
    echo [DELETED] examples/plugins/ directory
)

REM Launcher folder (not working/not used)
if exist "launcher" (
    rd /s /q "launcher"
    echo [DELETED] launcher/ directory
)

REM Remove go.work files if present (optional - only if not using workspace)
if exist "go.work" (
    del /f /q "go.work"
    echo [DELETED] go.work
)

if exist "go.work.sum" (
    del /f /q "go.work.sum"
    echo [DELETED] go.work.sum
)

echo.
echo ========================================
echo   Cleanup complete!
echo ========================================
echo.
echo Next steps:
echo 1. Review the changes with: git status
echo 2. Update .gitignore to prevent these from coming back
echo 3. Commit the cleanup: git add -A ^&^& git commit -m "chore: remove bloat and unused files"
echo.
pause
