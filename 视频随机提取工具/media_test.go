package main

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"videopicker/internal/picker"
)

func mediaRequest(a *App, method, path, rangeHeader string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, nil)
	if rangeHeader != "" {
		r.Header.Set("Range", rangeHeader)
	}
	r.Header.Set("If-Modified-Since", "Wed, 31 Dec 2099 23:59:59 GMT")
	r.Header.Set("If-None-Match", "*")
	w := httptest.NewRecorder()
	a.assetHandler().ServeHTTP(w, r)
	return w
}

func TestAssetHandlerThumbnails(t *testing.T) {
	a := licensedFixture(t, nil)
	if err := a.SaveThumbnail("开场", "sample.mp4", base64.StdEncoding.EncodeToString(appTestJPEG)); err != nil {
		t.Fatal(err)
	}
	vs, err := a.Videos("开场")
	if err != nil {
		t.Fatal(err)
	}
	path := "/thumbs/" + vs[0].Key + ".jpg"
	for _, method := range []string{http.MethodGet, http.MethodHead} {
		w := mediaRequest(a, method, path, "bytes=0-1")
		if w.Code != http.StatusOK || w.Header().Get("Content-Type") != "image/jpeg" || w.Header().Get("Content-Length") != strconv.Itoa(len(appTestJPEG)) || w.Header().Get("Cache-Control") != "private, max-age=31536000, immutable" {
			t.Fatalf("thumbnail response = %d, %v", w.Code, w.Header())
		}
		if w.Header().Get("Last-Modified") != "" || w.Header().Get("ETag") != "" {
			t.Fatalf("cache validators returned: %v", w.Header())
		}
		want := appTestJPEG
		if method == http.MethodHead {
			want = nil
		}
		if !bytes.Equal(w.Body.Bytes(), want) {
			t.Fatalf("thumbnail body = %q", w.Body.Bytes())
		}
	}
	for _, path := range []string{
		"/thumbs/bad.jpg", "/thumbs/" + strings.Repeat("A", 40) + ".jpg",
		"/thumbs/" + strings.Repeat("0", 40) + ".jpg", "/thumbs/" + vs[0].Key + ".none",
		"/thumbs/../" + vs[0].Key + ".jpg", "/thumbs/extra/" + vs[0].Key + ".jpg",
	} {
		if w := mediaRequest(a, http.MethodGet, path, ""); w.Code != http.StatusNotFound {
			t.Fatalf("invalid thumbnail %q = %d", path, w.Code)
		}
	}
	if w := mediaRequest(a, http.MethodPost, path, ""); w.Code != http.StatusNotFound {
		t.Fatalf("POST = %d", w.Code)
	}
}

func TestAssetHandlerMediaRanges(t *testing.T) {
	a := licensedFixture(t, nil)
	root := a.store.Settings().MaterialRoot
	data := make([]byte, mediaChunk+128)
	for i := range data {
		data[i] = byte(i % 251)
	}
	if err := os.WriteFile(filepath.Join(root, "开场", "large.mp4"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	path := "/media/" + url.PathEscape("开场") + "/large.mp4"
	for _, tc := range []struct {
		name, header  string
		start, length int
		status        int
	}{
		{"open range", "bytes=0-", 0, mediaChunk, http.StatusPartialContent},
		{"bounded middle", "bytes=13-29", 13, 17, http.StatusPartialContent},
		{"middle chunk", "bytes=64-", 64, mediaChunk, http.StatusPartialContent},
		{"end clamped", "bytes=1048600-9999999", 1048600, len(data) - 1048600, http.StatusPartialContent},
		{"suffix", "bytes=-64", len(data) - 64, 64, http.StatusPartialContent},
		{"large suffix", "bytes=-9999999", 0, mediaChunk, http.StatusPartialContent},
		{"several ranges", "bytes=5-8, 0-1", 5, 4, http.StatusPartialContent},
		{"large no range", "", 0, mediaChunk, http.StatusPartialContent},
		{"past end", fmt.Sprintf("bytes=%d-", len(data)), 0, 0, http.StatusRequestedRangeNotSatisfiable},
		{"backwards", "bytes=20-19", 0, 0, http.StatusRequestedRangeNotSatisfiable},
		{"malformed", "bytes=invalid", 0, 0, http.StatusRequestedRangeNotSatisfiable},
		{"wrong unit", "items=0-1", 0, 0, http.StatusRequestedRangeNotSatisfiable},
		{"no bounds", "bytes=-", 0, 0, http.StatusRequestedRangeNotSatisfiable},
		{"zero suffix", "bytes=-0", 0, 0, http.StatusRequestedRangeNotSatisfiable},
		{"negative", "bytes=-1-2", 0, 0, http.StatusRequestedRangeNotSatisfiable},
		{"plus sign", "bytes=+1-2", 0, 0, http.StatusRequestedRangeNotSatisfiable},
		{"overflow", "bytes=999999999999999999999-", 0, 0, http.StatusRequestedRangeNotSatisfiable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, method := range []string{http.MethodGet, http.MethodHead} {
				w := mediaRequest(a, method, path, tc.header)
				contentRange := fmt.Sprintf("bytes %d-%d/%d", tc.start, tc.start+tc.length-1, len(data))
				if tc.status == http.StatusRequestedRangeNotSatisfiable {
					contentRange = fmt.Sprintf("bytes */%d", len(data))
				}
				if w.Code != tc.status || w.Header().Get("Content-Range") != contentRange || w.Header().Get("Content-Length") != strconv.Itoa(tc.length) || w.Header().Get("Content-Type") != "video/mp4" || w.Header().Get("Accept-Ranges") != "bytes" || w.Header().Get("Cache-Control") != "no-store" {
					t.Fatalf("%s range response = %d, %v", method, w.Code, w.Header())
				}
				if w.Header().Get("Last-Modified") != "" || w.Header().Get("ETag") != "" {
					t.Fatalf("cache validators returned: %v", w.Header())
				}
				want := data[tc.start : tc.start+tc.length]
				if method == http.MethodHead {
					want = nil
				}
				if w.Body.Len() > mediaChunk || !bytes.Equal(w.Body.Bytes(), want) {
					t.Fatalf("%s body length=%d, want=%d", method, w.Body.Len(), len(want))
				}
			}
		})
	}
}

func TestAssetHandlerSmallMediaTypesAndPaths(t *testing.T) {
	a := licensedFixture(t, nil)
	dir := filepath.Join(a.store.Settings().MaterialRoot, "开场")
	for _, tc := range []struct{ name, contentType string }{
		{"small.mp4", "video/mp4"}, {"small.m4v", "video/mp4"}, {"small.MOV", "video/mp4"},
		{"small.webm", "video/webm"}, {"small.mkv", "video/x-matroska"}, {"small.avi", "application/octet-stream"},
		{"有 空格.mp4", "video/mp4"},
	} {
		if err := os.WriteFile(filepath.Join(dir, tc.name), []byte("small video"), 0o644); err != nil {
			t.Fatal(err)
		}
		path := "/media/" + url.PathEscape("开场") + "/" + url.PathEscape(tc.name)
		for _, method := range []string{http.MethodGet, http.MethodHead} {
			w := mediaRequest(a, method, path, "")
			if w.Code != http.StatusOK || w.Header().Get("Content-Type") != tc.contentType || w.Header().Get("Content-Length") != "11" || w.Header().Get("Content-Range") != "" || w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("Accept-Ranges") != "bytes" {
				t.Fatalf("%s small response = %d, %v", method, w.Code, w.Header())
			}
			want := "small video"
			if method == http.MethodHead {
				want = ""
			}
			if w.Body.String() != want {
				t.Fatalf("small body = %q", w.Body.String())
			}
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "empty.mp4"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if w := mediaRequest(a, http.MethodGet, "/media/开场/empty.mp4", ""); w.Code != http.StatusOK || w.Body.Len() != 0 || w.Header().Get("Content-Length") != "0" {
		t.Fatalf("empty video = %d, %v", w.Code, w.Header())
	}
	if w := mediaRequest(a, http.MethodGet, "/media/开场/empty.mp4", "bytes=0-"); w.Code != http.StatusRequestedRangeNotSatisfiable || w.Header().Get("Content-Range") != "bytes */0" {
		t.Fatalf("empty range = %d, %v", w.Code, w.Header())
	}
	for _, path := range []string{
		"/unknown", "/media/unknown/sample.mp4", "/media/开场/unknown.mp4", "/media/开场/sample.mp4/extra",
		"/media/../sample.mp4", "/media/开场/..", "/media/开场/../sample.mp4", "/media/开场/%2e%2e",
		"/media//sample.mp4", "/media/开场/", "/media/开场", "/media/开场/child%2fsample.mp4", "/media/开场/..%5csample.mp4",
	} {
		if w := mediaRequest(a, http.MethodGet, path, ""); w.Code != http.StatusNotFound {
			t.Fatalf("invalid path %q = %d", path, w.Code)
		}
	}
	if w := mediaRequest(a, http.MethodPost, "/media/开场/sample.mp4", ""); w.Code != http.StatusNotFound {
		t.Fatalf("POST media = %d", w.Code)
	}
}

func TestAssetHandlerDisabledOrMissingStore(t *testing.T) {
	a := licensedFixture(t, nil)
	if err := a.SaveThumbnail("开场", "sample.mp4", base64.StdEncoding.EncodeToString(appTestJPEG)); err != nil {
		t.Fatal(err)
	}
	vs, err := a.Videos("开场")
	if err != nil {
		t.Fatal(err)
	}
	a.store.Update(func(s *picker.Settings) { s.Thumbnails = false })
	for _, a := range []*App{a, NewApp(nil, nil, nil)} {
		for _, path := range []string{"/media/开场/sample.mp4", "/thumbs/" + vs[0].Key + ".jpg"} {
			if w := mediaRequest(a, http.MethodGet, path, ""); w.Code != http.StatusNotFound {
				t.Fatalf("disabled/missing asset %s = %d", path, w.Code)
			}
		}
	}
}
