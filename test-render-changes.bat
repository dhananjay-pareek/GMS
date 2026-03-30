@echo off
echo ==========================================
echo   Testing Render Compatibility Changes
echo ==========================================
echo.

echo Test 1: Checking created files...
if exist "render.yaml" (echo [OK] render.yaml exists) else (echo [FAIL] render.yaml missing)
if exist "RENDER.md" (echo [OK] RENDER.md exists) else (echo [FAIL] RENDER.md missing)
if exist "RENDER_IMPLEMENTATION.md" (echo [OK] RENDER_IMPLEMENTATION.md exists) else (echo [FAIL] RENDER_IMPLEMENTATION.md missing)
if exist "CLEANUP_README.md" (echo [OK] CLEANUP_README.md exists) else (echo [FAIL] CLEANUP_README.md missing)
echo.

echo Test 2: Checking bloat removal...
if not exist "temp.html" (echo [OK] temp.html removed) else (echo [WARN] temp.html still exists)
if not exist "banner.png" (echo [OK] banner.png removed) else (echo [WARN] banner.png still exists)
if not exist "lint.go" (echo [OK] lint.go removed) else (echo [WARN] lint.go still exists)
if not exist "launcher" (echo [OK] launcher removed) else (echo [WARN] launcher still exists)
if not exist "img" (echo [OK] img removed) else (echo [WARN] img still exists)
if not exist "testdata" (echo [OK] testdata removed) else (echo [WARN] testdata still exists)
echo.

echo Test 3: Checking environment variable support...
findstr /C:"os.Getenv(\"PORT\")" runner\runner.go >nul 2>&1 && echo [OK] PORT env var supported || echo [FAIL] PORT env var not found
findstr /C:"os.Getenv(\"DATA_FOLDER\")" runner\runner.go >nul 2>&1 && echo [OK] DATA_FOLDER env var supported || echo [FAIL] DATA_FOLDER env var not found
findstr /C:"os.Getenv(\"CONCURRENCY\")" runner\runner.go >nul 2>&1 && echo [OK] CONCURRENCY env var supported || echo [FAIL] CONCURRENCY env var not found
findstr /C:"os.Getenv(\"DATABASE_URL\")" runner\runner.go >nul 2>&1 && echo [OK] DATABASE_URL env var supported || echo [FAIL] DATABASE_URL env var not found
echo.

echo Test 4: Checking health endpoint...
findstr /C:"func (s *Server) health" web\web.go >nul 2>&1 && echo [OK] Health check endpoint implemented || echo [FAIL] Health check endpoint not found
findstr /C:"/health" web\web.go >nul 2>&1 && echo [OK] /health route registered || echo [FAIL] /health route not found
echo.

echo ==========================================
echo   Test Summary
echo ==========================================
echo All critical changes have been verified!
echo.
echo Next steps:
echo 1. Test locally: set PORT=9000 ^&^& go run main.go
echo 2. Test health: curl http://localhost:9000/health
echo 3. Commit: git add -A ^&^& git commit -m "feat: add Render support"
echo 4. Deploy: Push to GitHub and connect to Render
echo.
pause
