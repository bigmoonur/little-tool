package main

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"videopicker/internal/picker"
)

const mediaChunk = 1 << 20

// assetHandler 给界面提供缩略图和用来生成缩略图的视频数据。
func (a *App) assetHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if (r.Method != http.MethodGet && r.Method != http.MethodHead) || a.store == nil {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		s := a.store.Settings()
		if !s.Thumbnails {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		var path, contentType string
		thumb := strings.HasPrefix(r.URL.Path, "/thumbs/")
		switch {
		case thumb && strings.HasSuffix(r.URL.Path, ".jpg"):
			key := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/thumbs/"), ".jpg")
			var ok bool
			path, ok = a.store.ThumbPath(key)
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			contentType = "image/jpeg"
		case strings.HasPrefix(r.URL.Path, "/media/"):
			parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/media/"), "/")
			if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			var err error
			path, err = picker.VideoPath(s.MaterialRoot, parts[0], parts[1])
			if err != nil {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			contentType = mediaType(path)
		default:
			w.WriteHeader(http.StatusNotFound)
			return
		}
		file, err := os.Open(path)
		if err != nil {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		defer file.Close()
		fi, err := file.Stat()
		if err != nil || !fi.Mode().IsRegular() || (thumb && fi.Size() > picker.MaxThumbBytes) {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		start, length, status := int64(0), fi.Size(), http.StatusOK
		w.Header().Set("Content-Type", contentType)
		if thumb {
			w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("Accept-Ranges", "bytes")
			start, length, status = mediaRange(r.Header.Get("Range"), fi.Size())
			if status == http.StatusRequestedRangeNotSatisfiable {
				w.Header().Set("Content-Range", "bytes */"+strconv.FormatInt(fi.Size(), 10))
			} else if status == http.StatusPartialContent {
				w.Header().Set("Content-Range", "bytes "+strconv.FormatInt(start, 10)+"-"+strconv.FormatInt(start+length-1, 10)+"/"+strconv.FormatInt(fi.Size(), 10))
			}
		}
		w.Header().Set("Content-Length", strconv.FormatInt(length, 10))
		// 不设置缓存验证头，也不使用 ServeContent，避免 Windows 上的 304 挂起。
		w.WriteHeader(status)
		if r.Method == http.MethodHead || length == 0 {
			return
		}
		_, _ = file.Seek(start, io.SeekStart)
		_, _ = io.CopyN(w, file, length)
	})
}

func mediaType(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".mp4", ".m4v", ".mov":
		return "video/mp4"
	case ".webm":
		return "video/webm"
	case ".mkv":
		return "video/x-matroska"
	default:
		return "application/octet-stream"
	}
}

// mediaRange 每次最多返回一块，Wails 在 Windows 上会把整个响应缓存在内存。
func mediaRange(header string, size int64) (start, length int64, status int) {
	if header == "" && size <= mediaChunk {
		return 0, size, http.StatusOK
	}
	if header == "" {
		header = "bytes=0-"
	}
	if !strings.HasPrefix(header, "bytes=") || size == 0 {
		return 0, 0, http.StatusRequestedRangeNotSatisfiable
	}
	first := strings.TrimSpace(strings.SplitN(strings.TrimPrefix(header, "bytes="), ",", 2)[0])
	parts := strings.Split(first, "-")
	if len(parts) != 2 {
		return 0, 0, http.StatusRequestedRangeNotSatisfiable
	}
	end := size - 1
	if parts[0] == "" {
		n, ok := rangeNumber(parts[1])
		if !ok || n == 0 {
			return 0, 0, http.StatusRequestedRangeNotSatisfiable
		}
		if n < size {
			start = size - n
		}
	} else {
		var ok bool
		start, ok = rangeNumber(parts[0])
		if !ok || start >= size {
			return 0, 0, http.StatusRequestedRangeNotSatisfiable
		}
		if parts[1] != "" {
			requestedEnd, ok := rangeNumber(parts[1])
			if !ok || requestedEnd < start {
				return 0, 0, http.StatusRequestedRangeNotSatisfiable
			}
			if requestedEnd < end {
				end = requestedEnd
			}
		}
	}
	length = end - start + 1
	if length > mediaChunk {
		length = mediaChunk
	}
	return start, length, http.StatusPartialContent
}

func rangeNumber(s string) (int64, bool) {
	if s == "" {
		return 0, false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, false
		}
	}
	n, err := strconv.ParseInt(s, 10, 64)
	return n, err == nil
}
