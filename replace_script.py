#!/usr/bin/env python3
import os
import sys

def replace_in_file(filepath):
    """Replace dhananjay-pareek with gosom in a file"""
    try:
        with open(filepath, 'r', encoding='utf-8', errors='ignore') as f:
            content = f.read()
        
        if 'dhananjay-pareek' not in content:
            return False
            
        new_content = content.replace('dhananjay-pareek', 'gosom')
        with open(filepath, 'w', encoding='utf-8') as f:
            f.write(new_content)
        return True
    except Exception as e:
        print(f'Error processing {filepath}: {e}', file=sys.stderr)
        return False

def main():
    root_dir = r'd:\gmap-new'
    extensions = ('.go', '.mod', '.md', '.yaml', '.yml', '.bat', '.sum')
    count = 0
    
    for root, dirs, files in os.walk(root_dir):
        for file in files:
            if file.endswith(extensions):
                filepath = os.path.join(root, file)
                if replace_in_file(filepath):
                    count += 1
                    print(f'Updated: {filepath}')
    
    print(f'\nTotal files updated: {count}')

if __name__ == '__main__':
    main()
