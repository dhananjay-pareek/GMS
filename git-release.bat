@echo off
echo ==========================================
echo   Quick Git Commands for Release
echo ==========================================
echo.

echo Step 1: Check status
git status
echo.

echo ==========================================
echo Ready to commit? (This will commit all changes)
echo ==========================================
pause

echo.
echo Step 2: Adding all changes...
git add -A

echo.
echo Step 3: Committing...
git commit -m "feat: add Render deployment support (v1.11.0)" -m "- Add PORT, DATA_FOLDER, CONCURRENCY, DATABASE_URL env support" -m "- Add /health and /healthz health check endpoints" -m "- Create render.yaml for one-click deployment" -m "- Add comprehensive RENDER.md guide (8000+ words)" -m "- Remove 20MB of bloat (launcher/, img/, testdata/)" -m "- Update .gitignore to prevent bloat"

echo.
echo Step 4: Pushing to GitHub...
git push origin main

echo.
echo ==========================================
echo   Success! Changes pushed to GitHub
echo ==========================================
echo.
echo Next steps:
echo 1. Go to: https://github.com/dhananjay-pareek/GMAP-Scrapper/releases
echo 2. Click "Draft a new release"
echo 3. Tag: v1.11.0
echo 4. Title: v1.11.0 - Render Deployment Support
echo 5. Copy release notes from: RELEASE_NOTES_v1.11.0.md
echo 6. Click "Publish release"
echo.
pause
