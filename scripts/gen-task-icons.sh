#!/bin/bash
# gen-task-icons.sh — (re)generate the task-runner icons in assets/icons/ from an
# installed IntelliJ IDEA.
#
# Uses the IDE's own tool icons (JetBrains-themed, Apache-2.0 licensed — see
# THIRD-PARTY-NOTICES.md), rasterized to 256px: the run/execute arrow for the
# runtask keyword, plus npm / gradle / maven. Runners without an IDE icon
# (make, just, go-task) fall back to the run icon at display time, so only these
# four files are produced.
#
# Requires rsvg-convert (brew install librsvg) and IntelliJ IDEA installed.
set -euo pipefail

REPO="$(cd "$(dirname "$0")/.." && pwd)"
OUT="$REPO/assets/icons"
mkdir -p "$OUT"
command -v rsvg-convert >/dev/null || { echo "need rsvg-convert (brew install librsvg)"; exit 1; }

# Locate the newest installed IntelliJ IDEA.
APP=""
for dir in "$HOME/Applications" /Applications; do
  cand="$(ls -d "$dir/IntelliJ IDEA"*.app 2>/dev/null | sort -V | tail -1 || true)"
  [ -n "$cand" ] && APP="$cand" && break
done
[ -n "$APP" ] || { echo "IntelliJ IDEA.app not found in ~/Applications or /Applications"; exit 1; }
echo "extracting from $APP"

# extract <jar-dir> <entry-regex> <out-name> [sed-recolor]: find the first .jar
# under jar-dir that contains an SVG entry matching the regex, optionally recolor
# it (the IDE ships gradle/maven as theme-grey glyphs), and rasterize it. The dir
# is quoted and globbed locally so paths with spaces work.
extract() {
  local dir="$1" re="$2" out="$3" recolor="${4:-}" jar entry svg
  shopt -s nullglob
  for jar in "$dir"/*.jar; do
    entry="$(unzip -l "$jar" 2>/dev/null | grep -oE '[A-Za-z0-9_/]+\.svg' | grep -E "$re" | head -1 || true)"
    if [ -n "$entry" ]; then
      svg="$(unzip -p "$jar" "$entry")"
      [ -n "$recolor" ] && svg="$(printf '%s' "$svg" | sed -E "$recolor")"
      printf '%s' "$svg" | rsvg-convert -w 256 -h 256 -o "$OUT/$out.png" \
        && { echo "  $out.png  <- $(basename "$jar")::$entry"; shopt -u nullglob; return 0; }
    fi
  done
  shopt -u nullglob
  echo "  $out: NOT FOUND (skipped)"
}

# npm and the run arrow already ship in colour; gradle/maven are theme-grey, so
# recolour them to their brand colours (Gradle teal; Maven the JetBrains Maven
# blue #3574F0 the IDE uses for Maven build icons).
extract "$APP/Contents/lib"                           'actions/execute\.svg$'  run
extract "$APP/Contents/plugins/javascript-plugin/lib" 'fileTypes/npm\.svg$'    npm
extract "$APP/Contents/plugins/gradle/lib"            'icons/gradle\.svg$'     gradle 's/#6E6E6E/#1BA8CB/Ig'
extract "$APP/Contents/plugins/maven/lib"             'toolwindow/maven\.svg$' maven  's/#6C707E/#3574F0/Ig'

# Language-runner icons whose logos ship in IDEs not installed here (GoLand,
# RustRover, PhpStorm, RubyMine, Rider) are pulled from the public IntelliJ Icons
# catalog (https://intellij-icons.jetbrains.design). Named by runner: composer
# uses the PHP mark, rake the Ruby mark. (deno has no catalog icon → run arrow.)
CATALOG="https://intellij-icons.jetbrains.design/icons"
fetch() { # <catalog-path> <out-name>
  local svg
  svg="$(curl -fsS -m 20 "$CATALOG/$1" 2>/dev/null)" || { echo "  $2: FETCH FAILED ($1)"; return; }
  printf '%s' "$svg" | rsvg-convert -w 256 -h 256 -o "$OUT/$2.png" \
    && echo "  $2.png  <- catalog/$1"
}
fetch "GoGeneratedIcons/icons/go.svg"        go
fetch "RustroverCommonIcons/icons/cargo.svg" cargo
fetch "PhpIcons/icons/expui/php.svg"         composer
fetch "RubyIcons/icons/ruby/ruby.svg"        rake
fetch "NetIcons10/DotNet(Color).svg"         dotnet

# Drop any stale per-runner tiles so make/just/task/deno fall back to the run icon.
rm -f "$OUT"/make.png "$OUT"/just.png "$OUT"/task.png "$OUT"/deno.png

echo "done → $OUT"
