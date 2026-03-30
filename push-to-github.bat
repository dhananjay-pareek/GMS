@echo off
echo ==========================================
echo   Pushing to GitHub Repository
echo ==========================================
echo.
echo Repository: https://github.com/dhananjay-pareek/GMAP-Scrapper
echo.

cd /d D:\gmap-new

echo Step 1: Checking Git status...
echo.
git status
echo.

echo ==========================================
pause
echo.

echo Step 2: Adding all files...
git add -A
echo.

echo Step 3: Committing changes...
git commit -m "feat: add Render deployment support (v1.11.0)" -m "" -m "Major Features:" -m "- Environment variable support (PORT, DATA_FOLDER, CONCURRENCY, DATABASE_URL)" -m "- Health check endpoints (/health, /healthz)" -m "- One-click Render deployment (render.yaml)" -m "- Comprehensive documentation (RENDER.md - 8000+ words)" -m "- Repository URL updates to match actual repo" -m "" -m "Cleanup:" -m "- Removed 20MB of bloat (launcher/, img/, testdata/, bin/)" -m "- Removed temporary files and unused assets" -m "- Enhanced .gitignore" -m "" -m "Documentation:" -m "- RENDER.md - Complete Render deployment guide" -m "- RENDER_IMPLEMENTATION.md - Technical details" -m "- CLEANUP_README.md - Cleanup documentation" -m "- DEPLOY_READY.md - Quick reference" -m "- RELEASE_NOTES_v1.11.0.md - Full release notes" -m "- Updated README.md with Render section" -m "" -m "Modified Files:" -m "- runner/runner.go - Environment variable support" -m "- web/web.go - Health endpoints" -m "- README.md - Render section and URL fixes" -m "- Dockerfile - Render compatibility" -m "- render.yaml - Repository URL fix" -m "- .gitignore - Enhanced" -m "" -m "Breaking Changes: None (fully backward compatible)" -m "" -m "Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
echo.

if %ERRORLEVEL% NEQ 0 (
    echo.
    echo [ERROR] Commit failed!
    echo This might be because there are no changes to commit.
    echo.
    pause
    exit /b 1
)

echo Step 4: Pushing to GitHub...
echo.
git push origin main
echo.

if %ERRORLEVEL% NEQ 0 (
    echo.
    echo ==========================================
    echo [ERROR] Push failed!
    echo ==========================================
    echo.
    echo Common issues:
    echo 1. Authentication required - You may need to set up Git credentials
    echo 2. Branch name might be different - Try: git push origin master
    echo 3. Remote not configured properly
    echo.
    echo To fix authentication:
    echo   - GitHub CLI: gh auth login
    echo   - Personal Access Token: Store in credential manager
    echo   - SSH Key: Use git@github.com:dhananjay-pareek/GMAP-Scrapper.git
    echo.
    pause
    exit /b 1
)

echo.
echo ==========================================
echo   SUCCESS! Changes pushed to GitHub
echo ==========================================
echo.
echo Your changes are now live at:
echo https://github.com/dhananjay-pareek/GMAP-Scrapper
echo.
echo Next Steps:
echo 1. Visit: https://github.com/dhananjay-pareek/GMAP-Scrapper/releases
echo 2. Click "Draft a new release"
echo 3. Tag: v1.11.0
echo 4. Title: v1.11.0 - Render Deployment Support
echo 5. Copy description from: RELEASE_NOTES_v1.11.0.md
echo 6. Click "Publish release"
echo.
echo Or deploy to Render now:
echo https://dashboard.render.com
echo.
pause
