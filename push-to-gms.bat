@echo off
echo Pushing existing repository to GitHub (GMS)...
echo.

REM Add the new remote
echo Adding remote origin...
git remote add origin https://github.com/dhananjay-pareek/GMS.git
if errorlevel 1 (
    echo Remote might already exist, trying to set URL instead...
    git remote set-url origin https://github.com/dhananjay-pareek/GMS.git
)

REM Rename branch to main
echo Renaming branch to main...
git branch -M main

REM Push to the new repository
echo Pushing to origin main...
git push -u origin main

echo.
echo Done! Check above for any errors.
pause
