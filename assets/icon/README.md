# App icon

Drop the **Anchor Point Studio icon logo** here as `source.png` (square, PNG,
512 px or larger, transparent background preferred). Then run:

```sh
make icon        # writes assets/icon/playanything.ico and playanything.png
make rsrc        # embeds the icon + manifest into the Windows build
```

The release workflow does both automatically. Until `source.png` exists a
placeholder mark is used so builds never fail.

- `playanything.ico` — Windows (Explorer, taskbar, Alt-Tab, file associations)
- `playanything.png` — Linux `.desktop` icon and macOS app bundle
