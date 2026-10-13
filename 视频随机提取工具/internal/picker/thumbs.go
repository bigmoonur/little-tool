package picker

import (
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const MaxThumbBytes = 512 << 10

// ThumbKey 是缩略图的缓存名：素材根目录、分类、文件名、大小、修改时间任何一项变了，都是另一个视频。
func ThumbKey(root, category, name string, size int64, mod time.Time) string {
	parts := []string{absClean(root), category, name, strconv.FormatInt(size, 10), strconv.FormatInt(mod.UnixNano(), 10)}
	sum := sha1.Sum([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(sum[:])
}

func validThumbKey(key string) bool {
	if len(key) != 40 {
		return false
	}
	for _, c := range key {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

// LibraryKeys 扫描失败时不返回部分结果，避免离线素材的缓存被误删。
func LibraryKeys(root string) (map[string]bool, error) {
	if err := checkRoot(root, "素材根目录"); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, userErr("无法读取素材根目录：%v", err)
	}
	keys := map[string]bool{}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), TempPrefix) {
			continue
		}
		dir := filepath.Join(root, entry.Name())
		fi, err := os.Lstat(dir)
		if err != nil {
			return nil, userErr("无法读取素材根目录：%v", err)
		}
		if !isPlainDir(fi) {
			continue
		}
		files, err := os.ReadDir(dir)
		if err != nil {
			return nil, userErr("无法读取分类「%s」：%v", entry.Name(), err)
		}
		for _, file := range files {
			if !isVideo(file.Name()) {
				continue
			}
			fi, err := os.Lstat(filepath.Join(dir, file.Name()))
			if err != nil {
				return nil, userErr("无法读取分类「%s」：%v", entry.Name(), err)
			}
			if isPlainFile(fi) {
				keys[ThumbKey(root, entry.Name(), file.Name(), fi.Size(), fi.ModTime())] = true
			}
		}
	}
	return keys, nil
}

func (s *Store) ThumbDir() string { return filepath.Join(s.dir, "thumbs") }

// SaveThumb 先写临时文件再替换，界面不会读到半张图片。
func (s *Store) SaveThumb(key string, data []byte) error {
	if !validThumbKey(key) {
		return userErr("缩略图缓存名无效")
	}
	if len(data) > MaxThumbBytes {
		return userErr("缩略图不能超过 512 KiB")
	}
	if len(data) < 3 || data[0] != 0xff || data[1] != 0xd8 || data[2] != 0xff {
		return userErr("缩略图必须是 JPEG 图片")
	}
	s.thumbMu.Lock()
	defer s.thumbMu.Unlock()
	if err := os.MkdirAll(s.ThumbDir(), 0o755); err != nil {
		return userErr("无法保存缩略图：%v", err)
	}
	tmp, err := os.CreateTemp(s.ThumbDir(), key+"-*.tmp")
	if err != nil {
		return userErr("无法保存缩略图：%v", err)
	}
	defer os.Remove(tmp.Name())
	_, err = tmp.Write(data)
	closeErr := tmp.Close()
	if err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(tmp.Name(), filepath.Join(s.ThumbDir(), key+".jpg"))
	}
	if err == nil {
		err = os.Remove(filepath.Join(s.ThumbDir(), key+".none"))
		if errors.Is(err, fs.ErrNotExist) {
			err = nil
		}
	}
	if err != nil {
		return userErr("无法保存缩略图：%v", err)
	}
	return nil
}

// MarkNoThumb 保留成功的缓存，失败标记只用于避免重复尝试。
func (s *Store) MarkNoThumb(key string) error {
	if !validThumbKey(key) {
		return userErr("缩略图缓存名无效")
	}
	s.thumbMu.Lock()
	defer s.thumbMu.Unlock()
	if _, ok := s.thumbPath(key); ok {
		return nil
	}
	if err := os.MkdirAll(s.ThumbDir(), 0o755); err != nil {
		return userErr("无法保存缩略图标记：%v", err)
	}
	f, err := os.OpenFile(filepath.Join(s.ThumbDir(), key+".none"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if errors.Is(err, fs.ErrExist) {
		return nil
	}
	if err == nil {
		err = f.Close()
	}
	if err != nil {
		return userErr("无法保存缩略图标记：%v", err)
	}
	return nil
}

func (s *Store) thumbPath(key string) (string, bool) {
	if !validThumbKey(key) {
		return "", false
	}
	path := filepath.Join(s.ThumbDir(), key+".jpg")
	fi, err := os.Lstat(path)
	if err != nil || !isPlainFile(fi) {
		return "", false
	}
	return path, true
}

func (s *Store) ThumbPath(key string) (path string, ok bool) {
	s.thumbMu.Lock()
	defer s.thumbMu.Unlock()
	return s.thumbPath(key)
}

// ThumbStates 只读一次目录；成功的图片优先于失败标记。
func (s *Store) ThumbStates() map[string]string {
	s.thumbMu.Lock()
	defer s.thumbMu.Unlock()
	states := map[string]string{}
	entries, _ := os.ReadDir(s.ThumbDir())
	for _, e := range entries {
		ext := filepath.Ext(e.Name())
		key := strings.TrimSuffix(e.Name(), ext)
		if !validThumbKey(key) || !e.Type().IsRegular() {
			continue
		}
		switch ext {
		case ".jpg":
			states[key] = "ready"
		case ".none":
			if states[key] == "" {
				states[key] = "none"
			}
		}
	}
	return states
}

// PruneThumbs 同时删除残留临时文件，删除失败的文件下次再试。
func (s *Store) PruneThumbs(keep map[string]bool) int {
	s.thumbMu.Lock()
	defer s.thumbMu.Unlock()
	entries, _ := os.ReadDir(s.ThumbDir())
	removed := 0
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		ext := filepath.Ext(e.Name())
		key := strings.TrimSuffix(e.Name(), ext)
		if validThumbKey(key) && (ext == ".jpg" || ext == ".none") && keep[key] {
			continue
		}
		if os.Remove(filepath.Join(s.ThumbDir(), e.Name())) == nil {
			removed++
		}
	}
	return removed
}

func (s *Store) ClearThumbs() error {
	s.thumbMu.Lock()
	defer s.thumbMu.Unlock()
	if err := os.RemoveAll(s.ThumbDir()); err != nil {
		return userErr("无法清理缩略图：%v", err)
	}
	return nil
}
