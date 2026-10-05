# Cloud, NAS and network files

"I can't see a preview because the file isn't a true local file" is the problem
PlayAnything was built around. This page explains what is going on and what
PlayAnything does about it.

## Why Explorer / Finder show no preview

Sync clients with *on-demand* modes (Synology Drive On-demand Sync, OneDrive
Files On-Demand, Dropbox online-only, Google Drive streaming, iCloud "Optimize
Mac Storage") keep only a **placeholder** on disk: name, size, dates and an
icon, but no bytes. Windows marks these with the file attributes
`FILE_ATTRIBUTE_RECALL_ON_DATA_ACCESS` / `OFFLINE`, macOS with the `SF_DATALESS`
flag. Thumbnail and preview handlers deliberately refuse to touch such files
because opening one would trigger a download, so you see a generic icon and
often cannot scrub or play it from the shell either.

Files on SMB/NFS shares are a milder version of the same issue: Explorer
throttles thumbnailing on network drives and many players seek poorly over SMB.

## What PlayAnything does

1. **Detects** the situation before launching the player (`playanything info
   <file>` shows `locality: cloud file, not stored locally yet (Synology Drive)`).
2. **Opens the window immediately** (`force-window=immediate`) and shows
   "Opening … ⬇ Downloading from Synology Drive — not stored locally yet", so a
   click always gets an instant response even when the bytes take seconds.
3. **Streams while the sync client downloads.** Reading the file is what makes
   the cloud filter driver fetch it; mpv reads it with a large RAM cache
   (`cache=yes`, 512 MiB read-ahead, 4 MiB reads) so playback starts as soon as
   the first chunk lands and seeking inside already-downloaded ranges is instant.
4. Optionally **downloads first** (`--hydrate-first`, or `hydrate_first: true`
   in config.json) with a progress readout in the window and the terminal.
   Useful for very large files on slow links where you want full-speed scrubbing
   from the first frame.

After PlayAnything has played a placeholder once, the sync client usually keeps
the file local ("recently used"), so the second open is like any local file.

### Synology Drive specifics

- On-demand Sync is available on Windows 10 1809+ only and uses Microsoft's
  Cloud Files API, exactly like OneDrive. On macOS 12+ Synology Drive 3.x uses
  the File Provider framework (`~/Library/CloudStorage/SynologyDrive-…`).
- Right-click → *Synology Drive* → *Pin to local* keeps a folder permanently
  local; PlayAnything plays pinned and unpinned files alike.
- If you access the NAS directly over SMB (`\\NAS\video\…` or a mapped drive),
  there are no placeholders at all; PlayAnything just streams over SMB with the
  big cache. For 4K/8K 10-bit footage make sure the link is wired 1 GbE or
  better, or copy/pin first.

### Network shares and the cache

The tuned defaults live in the managed `mpv.conf`:

```
cache=yes
demuxer-max-bytes=512MiB      # read-ahead
demuxer-max-back-bytes=128MiB # keep behind the playhead for instant back-seeks
demuxer-readahead-secs=60
stream-buffer-size=4MiB       # fewer, larger reads over SMB/WebDAV/cloud filters
cache-pause-wait=2
network-timeout=120
```

Raise `demuxer-max-bytes` in `user.conf` if you have RAM to spare and play
very high bitrate material over slow links.

## Linux

Synology Drive Client on Linux syncs everything (no on-demand mode), so files
are always local. Network mounts (CIFS/NFS/SSHFS/rclone) are detected and get
the same cache treatment.
