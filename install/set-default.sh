#!/bin/sh
# Make PlayAnything the default app for every file type it supports (current user).
#   curl -fsSL https://raw.githubusercontent.com/inphaseye172/playanything/main/install/set-default.sh | sh
# Linux: xdg-mime defaults for all registered MIME types.
# macOS: uses duti (brew install duti) to set com.playanything.app for every extension.
# Env: PLAYANYTHING_UNSET=1 to undo (Linux only; on macOS pick another app in Get Info).
set -eu
EXE="${PLAYANYTHING_PREFIX:-$HOME/.local}/bin/playanything"
command -v playanything >/dev/null 2>&1 && EXE=$(command -v playanything)
[ -x "$EXE" ] || { echo "PlayAnything is not installed; run install.sh first" >&2; exit 1; }

case "$(uname -s)" in
  Darwin)
    APP="$HOME/Applications/PlayAnything.app"
    [ -d "$APP" ] || { echo "$APP not found; run install.sh first" >&2; exit 1; }
    if ! command -v duti >/dev/null 2>&1; then
      if command -v brew >/dev/null 2>&1; then brew install duti; else
        echo "duti is needed to change defaults from a script: brew install duti" >&2
        echo "Or: right-click a file > Get Info > Open with > PlayAnything > Change All..." >&2
        exit 1
      fi
    fi
    n=0
    for e in $("$EXE" extensions); do duti -s com.playanything.app ".$e" all 2>/dev/null && n=$((n+1)) || true; done
    echo "PlayAnything is now the default for $n extensions."
    ;;
  Linux)
    command -v xdg-mime >/dev/null 2>&1 || { echo "xdg-mime not found (install xdg-utils)" >&2; exit 1; }
    if [ "${PLAYANYTHING_UNSET:-0}" = 1 ]; then
      f="${XDG_CONFIG_HOME:-$HOME/.config}/mimeapps.list"
      [ -f "$f" ] && sed -i.bak '/=playanything.desktop/d' "$f" && echo "removed PlayAnything defaults from $f"
      exit 0
    fi
    # shellcheck disable=SC2046
    xdg-mime default playanything.desktop $("$EXE" mimetypes)
    echo "PlayAnything is now the default for $("$EXE" mimetypes | wc -w | tr -d ' ') MIME types."
    ;;
  *) echo "unsupported OS" >&2; exit 1 ;;
esac
