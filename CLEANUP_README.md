# Cleanup Summary

## Files to be Removed

This cleanup script removes bloat and unused files from the repository before deploying to Render.

### Root Directory Junk (8 files)
- `temp.html` - Corrupted HTML file (27KB)
- `command to rebuild and start.txt` - Personal dev notes
- `START_APP.bat` - Windows-only dev script
- `banner.png` - Unused image
- `gmaps-extractor.md` - Marketing copy
- `lint.go` - Build tag helper (not needed)
- `MacOS instructions.md` - OS-specific build guide
- `AGENTS.md` - AI agent development guide

### Compiled Binaries
- `google-maps-scraper` - Compiled binary (Linux)
- `google-maps-scraper.exe` - Compiled binary (Windows)
- `bin/` - Directory with compiled executables

### Marketing Assets
- `img/` - 12 banner images (~15MB) not referenced in code

### Test/Debug Data
- `testdata/` - 5 JSON files with panic/debug data

### Unused Code
- `examples/plugins/` - Example plugin code
- `launcher/` - **Non-working GUI launcher folder (separate module)**

### Go Workspace Files (Optional)
- `go.work` - Only needed for multi-module development
- `go.work.sum` - Workspace checksum

## Total Space Saved
Approximately **15-20 MB** of unnecessary files

## Usage

### Windows
```batch
cleanup-bloat.bat
```

### Linux/Mac
```bash
chmod +x cleanup-bloat.sh
./cleanup-bloat.sh
```

## After Cleanup

1. Review changes: `git status`
2. The improved `.gitignore` will prevent these files from being tracked again
3. Commit: `git add -A && git commit -m "chore: remove bloat and unused files"`

## Notes
- The launcher folder is a separate Fyne GUI application that's not integrated with the main app
- All marketing images are unused in the codebase
- Test data files contain only debug/panic information
