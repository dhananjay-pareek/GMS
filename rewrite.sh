#!/bin/bash
export FILTER_BRANCH_SQUELCH_WARNING="1"
git filter-branch -f --env-filter '
if [ "$GIT_AUTHOR_NAME" = "Dhananjay" ] || [ "$GIT_AUTHOR_NAME" = "Giorgos Komninos" ] || [ "$GIT_AUTHOR_NAME" = "dhananjay-pareek" ]; then
    export GIT_AUTHOR_NAME="Dhananjay"
    export GIT_AUTHOR_EMAIL="83391605+Dhananjay-Pareek@users.noreply.github.com"
fi
if [ "$GIT_COMMITTER_NAME" = "Dhananjay" ] || [ "$GIT_COMMITTER_NAME" = "Giorgos Komninos" ] || [ "$GIT_COMMITTER_NAME" = "dhananjay-pareek" ]; then
    export GIT_COMMITTER_NAME="Dhananjay"
    export GIT_COMMITTER_EMAIL="83391605+Dhananjay-Pareek@users.noreply.github.com"
fi
' --tag-name-filter cat -- --branches --tags
