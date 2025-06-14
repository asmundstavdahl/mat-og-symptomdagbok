#!/bin/bash

(
find -name "*".go
find -name "*".html
find -name "*".js
find -name "*".css
find -name "*".md
find -name "*".txt
find -name "*".json

ls go.mod
) \
| grep -vF -e .tmp -e .git \
| grep -v "\.aider.*"
