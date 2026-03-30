# GitHub Release Instructions

Follow these steps to create a new release on GitHub.

## Step 1: Commit All Changes

Open Command Prompt or Git Bash in your project folder and run:

```bash
cd D:\gmap-new

# Check what's changed
git status

# Add all changes
git add -A

# Commit with detailed message
git commit -m "feat: add Render deployment support (v1.11.0)

Major Changes:
- Add PORT, DATA_FOLDER, CONCURRENCY, DATABASE_URL env var support
- Add /health and /healthz health check endpoints
- Create render.yaml for one-click Render deployment
- Add comprehensive RENDER.md deployment guide (8000+ words)
- Update README with Render deployment section
- Remove 20MB of bloat (launcher/, img/, testdata/, bin/)
- Update .gitignore to prevent bloat from returning

New Files:
- render.yaml - Render service configuration
- RENDER.md - Complete deployment guide
- RENDER_IMPLEMENTATION.md - Technical details
- CLEANUP_README.md - Cleanup documentation
- DEPLOY_READY.md - Quick reference
- RELEASE_NOTES_v1.11.0.md - This release notes
- cleanup-bloat.bat/sh - Cleanup scripts
- test-render-changes.bat/sh - Test scripts

Modified Files:
- runner/runner.go - Environment variable support
- web/web.go - Health check endpoints
- README.md - Render deployment section
- Dockerfile - Render compatibility
- .gitignore - Enhanced exclusions

Removed:
- launcher/ - Non-functional GUI (~1MB)
- img/ - Unused marketing images (~15MB)
- testdata/ - Debug JSON files
- bin/ - Compiled binaries
- Various temporary and redundant files

This release makes the app fully compatible with Render.com for
one-click cloud deployment with persistent storage, health monitoring,
and production-ready configuration.

Breaking Changes: None (fully backward compatible)

Closes #render-deployment"

# Push to GitHub
git push origin main
```

## Step 2: Create GitHub Release

### Option A: Via GitHub Website (Recommended)

1. **Go to your repository**
   - https://github.com/dhananjay-pareek/GMAP-Scrapper

2. **Click "Releases"** (right sidebar)

3. **Click "Draft a new release"**

4. **Fill in the form:**
   - **Tag version**: `v1.11.0`
   - **Target**: `main` (or your default branch)
   - **Release title**: `v1.11.0 - Render Deployment Support`
   - **Description**: Copy/paste content from `RELEASE_NOTES_v1.11.0.md`

5. **Optional: Attach binaries**
   - Build binaries for different platforms:
   ```bash
   # Windows
   GOOS=windows GOARCH=amd64 go build -o google-maps-scraper-v1.11.0-windows-amd64.exe

   # Linux
   GOOS=linux GOARCH=amd64 go build -o google-maps-scraper-v1.11.0-linux-amd64

   # macOS Intel
   GOOS=darwin GOARCH=amd64 go build -o google-maps-scraper-v1.11.0-darwin-amd64
   
   # macOS Apple Silicon
   GOOS=darwin GOARCH=arm64 go build -o google-maps-scraper-v1.11.0-darwin-arm64
   ```
   - Upload these files in the "Attach binaries" section

6. **Check "Set as the latest release"**

7. **Click "Publish release"**

### Option B: Via GitHub CLI (Advanced)

If you have GitHub CLI installed:

```bash
# Create release with notes from file
gh release create v1.11.0 \
  --title "v1.11.0 - Render Deployment Support" \
  --notes-file RELEASE_NOTES_v1.11.0.md \
  --latest

# Optional: Upload binaries
# gh release upload v1.11.0 google-maps-scraper-v1.11.0-*
```

## Step 3: Update render.yaml Repository URL

Make sure the `render.yaml` file has the correct repository URL:

```yaml
services:
  - type: web
    name: google-maps-scraper
    runtime: docker
    repo: https://github.com/dhananjay-pareek/GMAP-Scrapper  # ✅ Your repo
    branch: main
```

Commit this change if needed:
```bash
git add render.yaml
git commit -m "fix: update render.yaml repository URL"
git push origin main
```

## Step 4: Test the Release

1. **Verify release is visible**
   - Go to https://github.com/dhananjay-pareek/GMAP-Scrapper/releases
   - You should see v1.11.0 at the top

2. **Test deployment to Render**
   - Go to https://dashboard.render.com
   - Click "New +" → "Blueprint"
   - Select your repository
   - Verify it detects `render.yaml`
   - Click "Apply"
   - Monitor the deployment

3. **Verify health endpoint**
   ```bash
   # Once deployed
   curl https://your-app.onrender.com/health
   ```

## Step 5: Announce the Release (Optional)

Share the release:
- Twitter/X
- LinkedIn
- Reddit (r/webscraping, r/golang)
- Hacker News
- Product Hunt

Example announcement:
```
🚀 Google Maps Scraper v1.11.0 is out!

Now with one-click deployment to Render.com:
✅ Environment variable configuration
✅ Health monitoring endpoints
✅ Persistent storage support
✅ 20MB lighter (bloat removed)
✅ Comprehensive docs

Deploy now: https://github.com/dhananjay-pareek/GMAP-Scrapper

#opensource #webscraping #golang #render
```

## Troubleshooting

### "Authentication failed" when pushing
```bash
# Use HTTPS with token or SSH
git remote set-url origin https://github.com/dhananjay-pareek/GMAP-Scrapper.git
# or
git remote set-url origin git@github.com:dhananjay-pareek/GMAP-Scrapper.git
```

### "Permission denied"
- Make sure you have write access to the repository
- Check your GitHub authentication (token or SSH key)

### "Nothing to commit"
```bash
# Check what's untracked
git status

# Make sure you added files
git add -A
```

### Release tag already exists
```bash
# Delete the tag locally and remotely
git tag -d v1.11.0
git push origin :refs/tags/v1.11.0

# Create again
git tag v1.11.0
git push origin v1.11.0
```

## Next Steps After Release

1. ✅ Star your own repository (looks professional!)
2. ✅ Update repository topics/tags: golang, web-scraping, google-maps, render, docker
3. ✅ Add repository description: "High-performance Google Maps scraper with web UI - Deploy to Render in one click"
4. ✅ Enable GitHub Discussions for community
5. ✅ Add badges to README (build status, release version, license)

---

**Ready to create the release?** Follow the steps above!

If you need any help, feel free to ask!
