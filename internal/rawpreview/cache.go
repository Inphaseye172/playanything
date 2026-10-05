package rawpreview

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// CacheKey identifies a RAW file by path, size and modification time so the
// cached preview is invalidated when the file changes.
func CacheKey(path string) (string, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	h := sha1.New()
	fmt.Fprintf(h, "%s|%d|%d", abs, fi.Size(), fi.ModTime().UnixNano())
	return hex.EncodeToString(h.Sum(nil))[:24], nil
}

// Cached returns the path of a cached preview JPEG for the RAW file at path,
// extracting it into cacheDir when missing. The rotation (degrees) is encoded
// in the cached file name so no sidecar is needed.
func Cached(cacheDir, path string) (jpgPath string, rotate int, err error) {
	key, err := CacheKey(path)
	if err != nil {
		return "", 0, err
	}
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return "", 0, err
	}
	matches, _ := filepath.Glob(filepath.Join(cacheDir, key+".r*.jpg"))
	if len(matches) == 1 {
		return matches[0], rotateFromName(matches[0]), nil
	}
	res, err := ExtractFile(path)
	if err != nil {
		return "", 0, err
	}
	jpgPath = filepath.Join(cacheDir, fmt.Sprintf("%s.r%d.jpg", key, res.Rotate()))
	tmp := jpgPath + ".tmp"
	if err := os.WriteFile(tmp, res.JPEG, 0o644); err != nil {
		return "", 0, err
	}
	if err := os.Rename(tmp, jpgPath); err != nil {
		os.Remove(tmp)
		return "", 0, err
	}
	return jpgPath, res.Rotate(), nil
}

func rotateFromName(p string) int {
	base := filepath.Base(p)
	i := strings.LastIndex(base, ".r")
	if i < 0 {
		return 0
	}
	n, _ := strconv.Atoi(strings.TrimSuffix(base[i+2:], ".jpg"))
	return n
}
