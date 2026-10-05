// Package media classifies files and URLs into the broad categories
// PlayAnything knows how to open: video, audio, images, camera RAW photos,
// REDCODE RAW (R3D), Blackmagic RAW, playlists and streams.
//
// Classification is by extension first, then by sniffing the first bytes of
// the file for files with a missing or unknown extension.
package media

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Kind is the broad category a path falls into.
type Kind int

const (
	Unknown   Kind = iota
	Video          // Anything FFmpeg/mpv demuxes with a video stream (mkv, mp4, mov, ...)
	Audio          // Audio-only containers (flac, mp3, wav, dsf, ...)
	Image          // Images FFmpeg can decode directly (jpg, png, webp, heic, exr, ...)
	RawImage       // Camera RAW photos (ARW, NEF, CR2/CR3, DNG, ...) - shown via embedded preview
	R3D            // REDCODE RAW cinema footage - needs RED's SDK (REDline / REDCINE-X)
	BRAW           // Blackmagic RAW - needs Blackmagic's SDK / player
	Playlist       // m3u / m3u8 / pls
	Stream         // URL with a scheme (http, https, rtsp, rtmp, srt, udp, ...)
	Directory      // A folder: play every media file inside
)

func (k Kind) String() string {
	switch k {
	case Video:
		return "video"
	case Audio:
		return "audio"
	case Image:
		return "image"
	case RawImage:
		return "camera-raw"
	case R3D:
		return "redcode-r3d"
	case BRAW:
		return "blackmagic-raw"
	case Playlist:
		return "playlist"
	case Stream:
		return "stream"
	case Directory:
		return "directory"
	default:
		return "unknown"
	}
}

// Extension tables. Extensions are lower case, without the leading dot.
// Keep these in sync with the Lua autoload script in internal/assets.
var (
	VideoExts = set(
		// Matroska / WebM
		"mkv", "mk3d", "webm",
		// MPEG-4 / QuickTime family (incl. ProRes, DNxHR, HEVC/H.265 10-bit, AV1)
		"mp4", "m4v", "mov", "qt", "3gp", "3g2", "f4v",
		// Legacy and broadcast containers
		"avi", "wmv", "asf", "flv", "ogv", "ogm", "ogx", "rm", "rmvb", "divx", "dv",
		"mpg", "mpeg", "mpe", "m1v", "m2v", "mpv2", "vob", "evo",
		"ts", "m2ts", "mts", "tp", "trp", "tsv",
		// Professional / camera containers
		"mxf", "gxf", "lxf", "y4m", "yuv", "nut", "wtv", "dvr-ms", "ivf", "obu",
		"h264", "264", "h265", "265", "hevc", "avc", "av1", "vc1", "mjpg", "mjpeg",
		// Misc
		"nsv", "bik", "bk2", "smk", "roq", "thp", "vp9", "dav", "amv", "mod", "tod", "rec", "swf",
	)

	AudioExts = set(
		"mp3", "mp2", "mp1", "mpa", "m4a", "m4b", "m4r", "aac", "ac3", "eac3", "ec3", "dts", "dtshd",
		"thd", "mlp", "truehd", "flac", "wav", "wave", "w64", "rf64", "bwf", "aiff", "aif", "aifc",
		"ogg", "oga", "opus", "spx", "wma", "wv", "ape", "mac", "tta", "tak", "mka", "mpc", "mp+",
		"ra", "ram", "au", "snd", "caf", "amr", "awb", "gsm", "dsf", "dff", "dsd", "alac", "voc",
		"shn", "ofr", "ofs", "adx", "aa3", "oma", "at3", "xm", "it", "s3m", "mptm", "umx", "669",
		"mtm", "okt", "ult", "far", "mdl", "ptm", "dbm", "amf", "psm", "mt2", "med", "stm",
	)

	ImageExts = set(
		"jpg", "jpeg", "jpe", "jfif", "jif", "png", "apng", "gif", "bmp", "dib", "webp",
		"tif", "tiff", "tga", "pnm", "pbm", "pgm", "ppm", "pam", "pfm", "pcx", "psd",
		"heic", "heif", "hif", "avif", "jxl", "jp2", "j2k", "jpx", "jpf", "exr", "hdr",
		"dds", "dpx", "sgi", "rgb", "xbm", "xpm", "ico", "cur", "qoi", "wbmp", "pic", "pict",
		"sr", "ras", "sun", "im1", "im8", "im24", "xwd", "cin",
	)

	// Camera RAW formats. All of these carry an embedded camera-rendered JPEG
	// preview that PlayAnything extracts and displays instantly.
	RawImageExts = set(
		"arw", "srf", "sr2", // Sony
		"cr2", "cr3", "crw", // Canon
		"nef", "nrw", // Nikon
		"dng", "gpr", // Adobe DNG, GoPro (DNG-based)
		"raf",        // Fujifilm
		"orf",        // Olympus / OM System
		"rw2", "rwl", // Panasonic / Leica
		"pef", "ptx", // Pentax
		"srw",        // Samsung
		"3fr", "fff", // Hasselblad
		"iiq",               // Phase One
		"mef",               // Mamiya
		"mos",               // Leaf
		"mrw",               // Minolta
		"erf",               // Epson
		"kdc", "dcr", "k25", // Kodak
		"x3f",        // Sigma (Foveon)
		"raw", "rwz", // Panasonic/Leica legacy, Rawzor
	)

	R3DExts      = set("r3d")
	BRAWExts     = set("braw")
	PlaylistExts = set("m3u", "m3u8", "pls")
)

var urlScheme = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9+.-]*://`)

// IsURL reports whether s looks like a URL with a scheme (not a Windows drive).
func IsURL(s string) bool {
	if len(s) > 1 && s[1] == ':' { // C:\... or C:/...
		return false
	}
	return urlScheme.MatchString(s)
}

// Ext returns the lower-case extension of path without the dot.
func Ext(path string) string {
	e := strings.ToLower(filepath.Ext(path))
	if strings.ContainsAny(e, `/\`) { // "dir.d/file" with no extension
		return ""
	}
	return strings.TrimPrefix(e, ".")
}

// ByExtension classifies by extension only; Unknown when the extension is not in any table.
func ByExtension(path string) Kind {
	e := Ext(path)
	switch {
	case e == "":
		return Unknown
	case VideoExts[e]:
		return Video
	case AudioExts[e]:
		return Audio
	case ImageExts[e]:
		return Image
	case RawImageExts[e]:
		return RawImage
	case R3DExts[e]:
		return R3D
	case BRAWExts[e]:
		return BRAW
	case PlaylistExts[e]:
		return Playlist
	}
	return Unknown
}

// IsMediaExt reports whether the extension belongs to anything PlayAnything plays.
// Used when expanding directories.
func IsMediaExt(path string) bool {
	switch ByExtension(path) {
	case Video, Audio, Image, RawImage, R3D, BRAW:
		return true
	}
	return false
}

// Detect classifies a path or URL. For files whose extension is unknown it
// sniffs the first bytes of the file. Errors reading the file are swallowed and
// result in Unknown (the caller will still hand the file to mpv, which may
// know better).
func Detect(path string) Kind {
	if IsURL(path) {
		return Stream
	}
	if fi, err := os.Stat(path); err == nil && fi.IsDir() {
		return Directory
	}
	if k := ByExtension(path); k != Unknown {
		return k
	}
	f, err := os.Open(path)
	if err != nil {
		return Unknown
	}
	defer f.Close()
	head := make([]byte, 512)
	n, _ := io.ReadFull(f, head)
	return Sniff(head[:n])
}

// Sniff classifies by magic bytes. It recognises the common containers so that
// files with a wrong or missing extension (very common with camera dumps and
// downloads) still open with one click.
func Sniff(b []byte) Kind {
	has := func(off int, sig string) bool {
		return len(b) >= off+len(sig) && string(b[off:off+len(sig)]) == sig
	}
	switch {
	// --- Camera RAW first: these look like TIFF/ISO-BMFF and would be misclassified below.
	case has(0, "FUJIFILMCCD-RAW"):
		return RawImage
	case has(4, "ftypcrx "):
		return RawImage // Canon CR3
	case has(0, "FOVb"):
		return RawImage // Sigma X3F
	case has(0, "\x00MRM"):
		return RawImage // Minolta MRW
	case has(0, "IIRO") || has(0, "MMOR") || has(0, "IIRS"):
		return RawImage // Olympus ORF
	case has(0, "IIU\x00"):
		return RawImage // Panasonic RW2
	case has(0, "II*\x00") || has(0, "MM\x00*"):
		return Image // Plain TIFF (DNG/NEF/CR2/ARW are caught by extension; TIFF sniff is a fallback)
	// --- Cinema RAW
	case has(4, "RED1") || has(4, "RED2"):
		return R3D
	// --- Images
	case bytes.HasPrefix(b, []byte{0xFF, 0xD8, 0xFF}):
		return Image
	case has(0, "\x89PNG\r\n\x1a\n"), has(0, "GIF87a"), has(0, "GIF89a"), has(0, "BM"):
		return Image
	case has(0, "RIFF") && has(8, "WEBP"):
		return Image
	case has(4, "ftypheic"), has(4, "ftypheix"), has(4, "ftypmif1"), has(4, "ftypmsf1"), has(4, "ftypavif"), has(4, "ftypavis"):
		return Image
	case has(0, "\xff\x0a") || has(0, "\x00\x00\x00\x0cJXL \r\n\x87\n"):
		return Image // JPEG XL
	case has(0, "v/1\x01"):
		return Image // OpenEXR
	case has(0, "#?RADIANCE") || has(0, "#?RGBE"):
		return Image
	case has(0, "SDPX") || has(0, "XPDS"):
		return Image // DPX
	case has(0, "8BPS"):
		return Image // Photoshop PSD
	// --- Audio
	case has(0, "fLaC"), has(0, "ID3"), has(0, "MAC "), has(0, "wvpk"), has(0, "DSD "), has(0, "FRM8"),
		has(0, "TTA1"), has(0, "MP+"), has(0, "MPCK"), has(0, ".snd"), has(0, "caff"), has(0, "#!AMR"):
		return Audio
	case has(0, "RIFF") && (has(8, "WAVE") || has(8, "RMP3")):
		return Audio
	case has(0, "FORM") && (has(8, "AIFF") || has(8, "AIFC")):
		return Audio
	case has(0, "OggS"):
		return Audio // could be Theora video; mpv handles both so this only affects the window hint
	case has(4, "ftypM4A ") || has(4, "ftypM4B "):
		return Audio
	case len(b) >= 4 && b[0] == 0xFF && (b[1]&0xE0) == 0xE0 && (b[1]&0x06) != 0 && (b[1]>>3)&3 != 1 &&
		b[1] != 0xFE && b[2]>>4 != 0 && b[2]>>4 != 0xF && (b[2]>>2)&3 != 3:
		return Audio // raw MPEG audio frame sync (mp3/mp2/aac-adts) with valid header fields
	// --- Video containers
	case has(0, "\x1aE\xdf\xa3"):
		return Video // EBML (Matroska / WebM)
	case has(4, "ftyp"), has(4, "moov"), has(4, "mdat"), has(4, "wide"), has(4, "free"), has(4, "skip"):
		return Video // ISO BMFF / QuickTime
	case has(0, "RIFF") && has(8, "AVI "):
		return Video
	case has(0, "FLV\x01"):
		return Video
	case has(0, "\x30\x26\xB2\x75\x8E\x66\xCF\x11"):
		return Video // ASF / WMV
	case has(0, "\x00\x00\x01\xBA") || has(0, "\x00\x00\x01\xB3"):
		return Video // MPEG program stream / elementary
	case len(b) >= 189 && b[0] == 0x47 && b[188] == 0x47:
		return Video // MPEG transport stream
	case has(0, ".RMF"):
		return Video // RealMedia
	case has(0, "YUV4MPEG2"):
		return Video
	case has(0, "DKIF"):
		return Video // IVF (VP8/VP9/AV1 raw)
	case has(0, "\x06\x0e\x2b\x34\x02\x05\x01\x01\x0d\x01\x02\x01\x01\x02"):
		return Video // MXF partition pack key
	case has(0, "#EXTM3U"):
		return Playlist
	case has(0, "[playlist]"):
		return Playlist
	}
	return Unknown
}

// ExpandDirectory lists the media files directly inside dir (non-recursive),
// naturally sorted the way a camera card or Explorer window would show them.
func ExpandDirectory(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		p := filepath.Join(dir, e.Name())
		if IsMediaExt(p) {
			out = append(out, p)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return NaturalLess(filepath.Base(out[i]), filepath.Base(out[j])) })
	return out, nil
}

// NaturalLess compares strings so that "clip2" < "clip10". Case-insensitive.
func NaturalLess(a, b string) bool {
	a, b = strings.ToLower(a), strings.ToLower(b)
	i, j := 0, 0
	for i < len(a) && j < len(b) {
		ca, cb := a[i], b[j]
		if isDigit(ca) && isDigit(cb) {
			si := i
			for i < len(a) && isDigit(a[i]) {
				i++
			}
			sj := j
			for j < len(b) && isDigit(b[j]) {
				j++
			}
			na := strings.TrimLeft(a[si:i], "0")
			nb := strings.TrimLeft(b[sj:j], "0")
			if len(na) != len(nb) {
				return len(na) < len(nb)
			}
			if na != nb {
				return na < nb
			}
			continue
		}
		if ca != cb {
			return ca < cb
		}
		i++
		j++
	}
	return len(a)-i < len(b)-j
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

func set(items ...string) map[string]bool {
	m := make(map[string]bool, len(items))
	for _, s := range items {
		m[s] = true
	}
	return m
}

// AllExtensions returns every extension PlayAnything associates itself with,
// sorted, for installers and docs.
func AllExtensions() []string {
	var out []string
	for _, m := range []map[string]bool{VideoExts, AudioExts, ImageExts, RawImageExts, R3DExts, BRAWExts, PlaylistExts} {
		for e := range m {
			out = append(out, e)
		}
	}
	sort.Strings(out)
	return out
}
