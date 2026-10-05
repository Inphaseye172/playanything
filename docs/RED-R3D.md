# RED R3D (REDCODE RAW) and other cinema RAW

## The honest part

FFmpeg — and therefore mpv, VLC, Chrome, and every open-source player — can
**demux** R3D files but cannot **decode** REDCODE RAW. Decoding requires RED's
proprietary R3D SDK, which may not be redistributed, so no open-source tool
will ever "just play" an R3D file natively. The same is true of Blackmagic RAW
(.braw), ProRes RAW and ARRIRAW.

PlayAnything therefore does the most useful one-click thing possible with the
tools RED gives away for free.

## What happens when you open an .R3D

1. **A cached proxy exists** → it plays instantly in mpv (GPU-decoded).
2. **`REDline` is installed** (it ships with the free
   [REDCINE-X PRO](https://www.red.com/download/redcine-x-pro-win) /
   [macOS](https://www.red.com/download/redcine-x-pro-mac)) → the player window
   opens with "Rendering a proxy with REDline…", REDline transcodes the clip
   once into the cache, then it plays. Next time it is instant.
3. **Only REDCINE-X PRO is installed** → the clip opens in REDCINE-X PRO.
4. **Nothing installed** → a message with the download link.

Proxies live in `<cache>/r3d-proxies/` (see `playanything doctor` for the
path) and are keyed by path, size and modification time.

## The REDline command

The default template in `config.json` is:

```
--i {input} --o {outbase} --outDir {outdir} --format 11 --QTcodec 2 --res 4
```

- `--format 11` = QuickTime transcode; `--QTcodec 2` = Apple ProRes 422 (HQ).
- `--res 4` = decode at quarter resolution (8K → 2K), which is fast on any
  machine and what you want for *looking at* footage. Use `--res 2` for half,
  `--res 1` for full.

Change it with:

```sh
playanything config redline_args "--i {input} --o {outbase} --outDir {outdir} --format 12 --res 2"   # DNxHD
playanything config redline_path "C:\Program Files\REDCINE-X PRO 64-bit\REDline.exe"               # if not auto-detected
```

Run `REDline --help` for every option (there are hundreds: color science, LUTs,
crop, burn-ins). RED documents it in the REDCINE-X PRO Operation Guide,
section "REDline". Each render writes a `.log` next to the proxy with the exact
command and REDline's output; `playanything info clip.R3D` shows what would run.

Multi-file clips (`A001_C001_0101XX_001.R3D`, `_002.R3D`, …) are one clip to
REDline; open the `_001` file.

## Blackmagic RAW

Install the free Blackmagic RAW Player (part of the "Blackmagic RAW" download at
<https://www.blackmagicdesign.com/support/family/blackmagic-raw>). PlayAnything
then opens `.braw` files in it. If you also have DaVinci Resolve, render a
ProRes/DNx proxy from there for mpv playback.

## Why not bundle the RED SDK?

Its license forbids redistribution and it is large and platform-specific.
PlayAnything stays a 4 MB dependency-free launcher; vendors' tools are detected
when present.
