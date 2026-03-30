import os

files_to_delete = [
    'a.txt',
    'cleanup-bloat.bat',
    'cleanup-bloat.sh',
    'cleanup-now.bat',
    'force-push-github.bat',
    'force-push-github.sh',
    'rebrand-status.bat',
    'rebrand-complete.sh',
    'git-release.bat',
    'DEPLOY_READY.md',
    'RENDER_IMPLEMENTATION.md',
    'CLEANUP_README.md',
    'REBRANDING_COMPLETE.md',
    'GITHUB_RELEASE_INSTRUCTIONS.md',
    'URL_UPDATE_SUMMARY.md',
    'test-render-changes.sh'
]

deleted = []
not_found = []

for file in files_to_delete:
    file_path = os.path.join('d:', os.sep, 'gmap-new', file)
    if os.path.exists(file_path):
        try:
            os.remove(file_path)
            deleted.append(file)
            print(f'Deleted: {file}')
        except Exception as e:
            print(f'Error deleting {file}: {e}')
    else:
        not_found.append(file)
        print(f'Not found: {file}')

print(f'\n=== Summary ===')
print(f'Successfully deleted: {len(deleted)} files')
print(f'Not found: {len(not_found)} files')
