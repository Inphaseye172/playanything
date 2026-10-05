# Third-party components

PlayAnything itself is MIT licensed (see LICENSE). The Windows bundle ships
these components alongside `playanything.exe`:

| Component | What it does | License | Source |
|---|---|---|---|
| **libmpv** (`libmpv-2.dll`) | playback engine: demuxing, decoding, GPU rendering, audio output, subtitles | GPLv3 (shinchiro build; mpv itself is LGPLv2.1+/GPLv2+) | https://github.com/shinchiro/mpv-winbuild-cmake · https://github.com/mpv-player/mpv |
| FFmpeg (inside libmpv) | every container and codec | LGPLv2.1+/GPLv2+ (build-dependent) | https://ffmpeg.org |
| libplacebo (inside libmpv) | GPU video renderer, HDR tone mapping | LGPLv2.1+ | https://libplacebo.org |
| libass, dav1d, libjxl, … (inside libmpv) | subtitles, AV1, JPEG XL, … | various permissive / LGPL | see shinchiro's build manifest |

The exact set of libraries and their licenses for a given `libmpv-2.dll` are
listed in the `mpv-dev-*` release notes at the shinchiro repository; the
build is reproducible from its CMake scripts. Because the DLL is GPLv3, the
Windows bundle as a whole is distributed under GPLv3-compatible terms; the
PlayAnything source code remains MIT and may be combined with any other
engine build (for example an LGPL build of libmpv).

Build-time tools and vendored code:

| Component | Use | License |
|---|---|---|
| github.com/ebitengine/purego | calling libmpv without cgo | Apache 2.0 |
| github.com/akavel/rsrc | embedding the icon and manifest into the exe | MIT |
| PS-SFTA (Danysys) | `install/set-default.ps1` UserChoice hash | MIT |

Camera-RAW previews, cloud-placeholder handling, folder playlists, the R3D
workflow, the user interface, installers and everything else in this
repository are original PlayAnything code.
