#!/bin/bash
# stage-icons.sh <bundle-dir>
#
# Copies the vendored brand-site icons (assets/icons/*.png) into
# <bundle-dir>/icons/ and sets <bundle-dir>/icon.png (the workflow icon).
#
# Installed IDEs render their own icon at runtime via Alfred's fileicon, so
# nothing is extracted from app bundles here — these files are only the
# not-installed fallback and the Alfred editor's canvas icons.
set -euo pipefail

BUNDLE="${1:?usage: stage-icons.sh <bundle-dir>}"
REPO="$(cd "$(dirname "$0")/.." && pwd)"
ICONS="$BUNDLE/icons"
mkdir -p "$ICONS"

# Start from a clean set so icons removed from assets/ don't linger in the bundle.
rm -f "$ICONS"/*.png

# Vendored brand-site icons.
if compgen -G "$REPO/assets/icons/*.png" >/dev/null; then
  cp "$REPO"/assets/icons/*.png "$ICONS"/ 2>/dev/null || true
fi

# default.png fallback, in case the vendored set lacks one.
if [ ! -f "$ICONS/default.png" ]; then
  if [ -f "$ICONS/idea.png" ]; then
    cp "$ICONS/idea.png" "$ICONS/default.png"
  else
    first="$(ls "$ICONS"/*.png 2>/dev/null | head -1 || true)"
    [ -n "$first" ] && cp "$first" "$ICONS/default.png"
  fi
fi

# Workflow icon (shown in Alfred preferences): the vendored JetBrains Toolbox icon.
if [ -f "$REPO/assets/icon.png" ]; then
  cp "$REPO/assets/icon.png" "$BUNDLE/icon.png"
elif [ -f "$ICONS/default.png" ]; then
  cp "$ICONS/default.png" "$BUNDLE/icon.png"
fi

echo "staged $(ls "$ICONS"/*.png 2>/dev/null | wc -l | tr -d ' ') icon(s) into $ICONS"
