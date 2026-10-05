<h1 align="center">PlayAnything</h1>

<p align="center"><b>One click, any media.</b><br>
A tiny, fast, GPU-accelerated launcher that plays <i>every</i> video, audio, photo and camera-RAW file —
on your disk, on your NAS, or sitting as a cloud placeholder in Synology Drive / OneDrive — in one window, instantly.</p>

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

That installs the 4 MB `playanything` launcher, installs [mpv](https://mpv.io) (the playback engine) if you
don't have it, writes a tuned player profile, and wires up your desktop:

- **Windows:** right-click any file or folder → **Play with PlayAnything**; appears in *Open with* for 230+ extensions and in
  *Settings → Default apps*; Start Menu shortcut; `playanything` on your PATH.
- **Linux:** `.desktop` entry registered as default for 130+ MIME types (video/audio/image/camera-raw).
- **macOS:** `~/Applications/PlayAnything.app` for Finder's *Open With* / double-click.

Want it to run as a background service (one always-ready window, instant opens, playlist queueing)?
Set `PLAYANYTHING_SERVICE=1` before running the installer — see [Background player](#background-player-service-mode).

Uninstall: same URLs with `uninstall.ps1` / `uninstall.sh`.

## Why this exists

| The problem | What PlayAnything does |
|---|---|
| Explorer/Finder show **no preview** for files on Synology Drive On-demand Sync, OneDrive Files On-Demand, Dropbox online-only, SMB shares — "it's not a true local file" | Detects cloud placeholders (`RECALL_ON_DATA_ACCESS`, `SF_DATALESS`) and network mounts, **opens the window instantly**, tells you it's downloading, and streams through a 512 MiB RAM cache while the sync client fetches the bytes. Optional `--hydrate-first` with a progress bar. [Details](docs/CLOUD-AND-NETWORK.md) |
| Camera RAW photos (ARW, CR2, CR3, NEF, DNG, RAF, ORF, RW2, PEF…) **won't open** in a media player | Extracts the camera's **embedded full-size JPEG preview** in ~10 ms (TIFF/EXIF parsing + whole-file JPEG scan), rotates per EXIF, caches it, and shows it — flip through a card of 500 RAWs with `→`. Optional true demosaic via darktable/RawTherapee/LibRaw with `--raw-full`. |
| 10-bit HEVC, HDR, AV1, 8K, ProRes, DNxHR, 5.1 stems, NVIDIA ShadowPlay recordings with **multiple audio tracks** | mpv + FFmpeg + libplacebo: every codec, GPU decode (`hwdec=auto-safe`), 10-bit output, HDR passthrough or tone-mapping, audio track list on screen, `a` to switch. |
| **RED R3D / Blackmagic RAW** need the vendor SDK; nothing open-source decodes them | Honest handling: renders a proxy once with RED's free `REDline` (if installed) and plays it, or hands off to REDCINE-X PRO / Blackmagic RAW Player, or tells you exactly what to install. [Details](docs/RED-R3D.md) |
| Players are heavy, phone-home, nag, or have installers that need admin | Single static Go binary, no runtime, no telemetry, per-user install, MIT licensed. mpv is the only dependency. |

\* R3D and BRAW cannot be decoded by any open-source software; see [docs/RED-R3D.md](docs/RED-R3D.md) for what PlayAnything does instead.

## Usage

Double-click a file, drag files onto the player window, or:

```sh
playanything movie.mkv                    # play
playanything ./DCIM/100MSDCF              # play a whole folder (natural sort)
playanything "https://example.com/a.mp4"  # stream
playanything --append song.flac           # add to the running player's playlist
playanything --hydrate-first big.mov      # download the cloud placeholder fully first
playanything --raw-full DSC01234.ARW      # true RAW development instead of embedded preview
playanything -f clip.mp4                  # fullscreen
playanything clip.mp4 -- --start=60 --volume=50   # anything after -- goes to mpv

playanything info clip.mkv                # what is it? local/cloud? which tracks? what will happen?
playanything doctor                       # mpv version, GPU decoders, config paths, optional tools
playanything config daemon always         # settings (see below)
```

### Keys (on top of mpv's defaults)

| Key | Action |
|---|---|
| `Space` / `←` `→` / `.` `,` | pause / seek 5 s / frame step |
| `a` `A` · `Ctrl+a` | next / previous audio track · list tracks |
| `s` `v` | next subtitle · toggle subtitles |
| `<` `>` · `PgUp` `PgDn` | previous / next file in folder |
| `r` `R` · `Ctrl+wheel` · `0` | rotate photo · zoom · reset |
| `f` `Enter` · double-click | fullscreen |
| `Ctrl+s` `Alt+s` | screenshot (to Desktop) |
| `i` · `Ctrl+h` · `d` | stats (shows hwdec in use) · toggle GPU decoding · deinterlace |
| `[` `]` `BS` | slower / faster / normal speed |
| `q` | quit |

## What it plays

Everything FFmpeg knows plus camera RAW. Short version:

- **Video:** MKV, MP4, MOV, WebM, AVI, WMV, FLV, MPEG-TS/M2TS, VOB, MXF, GXF, DV, OGV, RM, 3GP, raw H.264/HEVC/AV1 streams… with H.264, HEVC 8/10/12-bit (HDR10/HLG), AV1, VP9, VP8, MPEG-2, VC-1, ProRes, DNxHD/HR, Cineform, MJPEG, JPEG 2000…
- **Audio:** MP3, AAC, FLAC, ALAC, WAV/W64/BWF, AIFF, Ogg Vorbis, Opus, WMA, APE, WavPack, TTA, TAK, Musepack, DSD (DSF/DFF), AC-3/E-AC-3, DTS/DTS-HD, TrueHD, AMR, tracker modules…
- **Images:** JPEG, PNG, GIF, WebP, BMP, TIFF, TGA, PSD, HEIC/HEIF, AVIF, JPEG XL, JPEG 2000, OpenEXR, Radiance HDR, DDS, DPX, PNM…
- **Camera RAW (30 extensions):** Sony ARW/SRF/SR2 · Canon CR2/CR3/CRW · Nikon NEF/NRW · Adobe DNG · GoPro GPR · Fujifilm RAF · Olympus ORF · Panasonic RW2/RWL · Pentax PEF · Samsung SRW · Hasselblad 3FR/FFF · Phase One IIQ · Leaf MOS · Mamiya MEF · Minolta MRW · Epson ERF · Kodak KDC/DCR · Sigma X3F
- **Cinema RAW:** RED R3D and Blackmagic BRAW via the vendors' free tools (see above).
- **Playlists & streams:** M3U/M3U8/PLS, HTTP(S), RTSP, RTMP, SRT, UDP…

Full tables with codec/GPU notes: [docs/FORMATS.md](docs/FORMATS.md). `playanything extensions` prints the 232 registered extensions.

## GPU acceleration

The managed `mpv.conf` uses `vo=gpu-next` (libplacebo) with `hwdec=auto-safe`, which picks the native zero-copy path per platform:

| Platform | Decoder | Notes |
|---|---|---|
| Windows | D3D11VA (any GPU) or NVDEC | 10-bit HEVC/AV1 zero-copy into the D3D11 swapchain; HDR passthrough when Windows HDR is on |
| Linux | VAAPI (Intel/AMD), NVDEC (NVIDIA), Vulkan | Flatpak mpv needs the matching freedesktop VAAPI extension |
| macOS | VideoToolbox | H.264/HEVC/ProRes; AV1 on M3+ |

Press `i` during playback to see `Hardware decoding: …`. Override in `user.conf` (`hwdec=nvdec`, `gpu-api=vulkan`, …).

## Background player (service mode)

Optional. With it on, PlayAnything keeps **one idle mpv** alive with an IPC socket; opening a file sends it to that window instead of starting a process. You get a single reusable window, zero start-up latency, `--append` queueing, and a right-click *Add to PlayAnything playlist* entry.

```sh
playanything daemon            # run it in the foreground (or let the installer register it)
playanything config daemon always   # auto-start it when needed (default "auto" uses it only if running; "never" disables)
playanything stop
```

The installers register it per user — `HKCU\…\Run` on Windows, a systemd *user* unit on Linux, a launchd agent on macOS — when `PLAYANYTHING_SERVICE=1` is set. It is a user-session process on purpose: a real Windows "service" runs in session 0 and cannot show a window. Pressing `q` closes the window; the daemon restarts an idle instance in the background.

## Configuration

`playanything config` shows the file location and all keys:

| Key | Default | Meaning |
|---|---|---|
| `mpv_path` | auto | path to mpv if not found automatically |
| `daemon` | `auto` | `auto` / `always` / `never` — see above |
| `hydrate_first` | `false` | always download cloud placeholders fully before playing |
| `raw_full_decode` | `false` | develop RAW with an external tool instead of the embedded preview |
| `raw_decoder` | `auto` | `darktable-cli` / `rawtherapee-cli` / `dcraw_emu` / `dcraw` |
| `redline_path`, `redline_args` | auto / ProRes proxy | how R3D proxies are rendered ([docs](docs/RED-R3D.md)) |
| `use_system_mpv_config` | `false` | use your own `~/.config/mpv` instead of the managed profile |
| `fullscreen` | `false` | start fullscreen |

Player tuning goes in **`user.conf`** inside the managed mpv folder (`playanything doctor` prints it). It is included after `mpv.conf` and never overwritten; any [mpv option](https://mpv.io/manual/stable/#options) works there.

Directories (per user, no admin): `%LOCALAPPDATA%\PlayAnything` · `~/Library/Application Support/PlayAnything` · `~/.config/playanything` (+ `~/.cache/playanything`). Set `PLAYANYTHING_HOME` for a portable install.

## How it works

```
playanything <file>
   │  classify (extension → magic bytes)            internal/media
   │  cloud/network status (placeholder? offline?)  internal/cloud
   ├─ camera RAW ──► embedded JPEG + EXIF rotation ─┐ internal/rawpreview (also called by a Lua hook
   ├─ R3D/BRAW ────► REDline proxy / vendor player ─┤ internal/red         for every playlist entry)
   └─ everything else ─────────────────────────────┴─► mpv (direct launch, or `loadfile` to the
                                                            background instance over JSON IPC)
```

- **Engine:** [mpv](https://mpv.io) (FFmpeg + libplacebo). PlayAnything never re-implements decoding.
- **Profile:** `internal/assets/mpv/mpv.conf` + three small Lua scripts: `pa-rawhook.lua` (RAW → preview swap at load time), `pa-autoload.lua` (folder → playlist, natural order), `pa-osd.lua` (opening/cloud/track/error messages).
- **Launcher:** Go, stdlib only, static binary, ~4 MB, starts in a few ms. Windows build is a GUI-subsystem exe (no console flash) that attaches to your terminal when run from one.

## Building from source

```sh
git clone https://github.com/inphaseye172/playanything && cd playanything
make build        # ./bin/playanything
make test         # unit tests + IPC/daemon tests against a local mpv when installed
make cross        # dist/ for windows/macos/linux × amd64/arm64
```

Releases are produced by `.github/workflows/release.yml` when a `v*` tag is pushed, or from the Actions tab ("Release" → "Run workflow" → type the tag); the installers always fetch the latest release.

## FAQ

**Is this a new player?** No — it is the missing glue around mpv: format/cloud/RAW-aware launching, sane defaults, OS integration, and a service mode. If you already love mpv, use `use_system_mpv_config` to keep your config and still get the RAW/cloud/R3D handling.

**Can it really play R3D?** Only via RED's free tools (see [docs/RED-R3D.md](docs/RED-R3D.md)). Anyone claiming native open-source R3D playback is mistaken.

**Does it change my default apps silently?** No. Windows does not allow that; the installer registers PlayAnything everywhere it can (*Open with*, *Default apps*, right-click menu) and shows you where to flip the switch. On Linux it sets `xdg-mime` defaults (undo with your file manager); on macOS use *Get Info → Change All*.

**Thumbnails in Explorer for cloud files?** Out of scope — that is a shell extension problem on Microsoft's side. PlayAnything makes *opening* them one click and instant instead.

## Credits & licenses

PlayAnything is MIT licensed (see [LICENSE](LICENSE)). It launches [mpv](https://mpv.io) (GPLv2+/LGPLv2.1+), which bundles [FFmpeg](https://ffmpeg.org) and [libplacebo](https://libplacebo.org); on Windows the installer fetches [shinchiro's mpv builds](https://github.com/shinchiro/mpv-winbuild-cmake) via winget (`shinchiro.mpv`). REDline/REDCINE-X PRO are © RED Digital Cinema; Blackmagic RAW Player is © Blackmagic Design; neither is included.

---

<sub>Keywords: media player, one click, play any file, universal player, mkv player, mp4 player, mov player, flac player, raw photo viewer, arw viewer, cr3 viewer, nef viewer, dng viewer, r3d player, braw, 10-bit video, HDR playback, multiple audio tracks, nvidia shadowplay separate tracks, GPU accelerated video player, NVDEC, D3D11VA, VAAPI, Synology Drive on-demand sync preview, OneDrive files on-demand video, NAS video playback, SMB streaming, placeholder files, mpv frontend, mpv launcher, portable media player Windows, lightweight media player, open source media player, winget, homebrew, systemd user service.</sub>
