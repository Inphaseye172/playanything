<h1 align="center">PlayAnything</h1>

<p align="center"><b>One click, any media.</b><br>
A native, lightweight player from <b>Anchor Point Studio</b> that opens <i>every</i> video, audio, photo and camera-RAW file —
on your disk, on your NAS, or sitting as a cloud placeholder in Synology Drive / OneDrive — in its own window, instantly, GPU accelerated.</p>

<p align="center">
<code>MKV</code> <code>MP4</code> <code>MOV</code> <code>MXF</code> <code>WebM</code> <code>AVI</code> <code>TS/M2TS</code> ·
<code>FLAC</code> <code>MP3</code> <code>WAV</code> <code>DSD</code> <code>Opus</code> <code>ALAC</code> ·
<code>JPG</code> <code>PNG</code> <code>HEIC</code> <code>AVIF</code> <code>EXR</code> <code>TIFF</code> ·
<code>Sony ARW</code> <code>Canon CR2/CR3</code> <code>Nikon NEF</code> <code>DNG</code> <code>Fuji RAF</code> ·
<code>RED R3D</code>* <code>BRAW</code>* ·
10-bit · HDR · multi-audio-track · NVDEC / D3D11VA / VAAPI / VideoToolbox
</p>

---

## Install (one line)

**Windows 10/11** — PowerShell, no admin rights needed:

```powershell
irm https://raw.githubusercontent.com/inphaseye172/playanything/main/install/install.ps1 | iex
```

**macOS / Linux**:

```sh
curl -fsSL https://raw.githubusercontent.com/inphaseye172/playanything/main/install/install.sh | sh
```

On Windows that installs the complete app (about 70 MB: `playanything.exe` plus its built-in playback engine), writes the
tuned player profile, and wires up your desktop: right-click any file or folder → **Play with PlayAnything**; *Open with*
for 230+ extensions; *Settings → Default apps*; Start Menu shortcut; `playanything` on your PATH. Nothing else to install.

Make it the default for **every** supported file type in one go:

```powershell
irm https://raw.githubusercontent.com/inphaseye172/playanything/main/install/set-default.ps1 | iex
```

Uninstall: same URLs with `uninstall.ps1` / `uninstall.sh`.

## What you get

- **Its own window.** Title bar, menus, a clean control bar (play, time, seek, volume, audio/subtitle track pickers, fullscreen),
  keyboard shortcuts, drag & drop, one reusable window (opening a second file goes to the running window), Esc/F fullscreen,
  always-on-top, DPI-aware. The Anchor Point Studio icon on the taskbar and in Explorer.
- **Every format.** 232 registered extensions. H.264, HEVC 8/10/12-bit (HDR10/HLG), AV1, VP9, ProRes, DNxHR, MJPEG, Cineform, MPEG-2,
  VC-1 … FLAC, ALAC, DSD, Opus, TrueHD, DTS-HD … JPEG, PNG, HEIC, AVIF, JPEG XL, EXR, TIFF, PSD … See [docs/FORMATS.md](docs/FORMATS.md).
- **GPU decoding and 10-bit/HDR output.** D3D11VA/NVDEC on Windows, VAAPI/NVDEC on Linux, VideoToolbox on macOS; HDR passthrough on HDR
  displays, tone-mapping otherwise. Press `I` to see the decoder in use.
- **Multiple audio tracks** (NVIDIA ShadowPlay separate tracks, OBS multi-track, dual-language): listed on screen when the file opens,
  `A` cycles, the **A** button on the control bar picks.
- **Cloud and NAS files that other players choke on.** Synology Drive On-demand Sync, OneDrive Files On-Demand, Dropbox online-only and
  SMB shares are detected (`RECALL_ON_DATA_ACCESS`, `SF_DATALESS`, network mounts). The window opens instantly, says it is downloading, and
  streams through a 512 MiB cache while the sync client fetches bytes. [Details](docs/CLOUD-AND-NETWORK.md).
- **Camera RAW photos open like JPEGs.** ARW, CR2, CR3, NEF, DNG, RAF, ORF, RW2, PEF, SRW, 3FR, IIQ … the camera's embedded full-size
  JPEG is extracted in-process in milliseconds, rotated per EXIF, cached. Page through a whole card with `→`. True demosaic optional
  (`--raw-full`, darktable/RawTherapee/LibRaw).
- **RED R3D and Blackmagic RAW**, honestly handled: nothing open-source can decode them. With RED's free REDline installed, PlayAnything
  renders a proxy once (progress on screen) and plays it; otherwise it hands the clip to REDCINE-X PRO or Blackmagic RAW Player, or
  tells you exactly what to install. [Details](docs/RED-R3D.md).
- **Light.** A single 5 MB Go executable plus the engine library. No runtime, no telemetry, per-user install, MIT licensed.

\* see [docs/RED-R3D.md](docs/RED-R3D.md).

## Usage

Double-click a file, right-click → *Play with PlayAnything*, drag files onto the window, or:

```sh
playanything movie.mkv                    # play (opens in the running window if one exists)
playanything ./DCIM/100MSDCF              # a whole folder, natural order
playanything "https://example.com/a.mp4"  # stream
playanything --append song.flac           # queue after the current playlist
playanything --hydrate-first big.mov      # download a cloud placeholder fully first
playanything --raw-full DSC01234.ARW      # true RAW development instead of the embedded preview
playanything -f clip.mp4                  # fullscreen

playanything info clip.mkv                # what is it? local/cloud? which tracks? what will happen?
playanything doctor                       # engine version, GPU decoders, config paths, optional tools
playanything config daemon always         # settings (see below)
```

### Keys

| Key | Action |
|---|---|
| `Space` · `←` `→` · `↑` `↓` · `.` `,` | pause · seek 5 s · seek 1 min · frame step |
| `A` `Shift+A` · `Ctrl+A` | next / previous audio track · list tracks |
| `S` `V` | next subtitle · toggle subtitles |
| `PgUp` `PgDn` | previous / next file in folder |
| `R` · `Ctrl+wheel` · `0` | rotate photo · zoom · reset |
| `F` `Enter` `Alt+Enter` · double-click · `Esc` | fullscreen · leave fullscreen |
| `Ctrl+O` · `Ctrl+Shift+O` · `Ctrl+B` | open · add to playlist · hide controls |
| `Ctrl+S` · `I` · `Ctrl+H` · `D` | screenshot (Desktop) · stats · toggle GPU decoding · deinterlace |
| `[` `]` `Backspace` · `Q` | slower / faster / normal · quit |

## How it is built

PlayAnything is a Go application with no cgo and no UI toolkit:

```
cmd/playanything          CLI + Windows entry point (GUI subsystem; attaches to your terminal when run from one)
internal/winui            native Win32 window: video surface, control bar (GDI), menus, keys, drag & drop, single instance, fullscreen
internal/player           the player: folder playlists, RAW preview hook, cloud awareness, R3D proxies, tracks, typed events
internal/engine           the embedded playback engine: libmpv (FFmpeg + libplacebo) loaded at runtime via purego
internal/rawpreview       camera-RAW embedded-JPEG extraction (TIFF/EXIF walk + format-agnostic scan), ~10 ms per 40 MB
internal/cloud            Synology/OneDrive/Dropbox placeholder + network-share detection, hydration with progress
internal/media            232 extensions, magic-byte sniffing, natural sort, MIME table
internal/red              REDline / REDCINE-X / Blackmagic RAW Player integration
internal/assets           the engine profile: mpv.conf (gpu-next, hwdec=auto-safe, HDR, big network cache), input.conf, Lua helpers
install/                  one-line installers, default-app scripts, service units
```

**About the engine.** Decoding H.264, HEVC, AV1, ProRes and the rest is the job of FFmpeg, and rendering 10-bit/HDR correctly is the
job of libplacebo. PlayAnything embeds them through **libmpv**, the library form of mpv, loaded from the app folder
(`libmpv-2.dll` on Windows). There is no separate mpv program, no mpv window and no mpv install step: the window, controls, behaviour,
icon and installer are PlayAnything's. Writing our own codecs would be years of work for a worse result, so that is the one piece we
take from the open-source world, exactly as VLC, Plex, Jellyfin and Chrome do. The set-up is documented in
[THIRD_PARTY.md](THIRD_PARTY.md).

**Platforms.** The native window ships for Windows (x64 and ARM64). On macOS and Linux PlayAnything currently runs in launcher mode
(it drives a separately installed mpv with the same profile, RAW previews and cloud handling); native windows for those platforms are
the next step. `playanything config engine external` selects launcher mode on Windows too.

## Background player (service mode)

On Windows the app is single-instance: the first window stays open and every further open goes to it, so there is nothing to run as a
service. On macOS/Linux launcher mode, `PLAYANYTHING_SERVICE=1` at install time registers a per-user background player (systemd
user unit / launchd agent) for the same effect; `playanything daemon` / `playanything stop` control it.

## Configuration

`playanything config` shows the file location and all keys:

| Key | Default | Meaning |
|---|---|---|
| `engine` | `embedded` | `embedded` (own window + built-in engine) or `external` (launch an installed mpv) |
| `mpv_path` | auto | external engine only |
| `daemon` | `auto` | launcher mode background player: `auto` / `always` / `never` |
| `hydrate_first` | `false` | always download cloud placeholders fully before playing |
| `raw_full_decode`, `raw_decoder` | `false`, `auto` | develop RAW with darktable-cli / rawtherapee-cli / dcraw_emu / dcraw |
| `redline_path`, `redline_args` | auto, ProRes proxy | how R3D proxies are rendered ([docs](docs/RED-R3D.md)) |
| `use_system_mpv_config` | `false` | use your own `~/.config/mpv` profile instead of the managed one |
| `fullscreen` | `false` | start fullscreen |

Engine tuning goes in **`user.conf`** inside the profile folder (Help → *Open settings folder*, or `playanything doctor`). It is
included after the managed `mpv.conf` and never overwritten; any [mpv option](https://mpv.io/manual/stable/#options) works there
(`hwdec=nvdec`, `gpu-api=vulkan`, `alang=en,eng`, `screenshot-directory=…`). Environment overrides: `PLAYANYTHING_HOME` (portable
install root), `PLAYANYTHING_LIBMPV` (engine library path), `PLAYANYTHING_MPV` (external mpv path), `PLAYANYTHING_DEBUG=1` (log).

## Building from source

```sh
git clone https://github.com/inphaseye172/playanything && cd playanything
make build            # ./bin/playanything (Linux/macOS launcher build; Windows: GOOS=windows make build)
make test             # unit + headless engine/player tests (needs libmpv + ffmpeg installed locally)
make icon rsrc        # app icon from assets/icon/source.png (see assets/icon/README.md) + Windows resources
make windows-bundle   # dist/playanything-windows-{amd64,arm64}.zip when libmpv/<arch>/libmpv-2.dll is present
```

Releases: push a `v*` tag, or run the *Release* workflow from the Actions tab with a tag name. The workflow fetches the engine DLLs,
builds every target, zips the Windows bundles and publishes them; the installers always fetch the latest release.

## Credits & licenses

PlayAnything © Anchor Point Studio, MIT licensed ([LICENSE](LICENSE)). It embeds [libmpv](https://mpv.io) (mpv project,
GPL/LGPL), which contains [FFmpeg](https://ffmpeg.org) and [libplacebo](https://libplacebo.org); Windows builds of the engine come from
[shinchiro/mpv-winbuild-cmake](https://github.com/shinchiro/mpv-winbuild-cmake). REDline/REDCINE-X PRO are © RED Digital Cinema and
Blackmagic RAW Player is © Blackmagic Design; neither is included. Full list: [THIRD_PARTY.md](THIRD_PARTY.md).

---

<sub>Keywords: media player, one click, play any file, universal player, mkv player, mp4 player, mov player, flac player, raw photo viewer, arw viewer, cr3 viewer, nef viewer, dng viewer, r3d player, braw, 10-bit video, HDR playback, multiple audio tracks, nvidia shadowplay separate tracks, GPU accelerated video player, NVDEC, D3D11VA, VAAPI, Synology Drive on-demand sync preview, OneDrive files on-demand video, NAS video playback, SMB streaming, placeholder files, lightweight media player Windows, open source media player, native Win32 player, Go media player, libmpv, Anchor Point Studio.</sub>
