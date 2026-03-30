@echo off
REM Run this file by double-clicking it or running: cleanup-now.bat

echo ========================================
echo   Cleaning up bloat from repository
echo ========================================
echo.

del /f /q "temp.html" 2>nul && echo [DELETED] temp.html || echo [SKIP] temp.html not found
del /f /q "command to rebuild and start.txt" 2>nul && echo [DELETED] command to rebuild and start.txt || echo [SKIP] command to rebuild and start.txt not found
del /f /q "START_APP.bat" 2>nul && echo [DELETED] START_APP.bat || echo [SKIP] START_APP.bat not found
del /f /q "banner.png" 2>nul && echo [DELETED] banner.png || echo [SKIP] banner.png not found
del /f /q "gmaps-extractor.md" 2>nul && echo [DELETED] gmaps-extractor.md || echo [SKIP] gmaps-extractor.md not found
del /f /q "lint.go" 2>nul && echo [DELETED] lint.go || echo [SKIP] lint.go not found
del /f /q "MacOS instructions.md" 2>nul && echo [DELETED] MacOS instructions.md || echo [SKIP] MacOS instructions.md not found
del /f /q "AGENTS.md" 2>nul && echo [DELETED] AGENTS.md || echo [SKIP] AGENTS.md not found
del /f /q "google-maps-scraper" 2>nul && echo [DELETED] google-maps-scraper || echo [SKIP] google-maps-scraper not found
del /f /q "google-maps-scraper.exe" 2>nul && echo [DELETED] google-maps-scraper.exe || echo [SKIP] google-maps-scraper.exe not found
del /f /q "go.work" 2>nul && echo [DELETED] go.work || echo [SKIP] go.work not found
del /f /q "go.work.sum" 2>nul && echo [DELETED] go.work.sum || echo [SKIP] go.work.sum not found

echo.
echo Removing directories...
if exist "bin" (rd /s /q "bin" && echo [DELETED] bin/) else (echo [SKIP] bin/ not found)
if exist "img" (rd /s /q "img" && echo [DELETED] img/) else (echo [SKIP] img/ not found)
if exist "testdata" (rd /s /q "testdata" && echo [DELETED] testdata/) else (echo [SKIP] testdata/ not found)
if exist "launcher" (rd /s /q "launcher" && echo [DELETED] launcher/) else (echo [SKIP] launcher/ not found)
if exist "examples\plugins" (rd /s /q "examples\plugins" && echo [DELETED] examples\plugins/) else (echo [SKIP] examples\plugins/ not found)

echo.
echo ========================================
echo   Cleanup complete!
echo ========================================
echo.
echo Run: git status
echo to see what was removed
echo.
