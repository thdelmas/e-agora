#!/usr/bin/env bash
#
# QA gate: enforce file dimension limits across hand-written source and docs.
#
#   error    file has more than MAX_LINES lines
#   error    any line is wider than MAX_COLS characters (a tab counts as 1)
#   warning  file has more than WARN_LINES lines (but within MAX_LINES)
#
# Scope: tracked Go / JS / Vue / CSS / Markdown / HTML files. Generated and
# vendored files (go.sum, package-lock.json) and data blobs (*.json) fall
# outside the extension allowlist and are never checked.
#
# Markdown (*.md) is checked for line count ONLY — its tables, ASCII diagrams,
# and links can't be hard-wrapped at a column, so the width rule would only
# punish content that has no break point. Code files get both rules.
#
# A code line that genuinely cannot wrap (e.g. a single long URL) may carry the
# marker `qa:allow-long` anywhere on it to opt that one line out of the width
# check. Use sparingly — it's an escape hatch, not a habit.
#
# Exit status is non-zero when any error is found; warnings alone pass.
#
# Tune the limits with env vars, e.g.  MAX_COLS=100 scripts/check-file-size.sh
set -euo pipefail

MAX_LINES="${MAX_LINES:-500}"
WARN_LINES="${WARN_LINES:-250}"
MAX_COLS="${MAX_COLS:-80}"

# Extensions we treat as hand-written source or docs.
EXT_RE='\.(go|js|mjs|cjs|vue|css|md|html)$'

cd "$(git rev-parse --show-toplevel)"

errors=0
warnings=0

# -z / read -d '' keeps paths with spaces or newlines intact.
while IFS= read -r -d '' file; do
	# One perl pass yields: line count, widest line, and that line's number.
	# -CSD decodes UTF-8 so length() counts characters, not bytes, and a tab
	# counts as a single character (mawk, the default awk here, can't do this).
	# Lines carrying the qa:allow-long marker are skipped when finding the widest.
	read -r lines maxcol maxline < <(perl -CSD -ne '
		chomp;
		next if /qa:allow-long/;
		my $l = length;
		if ($l > $m) { $m = $l; $ml = $. }
		END { print(($. + 0), " ", ($m + 0), " ", ($ml + 0), "\n") }
	' "$file")

	if [ "$lines" -gt "$MAX_LINES" ]; then
		printf 'ERROR   %s: %d lines (max %d)\n' "$file" "$lines" "$MAX_LINES"
		errors=$((errors + 1))
	elif [ "$lines" -gt "$WARN_LINES" ]; then
		printf 'WARNING %s: %d lines (warn above %d, max %d)\n' \
			"$file" "$lines" "$WARN_LINES" "$MAX_LINES"
		warnings=$((warnings + 1))
	fi

	# The width rule is for code; Markdown gets the line-count rule only.
	if [ "${file##*.}" != md ] && [ "$maxcol" -gt "$MAX_COLS" ]; then
		printf 'ERROR   %s:%d: line is %d columns (max %d)\n' \
			"$file" "$maxline" "$maxcol" "$MAX_COLS"
		errors=$((errors + 1))
	fi
done < <(git ls-files -z | grep -zE "$EXT_RE")

printf -- '----\n'
printf '%d error(s), %d warning(s)\n' "$errors" "$warnings"

[ "$errors" -eq 0 ]
