#!/bin/sh
# Removes PlayAnything (per-user). mpv stays installed.
#   curl -fsSL https://raw.githubusercontent.com/inphaseye172/playanything/main/install/uninstall.sh | sh
# Set PLAYANYTHING_PURGE=1 to also delete config and caches.
set -u
PREFIX="${PLAYANYTHING_PREFIX:-$HOME/.local}"
EXE="$PREFIX/bin/playanything"
[ -x "$EXE" ] && "$EXE" stop >/dev/null 2>&1
pkill -f "playanything daemon" 2>/dev/null || true

if [ "$(uname -s)" = Linux ]; then
  systemctl --user disable --now playanything.service 2>/dev/null || true
  rm -f "${XDG_CONFIG_HOME:-$HOME/.config}/systemd/user/playanything.service"
  systemctl --user daemon-reload 2>/dev/null || true
  rm -f "${XDG_DATA_HOME:-$HOME/.local/share}/applications/playanything.desktop"
  rm -f "${XDG_DATA_HOME:-$HOME/.local/share}/icons/hicolor/scalable/apps/playanything.svg"
  command -v update-desktop-database >/dev/null 2>&1 && update-desktop-database "${XDG_DATA_HOME:-$HOME/.local/share}/applications" 2>/dev/null
  CFG="${XDG_CONFIG_HOME:-$HOME/.config}/playanything"; CACHE="${XDG_CACHE_HOME:-$HOME/.cache}/playanything"
else
  PLIST="$HOME/Library/LaunchAgents/com.playanything.daemon.plist"
  launchctl bootout "gui/$(id -u)" "$PLIST" 2>/dev/null || launchctl unload "$PLIST" 2>/dev/null || true
  rm -f "$PLIST"
  rm -rf "$HOME/Applications/PlayAnything.app"
  CFG="$HOME/Library/Application Support/PlayAnything"; CACHE="$HOME/Library/Caches/PlayAnything"
fi
rm -f "$EXE"
for rc in "$HOME/.profile" "$HOME/.bashrc" "$HOME/.zshrc"; do
  [ -f "$rc" ] && grep -v "# added by PlayAnything" "$rc" >"$rc.tmp" && mv "$rc.tmp" "$rc"
done
if [ "${PLAYANYTHING_PURGE:-0}" = 1 ]; then
  rm -rf "$CFG" "$CACHE"
  echo "PlayAnything removed, including config and caches."
else
  echo "PlayAnything removed. Config kept at: $CFG"
  echo "Cache kept at:  $CACHE   (set PLAYANYTHING_PURGE=1 to delete them too)"
fi
