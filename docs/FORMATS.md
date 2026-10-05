# Supported formats

PlayAnything plays everything mpv/FFmpeg can demux and decode, which in practice
is every container and codec you will meet, plus three categories mpv does not
handle on its own: camera RAW photos, REDCODE RAW and Blackmagic RAW.

Run `playanything extensions` for the exact list your build registers
(232 extensions) and `playanything info <file>` to see how a given file is
classified and what will happen when you open it.

## Video containers and codecs

| Containers | Codecs (software decode always; GPU decode where listed) |
|---|---|
| MKV, MK3D, WebM | H.264/AVC (GPU), H.265/HEVC 8/10/12-bit incl. HDR10, HLG, Dolby Vision profile 8 base layer (GPU), AV1 8/10-bit (GPU on RTX 30+/RDNA2+/Arc/Apple M3+), VP9 (GPU), VP8 (GPU), MPEG-2 |
| MP4, M4V, MOV, QT, 3GP, F4V | H.264, HEVC, AV1, ProRes 422/4444/RAW-wrapped (ProRes GPU on Apple Silicon), DNxHD/DNxHR, MJPEG, Cineform, Apple Animation, PNG/QTRLE |
| MXF, GXF, LXF | XDCAM, AVC-Intra, DNxHD/HR, ProRes, JPEG 2000, uncompressed v210 |
| AVI, WMV, ASF, FLV, RM/RMVB, DivX | Xvid/DivX, VC-1 (GPU), WMV 7/8/9, Sorenson, RealVideo, Indeo |
| MPEG-TS/M2TS/MTS (AVCHD, broadcast captures), VOB/EVO, MPG/MPEG | MPEG-1/2, H.264, HEVC, AC-3/E-AC-3/DTS/TrueHD audio |
| Elementary streams: .h264 .264 .hevc .265 .av1 .ivf .obu .y4m .yuv | raw bitstreams straight from encoders |
| Misc: OGV/OGM, NUT, WTV, DVR-MS, DV, Bink/Smacker (game video), SWF (video only) | |

Multiple audio tracks, multiple subtitle tracks, chapters, attachments and
external audio/subtitle files with matching names are all handled. When a file
has several audio tracks (NVIDIA ShadowPlay "separate tracks", OBS multi-track
recordings, dual-language rips), PlayAnything lists them on screen when the
file opens and `a` cycles through them.

## Audio

MP3, AAC/M4A/M4B, FLAC (up to 32-bit/384 kHz), ALAC, WAV/W64/RF64/BWF (PCM up
to 32-bit float, multichannel), AIFF, Ogg Vorbis, Opus, Speex, WMA, APE
(Monkey's Audio), WavPack, TTA, TAK, Musepack, DSD (DSF/DFF, converted to PCM),
AC-3, E-AC-3, DTS/DTS-HD, Dolby TrueHD/MLP, AMR, GSM, AU/SND, CAF, MKA,
tracker modules (MOD/XM/IT/S3M via libopenmpt when mpv is built with it).

Embedded cover art is shown; 5.1/7.1 is passed to the output device when it
supports it (`audio-channels=auto-safe`).

## Images

JPEG, PNG/APNG, GIF (animated), BMP, WebP (animated), TIFF (8/16-bit, multi-page
first page), TGA, PNM/PBM/PGM/PPM/PAM/PFM, PCX, PSD (flattened), HEIC/HEIF
(needs an mpv/FFmpeg with HEVC + HEIF demuxer, standard in current builds),
AVIF, JPEG XL (when FFmpeg has libjxl), JPEG 2000, OpenEXR (tone-mapped), Radiance
HDR, DDS, DPX, SGI, XBM/XPM, ICO/CUR, QOI, PICT, Sun Raster.

Images stay on screen until you move on; `<`/`>` or Page Up/Down step through the
folder in natural order; `r` rotates; Ctrl+wheel zooms.

## Camera RAW photos

| Maker | Extensions | How it is displayed |
|---|---|---|
| Sony | ARW, SRF, SR2 | embedded 1616×1080 JPEG preview (IFD0 `JPEGInterchangeFormat`) |
| Canon | CR2, CR3, CRW | CR2: full-size JPEG in IFD0; CR3: full-size JPEG track; CRW: scan |
| Nikon | NEF, NRW | full-size JPEG in SubIFD0, or maker-note `PreviewIFD` |
| Adobe / GoPro | DNG, GPR | largest preview IFD (`NewSubfileType=1`) |
| Fujifilm | RAF | JPEG pointed to by the RAF header (offset 84) |
| Olympus / OM System | ORF | maker-note preview (found by scan) |
| Panasonic / Leica | RW2, RWL, RAW | `JpgFromRaw` (tag 0x002E) |
| Pentax | PEF, PTX | maker-note preview (scan) |
| Samsung | SRW | preview IFD |
| Hasselblad | 3FR, FFF | preview IFD |
| Phase One / Leaf / Mamiya | IIQ, MOS, MEF | preview IFD / scan |
| Minolta, Epson, Kodak, Sigma | MRW, ERF, KDC/DCR/K25, X3F | scan |

PlayAnything parses the TIFF/EXIF structure of the RAW file and also scans the
whole file for complete JPEG streams, then shows the largest 8-bit JPEG it finds
rotated according to the EXIF orientation. This is the same preview Lightroom,
Finder and the camera's LCD show, and it takes milliseconds (a 40 MB file scans
in about 10 ms). The preview is cached under the cache directory, keyed by
path+size+mtime.

For a true demosaic (full resolution, your own white balance), pass `--raw-full`
or set `raw_full_decode: true`. PlayAnything then calls, in order of preference,
`darktable-cli`, `rawtherapee-cli`, `dcraw_emu` (LibRaw) or `dcraw`, and caches
the result. Those tools are optional and not installed by PlayAnything.

Formats without an embedded JPEG (ARRIRAW .ari, some scientific TIFF RAWs)
are not viewable without a developer.

## Cinema RAW

| Format | Status |
|---|---|
| REDCODE RAW (.R3D) | **Needs RED's SDK.** FFmpeg has no REDCODE decoder. PlayAnything renders a proxy with RED's free `REDline` tool when installed (once per clip, cached), or opens the clip in REDCINE-X PRO. See [RED-R3D.md](RED-R3D.md). |
| Blackmagic RAW (.braw) | **Needs Blackmagic's SDK.** Opened in Blackmagic RAW Player when installed. |
| ProRes RAW (.mov) | FFmpeg decodes the container but not ProRes RAW frames; not playable without Apple's SDK. |
| ARRIRAW (.ari/.arx) | Not supported. |
| Canon Cinema RAW Light (.crm) | Not supported (Canon SDK only). |

## Playlists and streams

M3U/M3U8/PLS playlists; `http(s)://`, `rtsp://`, `rtmp://`, `srt://`, `udp://`,
`rtp://`, `smb://` (when mpv is built with libsmbclient; otherwise mount the
share) and anything else FFmpeg's protocols support. YouTube and other sites
work when `yt-dlp` is installed (mpv's built-in hook).
