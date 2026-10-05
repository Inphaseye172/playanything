#!/bin/sh
# PlayAnything installer for macOS and Linux (per-user, no root needed except for mpv packages).
#
#   curl -fsSL https://raw.githubusercontent.com/inphaseye172/playanything/main/install/install.sh | sh
#
# Options (environment variables):
#   PLAYANYTHING_SERVICE=1        also run the background player at login (single window, instant opens)
#   PLAYANYTHING_VERSION=v1.2.3   install a specific release
#   PLAYANYTHING_PREFIX=$HOME/.local   where bin/playanything goes
#   PLAYANYTHING_NO_ASSOC=1       skip desktop / Finder integration
#
# Uninstall:
#   curl -fsSL https://raw.githubusercontent.com/inphaseye172/playanything/main/install/uninstall.sh | sh
set -eu

REPO="inphaseye172/playanything"
PREFIX="${PLAYANYTHING_PREFIX:-$HOME/.local}"
BIN="$PREFIX/bin"
EXE="$BIN/playanything"

step() { printf '\n\033[36m> %s\033[0m\n' "$*"; }
ok()   { printf '  \033[32m[ok]\033[0m %s\n' "$*"; }
warn() { printf '  \033[33m[!!]\033[0m %s\n' "$*"; }
die()  { printf '  \033[31m[error]\033[0m %s\n' "$*" >&2; exit 1; }
have() { command -v "$1" >/dev/null 2>&1; }

OS=$(uname -s)
case "$OS" in
  Darwin) GOOS=darwin ;;
  Linux)  GOOS=linux ;;
  *) die "unsupported OS: $OS (use install.ps1 on Windows)" ;;
esac
case "$(uname -m)" in
  x86_64|amd64) GOARCH=amd64 ;;
  aarch64|arm64) GOARCH=arm64 ;;
  *) die "unsupported CPU: $(uname -m)" ;;
esac

fetch() { # url dest
  if have curl; then curl -fsSL --retry 3 -o "$2" "$1"
  elif have wget; then wget -q -O "$2" "$1"
  else die "need curl or wget"; fi
}

# Run a privileged command; works when the script is piped into sh.
as_root() {
  if [ "$(id -u)" = 0 ]; then "$@"
  elif have sudo; then
    # stdin is this script when piped into sh; let sudo prompt on the terminal
    # shellcheck disable=SC2024
    if [ -r /dev/tty ]; then sudo "$@" </dev/tty; else sudo -n "$@"; fi
  elif have doas; then doas "$@"
  else return 1; fi
}

printf 'PlayAnything installer\nInstalling to %s\n' "$BIN"
mkdir -p "$BIN"

# ------------------------------------------------------------------ 1. launcher
ASSET="playanything-${GOOS}-${GOARCH}"
if [ -n "${PLAYANYTHING_VERSION:-}" ]; then
  URL="https://github.com/$REPO/releases/download/${PLAYANYTHING_VERSION}/$ASSET"
else
  URL="https://github.com/$REPO/releases/latest/download/$ASSET"
fi
step "Downloading PlayAnything ($ASSET)"
if fetch "$URL" "$EXE.tmp" 2>/dev/null && [ "$(wc -c <"$EXE.tmp")" -gt 1000000 ]; then
  chmod +x "$EXE.tmp" && mv "$EXE.tmp" "$EXE"
  ok "launcher: $EXE"
else
  rm -f "$EXE.tmp"
  warn "release download failed ($URL)"
  if have go; then
    step "Building from source with Go"
    GOBIN="$BIN" CGO_ENABLED=0 go install -ldflags '-s -w' "github.com/$REPO/cmd/playanything@main"
    [ -x "$EXE" ] || die "go install did not produce $EXE"
    ok "built $EXE"
  else
    die "could not download a release and Go is not installed; see https://github.com/$REPO/releases"
  fi
fi

# ------------------------------------------------------------------ 2. mpv
step "Checking for mpv (the playback engine)"
MPV=""
if have mpv; then MPV=$(command -v mpv)
elif [ -x /opt/homebrew/bin/mpv ]; then MPV=/opt/homebrew/bin/mpv
elif [ -x /usr/local/bin/mpv ]; then MPV=/usr/local/bin/mpv
elif [ -x /Applications/mpv.app/Contents/MacOS/mpv ]; then MPV=/Applications/mpv.app/Contents/MacOS/mpv
elif have flatpak && flatpak info io.mpv.Mpv >/dev/null 2>&1; then MPV="flatpak:io.mpv.Mpv"
fi

if [ -z "$MPV" ]; then
  if [ "$GOOS" = darwin ]; then
    BREW=$(command -v brew 2>/dev/null || true)
    [ -n "$BREW" ] || { [ -x /opt/homebrew/bin/brew ] && BREW=/opt/homebrew/bin/brew; } || true
    [ -n "$BREW" ] || { [ -x /usr/local/bin/brew ] && BREW=/usr/local/bin/brew; } || true
    if [ -n "$BREW" ]; then
      step "Installing mpv with Homebrew"
      "$BREW" install mpv </dev/tty >/dev/null 2>&1 || "$BREW" install mpv
    else
      warn "Homebrew not found. Install mpv from https://mpv.io/installation/ (e.g. the mpv.app bundle into /Applications), or install Homebrew first: https://brew.sh"
    fi
  else
    step "Installing mpv with your package manager"
    if   have apt-get; then as_root sh -c 'apt-get update -qq && apt-get install -y -qq mpv' || warn "apt failed"
    elif have dnf;     then as_root dnf install -y mpv || warn "dnf failed (enable RPM Fusion for full codec support)"
    elif have pacman;  then as_root pacman -S --noconfirm --needed mpv || warn "pacman failed"
    elif have zypper;  then as_root zypper --non-interactive install mpv || warn "zypper failed"
    elif have apk;     then as_root apk add mpv || warn "apk failed"
    elif have xbps-install; then as_root xbps-install -y mpv || warn "xbps failed"
    elif have emerge;  then as_root emerge media-video/mpv || warn "emerge failed"
    elif have nix-env; then nix-env -iA nixpkgs.mpv || warn "nix failed"
    fi
    if ! have mpv && have flatpak; then
      step "Installing mpv from Flathub (flatpak)"
      if flatpak install -y --user flathub io.mpv.Mpv; then MPV="flatpak:io.mpv.Mpv"; else warn "flatpak failed"; fi
    fi
  fi
  if have mpv; then MPV=$(command -v mpv)
  elif [ -x /opt/homebrew/bin/mpv ]; then MPV=/opt/homebrew/bin/mpv
  elif [ -x /usr/local/bin/mpv ]; then MPV=/usr/local/bin/mpv
  fi
fi
[ -n "$MPV" ] || die "mpv is not installed. Install it (https://mpv.io/installation/) and re-run this script."
ok "mpv: $MPV"

# ------------------------------------------------------------------ 3. PATH + profile
step "Configuring"
case ":$PATH:" in
  *":$BIN:"*) ;;
  *)
    LINE="export PATH=\"$BIN:\$PATH\"  # added by PlayAnything"
    touched=0
    for rc in "$HOME/.profile" "$HOME/.bashrc" "$HOME/.zshrc" "$HOME/.zprofile"; do
      if [ -f "$rc" ]; then
        touched=1
        grep -Fq "# added by PlayAnything" "$rc" || printf '\n%s\n' "$LINE" >>"$rc"
      fi
    done
    if [ "$touched" = 0 ]; then
      # No rc file yet: create the one the login shell reads (zsh on macOS).
      case "$(basename "${SHELL:-sh}")" in zsh) printf '%s\n' "$LINE" >>"$HOME/.zprofile" ;; *) printf '%s\n' "$LINE" >>"$HOME/.profile" ;; esac
    fi
    export PATH="$BIN:$PATH"
    ok "added $BIN to PATH in your shell profile (open a new terminal)"
    ;;
esac
"$EXE" setup | sed 's/^/  /'
case "$MPV" in
  flatpak:*) ;; # auto-detected by playanything
  *) if ! have mpv; then "$EXE" config mpv_path "$MPV" >/dev/null; ok "mpv path saved"; fi ;;
esac
[ "${PLAYANYTHING_SERVICE:-0}" = 1 ] && "$EXE" config daemon always >/dev/null

# ------------------------------------------------------------------ 4. desktop integration
if [ "${PLAYANYTHING_NO_ASSOC:-0}" != 1 ]; then
  if [ "$GOOS" = linux ]; then
    step "Registering with your desktop (freedesktop .desktop + MIME types)"
    APPS="${XDG_DATA_HOME:-$HOME/.local/share}/applications"
    ICONS="${XDG_DATA_HOME:-$HOME/.local/share}/icons/hicolor/scalable/apps"
    mkdir -p "$APPS" "$ICONS"
    MIMES=$("$EXE" mimetypes | tr ' ' ';')
    cat >"$APPS/playanything.desktop" <<DESKTOP
[Desktop Entry]
Type=Application
Version=1.0
Name=PlayAnything
GenericName=Media Player
Comment=One click, any media: video, audio, photos, camera RAW - local, NAS or cloud
Exec="$EXE" %U
TryExec=$EXE
Icon=playanything
Terminal=false
Categories=AudioVideo;Audio;Video;Player;Graphics;Viewer;
Keywords=player;video;audio;photo;raw;mkv;mp4;flac;hdr;10bit;nas;synology;
StartupNotify=false
Actions=append;
MimeType=$MIMES;

[Desktop Action append]
Name=Add to PlayAnything playlist
Exec="$EXE" --append %U
DESKTOP
    cat >"$ICONS/playanything.svg" <<'SVG'
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 256 256" width="256" height="256"><defs><linearGradient id="g" x1="0" y1="0" x2="1" y2="1"><stop offset="0" stop-color="#1e2a3a"/><stop offset="1" stop-color="#0b1220"/></linearGradient></defs><rect x="16" y="16" width="224" height="224" rx="52" fill="url(#g)"/><circle cx="128" cy="128" r="82" fill="none" stroke="#3ddc97" stroke-width="10" opacity="0.9"/><path d="M106 86 L170 128 L106 170 Z" fill="#ffffff"/></svg>
SVG
    if have update-desktop-database; then update-desktop-database "$APPS" 2>/dev/null || true; fi
    if have gtk-update-icon-cache; then gtk-update-icon-cache -q -t "${XDG_DATA_HOME:-$HOME/.local/share}/icons/hicolor" 2>/dev/null || true; fi
    if have xdg-mime; then
      # shellcheck disable=SC2046
      if xdg-mime default playanything.desktop $("$EXE" mimetypes) 2>/dev/null; then
        ok "PlayAnything is now the default for $(echo "$MIMES" | tr ';' '\n' | wc -l | tr -d ' ') MIME types"
      else
        warn "xdg-mime could not set defaults; use your file manager's 'Open With > Set as default'"
      fi
    fi
    ok "menu entry + right-click 'Open With' available"
  else
    step "Creating PlayAnything.app for Finder (double-click / Open With)"
    APPDIR="$HOME/Applications"; mkdir -p "$APPDIR"
    APP="$APPDIR/PlayAnything.app"
    TMPD=$(mktemp -d -t playanything.XXXXXX)
    TMPSCPT="$TMPD/PlayAnything.applescript"
    cat >"$TMPSCPT" <<APPLESCRIPT
on open theItems
	set args to ""
	repeat with f in theItems
		set args to args & " " & quoted form of POSIX path of f
	end repeat
	do shell script quoted form of "$EXE" & args & " >/dev/null 2>&1 &"
end open
on run
	do shell script quoted form of "$EXE" & " >/dev/null 2>&1 &"
end run
APPLESCRIPT
    rm -rf "$APP"
    if osacompile -o "$APP" "$TMPSCPT" 2>/dev/null; then
      PL="$APP/Contents/Info.plist"
      PB=/usr/libexec/PlistBuddy
      $PB -c "Set :CFBundleIdentifier com.playanything.app" "$PL" >/dev/null 2>&1 || $PB -c "Add :CFBundleIdentifier string com.playanything.app" "$PL"
      $PB -c "Set :CFBundleName PlayAnything" "$PL" >/dev/null 2>&1 || $PB -c "Add :CFBundleName string PlayAnything" "$PL"
      $PB -c "Add :LSUIElement bool true" "$PL" >/dev/null 2>&1 || true
      $PB -c "Add :CFBundleDocumentTypes array" "$PL" >/dev/null 2>&1 || true
      $PB -c "Add :CFBundleDocumentTypes:0 dict" "$PL" \
          -c "Add :CFBundleDocumentTypes:0:CFBundleTypeName string Media" \
          -c "Add :CFBundleDocumentTypes:0:CFBundleTypeRole string Viewer" \
          -c "Add :CFBundleDocumentTypes:0:LSHandlerRank string Alternate" \
          -c "Add :CFBundleDocumentTypes:0:LSItemContentTypes array" \
          -c "Add :CFBundleDocumentTypes:0:LSItemContentTypes: string public.movie" \
          -c "Add :CFBundleDocumentTypes:0:LSItemContentTypes: string public.audio" \
          -c "Add :CFBundleDocumentTypes:0:LSItemContentTypes: string public.image" \
          -c "Add :CFBundleDocumentTypes:0:LSItemContentTypes: string public.camera-raw-image" \
          -c "Add :CFBundleDocumentTypes:0:LSItemContentTypes: string com.adobe.raw-image" \
          -c "Add :CFBundleDocumentTypes:0:CFBundleTypeExtensions array" "$PL" >/dev/null
      for e in $("$EXE" extensions); do
        $PB -c "Add :CFBundleDocumentTypes:0:CFBundleTypeExtensions: string $e" "$PL" >/dev/null
      done
      /System/Library/Frameworks/CoreServices.framework/Frameworks/LaunchServices.framework/Support/lsregister -f "$APP" 2>/dev/null || true
      ok "$APP (right-click a file > Open With > PlayAnything; 'Change All…' in Get Info makes it default)"
      if have duti; then
        for e in $("$EXE" extensions); do duti -s com.playanything.app ".$e" all 2>/dev/null || true; done
        ok "set as default handler with duti"
      fi
    else
      warn "osacompile failed; skipping the .app bundle (the command line still works)"
    fi
    rm -rf "$TMPD"
  fi
fi

# ------------------------------------------------------------------ 5. background player
if [ "${PLAYANYTHING_SERVICE:-0}" = 1 ]; then
  step "Enabling the background player at login"
  if [ "$GOOS" = linux ] && have systemctl; then
    UNITDIR="${XDG_CONFIG_HOME:-$HOME/.config}/systemd/user"; mkdir -p "$UNITDIR"
    cat >"$UNITDIR/playanything.service" <<UNIT
[Unit]
Description=PlayAnything background player (idle mpv with IPC)
After=graphical-session.target
PartOf=graphical-session.target

[Service]
Type=simple
ExecStart=$EXE daemon
ExecStop=$EXE stop
Restart=on-failure
RestartSec=2

[Install]
WantedBy=graphical-session.target
UNIT
    if systemctl --user daemon-reload && systemctl --user enable --now playanything.service; then
      ok "systemd user service enabled (systemctl --user status playanything)"
    else
      warn "systemctl --user failed; start it manually with: $EXE daemon &"
    fi
  elif [ "$GOOS" = darwin ]; then
    LA="$HOME/Library/LaunchAgents"; mkdir -p "$LA"
    PLIST="$LA/com.playanything.daemon.plist"
    cat >"$PLIST" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key><string>com.playanything.daemon</string>
  <key>ProgramArguments</key><array><string>$EXE</string><string>daemon</string></array>
  <key>RunAtLoad</key><true/>
  <key>KeepAlive</key><dict><key>SuccessfulExit</key><false/></dict>
  <key>ProcessType</key><string>Interactive</string>
  <key>StandardOutPath</key><string>/tmp/playanything-daemon.log</string>
  <key>StandardErrorPath</key><string>/tmp/playanything-daemon.log</string>
</dict>
</plist>
PLIST
    launchctl bootout "gui/$(id -u)" "$PLIST" 2>/dev/null || true
    launchctl bootstrap "gui/$(id -u)" "$PLIST" 2>/dev/null || launchctl load "$PLIST"
    ok "launchd agent installed ($PLIST)"
  else
    warn "no systemd/launchd; start the background player yourself: $EXE daemon &"
  fi
fi

# ------------------------------------------------------------------ 6. report
step "Checking the installation"
"$EXE" doctor | sed 's/^/  /' || true
printf '\n\033[32mDone.\033[0m  Try:  playanything <file|folder|url>\n\n'
