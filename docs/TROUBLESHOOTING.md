# Troubleshooting

Start with:

```sh
playanything doctor          # mpv found? which GPU decoders? config paths? tools?
playanything info <file>     # what is this file and what would happen
PLAYANYTHING_DEBUG=1 playanything <file>   # print the exact mpv command line
```

## "mpv not found"

PlayAnything is a launcher; mpv does the playing. Install it:

- Windows: `winget install -e --id shinchiro.mpv` (or re-run `install.ps1`,
  which also falls back to scoop/choco/portable download)
- macOS: `brew install mpv`
- Linux: `sudo apt install mpv`, `sudo dnf install mpv`, `sudo pacman -S mpv`,
  or `flatpak install flathub io.mpv.Mpv`

If mpv lives somewhere unusual: `playanything config mpv_path /path/to/mpv`.

## Video stutters / high CPU

Check GPU decoding is active: press `i` while playing; the stats page shows
`Hardware decoding: d3d11va / nvdec / vaapi / videotoolbox` or `no`.

- Windows + NVIDIA: install the current Game Ready / Studio driver. D3D11VA is
  the default and works for H.264/HEVC/VP9/AV1. To force NVDEC add
  `hwdec=nvdec` to `user.conf`.
- Windows + Intel/AMD: D3D11VA covers it; make sure the file's codec is
  supported by your GPU generation (10-bit HEVC needs Skylake+/Polaris+; AV1
  needs Arc/RTX 30/RDNA 2 or newer).
- Linux: NVIDIA → `nvdec` (needs the proprietary driver and `libnvidia-decode`);
  Intel/AMD → `vaapi` (install `intel-media-driver` / `mesa-va-drivers`).
  Flatpak mpv needs the matching freedesktop VAAPI extension.
- macOS: VideoToolbox handles H.264/HEVC/ProRes (and AV1 on M3+).
- Ctrl+h toggles GPU decoding at runtime to compare.

Footage with no hardware path (8K ProRes 4444 on a laptop, MJPEG, Cineform)
decodes in software; the `pa-huge` auto-profile switches to cheaper scaling
for ≥5000 px material.

## Colours look washed out / HDR looks grey

mpv tone-maps HDR to SDR on SDR displays (`tone-mapping=auto`). On an HDR
display enable HDR in Windows settings; `target-colorspace-hint=yes` then
passes HDR through. Try `tone-mapping=bt.2390` or `hable` in `user.conf` for a
different look.

## No preview for a RAW photo

`playanything info photo.ARW` tells you what was found. Files that genuinely
contain no JPEG (rare; some DNGs written by converters with "no preview") need
`--raw-full` and an installed developer (`darktable-cli` / `rawtherapee-cli`).

## Audio track is wrong

Press `a` to cycle audio tracks, Ctrl+a to list them. To always prefer a
language: `alang=en,eng` in `user.conf`. External `.wav`/`.flac` with the same
base name is picked up automatically (`audio-file-auto=fuzzy`).

## Explorer still shows the old player on double-click

Windows protects the user's default-app choice; installers cannot change it.
Either right-click → *Play with PlayAnything* (always available), or
right-click → *Open with* → *Choose another app* → PlayAnything → tick
*Always*, or Settings → Apps → Default apps → PlayAnything → *Set default*.

## macOS: "PlayAnything.app can't be opened"

The app bundle is a tiny AppleScript wrapper made on your machine by the
installer, so it is not notarized. Right-click → Open once, or
`xattr -d com.apple.quarantine ~/Applications/PlayAnything.app`.

## The background player

`playanything config daemon never` disables it; `always` starts it on demand;
`auto` (default) uses it only when already running. `playanything stop` ends it.
Logs: `systemctl --user status playanything` (Linux), `/tmp/playanything-daemon.log`
(macOS), or run `playanything daemon` in a terminal to watch it.

## Resetting

`playanything setup --force` rewrites the managed `mpv.conf`, `input.conf` and
scripts (your `user.conf` is kept). Delete the cache directory shown by `doctor`
to clear RAW previews and R3D proxies.
