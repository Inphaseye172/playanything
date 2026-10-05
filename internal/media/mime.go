package media

import "sort"

// mimeByExt maps extensions to the MIME types desktop environments use for
// default-application registration (freedesktop .desktop MimeType=, xdg-mime).
// Only well-established types are listed; unknown extensions still open via
// the "Open with" / context-menu entries that installers add.
var mimeByExt = map[string][]string{
	// video
	"mkv": {"video/x-matroska"}, "mk3d": {"video/x-matroska-3d"}, "webm": {"video/webm"},
	"mp4": {"video/mp4"}, "m4v": {"video/x-m4v"}, "mov": {"video/quicktime"}, "qt": {"video/quicktime"},
	"3gp": {"video/3gpp"}, "3g2": {"video/3gpp2"}, "avi": {"video/x-msvideo", "video/avi"},
	"wmv": {"video/x-ms-wmv"}, "asf": {"video/x-ms-asf"}, "flv": {"video/x-flv"}, "f4v": {"video/x-f4v"},
	"ogv": {"video/ogg"}, "ogm": {"video/x-ogm+ogg"}, "rm": {"application/vnd.rn-realmedia"}, "rmvb": {"application/vnd.rn-realmedia-vbr"},
	"mpg": {"video/mpeg"}, "mpeg": {"video/mpeg"}, "mpe": {"video/mpeg"}, "m2v": {"video/mpeg"}, "vob": {"video/mpeg", "video/x-ms-vob"},
	"ts": {"video/mp2t"}, "m2ts": {"video/mp2t"}, "mts": {"video/mp2t"}, "mxf": {"application/mxf"},
	"dv": {"video/dv"}, "y4m": {"video/x-yuv4mpeg"}, "divx": {"video/x-msvideo"}, "ivf": {"video/x-ivf"},
	"h264": {"video/h264"}, "264": {"video/h264"}, "hevc": {"video/h265"}, "h265": {"video/h265"}, "265": {"video/h265"},
	"nut": {"video/x-nut"}, "wtv": {"video/x-ms-wtv"}, "dvr-ms": {"video/x-ms-dvr"},
	// audio
	"mp3": {"audio/mpeg"}, "mp2": {"audio/mpeg"}, "m4a": {"audio/mp4", "audio/x-m4a"}, "m4b": {"audio/x-m4b"},
	"aac": {"audio/aac"}, "ac3": {"audio/ac3"}, "eac3": {"audio/eac3"}, "dts": {"audio/vnd.dts"}, "dtshd": {"audio/vnd.dts.hd"},
	"thd": {"audio/vnd.dolby.mlp"}, "mlp": {"audio/vnd.dolby.mlp"}, "truehd": {"audio/vnd.dolby.mlp"},
	"flac": {"audio/flac", "audio/x-flac"}, "wav": {"audio/x-wav", "audio/wav", "audio/vnd.wave"}, "w64": {"audio/x-w64"},
	"aiff": {"audio/x-aiff"}, "aif": {"audio/x-aiff"}, "aifc": {"audio/x-aifc"}, "ogg": {"audio/ogg", "audio/x-vorbis+ogg"},
	"oga": {"audio/ogg"}, "opus": {"audio/opus", "audio/x-opus+ogg"}, "spx": {"audio/x-speex+ogg"},
	"wma": {"audio/x-ms-wma"}, "wv": {"audio/x-wavpack"}, "ape": {"audio/x-ape"}, "tta": {"audio/x-tta"}, "tak": {"audio/x-tak"},
	"mka": {"audio/x-matroska"}, "mpc": {"audio/x-musepack"}, "au": {"audio/basic"}, "caf": {"audio/x-caf"}, "amr": {"audio/amr"},
	"dsf": {"audio/x-dsf"}, "dff": {"audio/x-dff"}, "alac": {"audio/x-alac"}, "shn": {"audio/x-shorten"}, "voc": {"audio/x-voc"},
	"xm": {"audio/x-xm", "audio/xm"}, "it": {"audio/x-it"}, "s3m": {"audio/x-s3m"}, "mod": {"audio/x-mod"},
	// images
	"jpg": {"image/jpeg"}, "jpeg": {"image/jpeg"}, "jpe": {"image/jpeg"}, "jfif": {"image/jpeg"},
	"png": {"image/png"}, "apng": {"image/apng"}, "gif": {"image/gif"}, "bmp": {"image/bmp"}, "dib": {"image/bmp"},
	"webp": {"image/webp"}, "tif": {"image/tiff"}, "tiff": {"image/tiff"}, "tga": {"image/x-tga"},
	"pnm": {"image/x-portable-anymap"}, "pbm": {"image/x-portable-bitmap"}, "pgm": {"image/x-portable-graymap"}, "ppm": {"image/x-portable-pixmap"},
	"pcx": {"image/x-pcx"}, "psd": {"image/vnd.adobe.photoshop"}, "heic": {"image/heic", "image/heif"}, "heif": {"image/heif"},
	"avif": {"image/avif"}, "jxl": {"image/jxl"}, "jp2": {"image/jp2"}, "j2k": {"image/x-jp2-codestream"},
	"exr": {"image/x-exr"}, "hdr": {"image/vnd.radiance"}, "dds": {"image/x-dds"}, "dpx": {"image/x-dpx"},
	"sgi": {"image/x-sgi"}, "xbm": {"image/x-xbitmap"}, "xpm": {"image/x-xpixmap"}, "ico": {"image/vnd.microsoft.icon", "image/x-icon"},
	"qoi": {"image/qoi"}, "pic": {"image/x-pict"}, "pict": {"image/x-pict"}, "ras": {"image/x-cmu-raster"},
	// camera raw (freedesktop shared-mime-info names)
	"arw": {"image/x-sony-arw"}, "srf": {"image/x-sony-srf"}, "sr2": {"image/x-sony-sr2"},
	"cr2": {"image/x-canon-cr2"}, "cr3": {"image/x-canon-cr3"}, "crw": {"image/x-canon-crw"},
	"nef": {"image/x-nikon-nef"}, "nrw": {"image/x-nikon-nrw"}, "dng": {"image/x-adobe-dng"},
	"raf": {"image/x-fuji-raf"}, "orf": {"image/x-olympus-orf"}, "rw2": {"image/x-panasonic-rw2"}, "rwl": {"image/x-panasonic-rw2"},
	"pef": {"image/x-pentax-pef"}, "srw": {"image/x-samsung-srw"}, "3fr": {"image/x-hasselblad-3fr"}, "fff": {"image/x-hasselblad-fff"},
	"iiq": {"image/x-phaseone-iiq"}, "mef": {"image/x-mamiya-mef"}, "mos": {"image/x-leaf-mos"}, "mrw": {"image/x-minolta-mrw"},
	"erf": {"image/x-epson-erf"}, "kdc": {"image/x-kodak-kdc"}, "dcr": {"image/x-kodak-dcr"}, "k25": {"image/x-kodak-k25"},
	"x3f": {"image/x-sigma-x3f"}, "raw": {"image/x-panasonic-raw", "image/x-dcraw"},
	// cinema raw / playlists
	"r3d": {"video/x-red-r3d"}, "braw": {"video/x-blackmagic-raw"},
	"m3u": {"audio/x-mpegurl", "application/vnd.apple.mpegurl"}, "m3u8": {"application/vnd.apple.mpegurl", "application/x-mpegurl"}, "pls": {"audio/x-scpls"},
}

// MIMETypes returns every MIME type PlayAnything registers for, sorted.
func MIMETypes() []string {
	seen := map[string]bool{}
	var out []string
	for _, list := range mimeByExt {
		for _, m := range list {
			if !seen[m] {
				seen[m] = true
				out = append(out, m)
			}
		}
	}
	sort.Strings(out)
	return out
}

// MIMEFor returns the MIME types for an extension (without dot), or nil.
func MIMEFor(ext string) []string { return mimeByExt[ext] }
