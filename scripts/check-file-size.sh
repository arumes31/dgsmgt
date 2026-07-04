#!/usr/bin/env sh
# Fails when a source file exceeds MAX_LINES — keeps monolith files out of the
# repo. Split by topic instead (see CLAUDE.md "File size limit").
set -eu

MAX_LINES="${MAX_LINES:-500}"
fail=0

# shellcheck disable=SC2046
for f in $(git ls-files 'internal/**/*.go' 'cmd/**/*.go' 'static/**/*.js' 'static/**/*.html' 'static/**/*.css'); do
    case "$f" in
        legacy_tests/*) continue ;;
    esac
    lines=$(wc -l < "$f")
    if [ "$lines" -gt "$MAX_LINES" ]; then
        echo "FAIL: $f has $lines lines (max $MAX_LINES) — split it by topic"
        fail=1
    fi
done

if [ "$fail" -eq 1 ]; then
    echo ""
    echo "File size check failed. Split large files into focused modules"
    echo "(e.g. one handler group / UI section per file)."
    exit 1
fi
echo "File size check OK (max $MAX_LINES lines)"
