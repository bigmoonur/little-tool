// Package picker 实现视频随机提取：扫描分类、随机抽取、暂存复制与失败回滚。
package picker

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// VideoExtensions 是按扩展名识别的视频格式，不检查编码。
var VideoExtensions = map[string]bool{
	".mp4": true, ".mov": true, ".mkv": true, ".avi": true, ".wmv": true, ".flv": true, ".webm": true,
	".m4v": true, ".mpeg": true, ".mpg": true, ".ts": true, ".mts": true, ".m2ts": true, ".3gp": true,
}

// TempPrefix 是复制时在目标根目录里创建的临时文件夹前缀。
const TempPrefix = ".video-picker-"

// UserError 是可以直接展示给用户的错误。
type UserError struct{ Msg string }

func (e *UserError) Error() string { return e.Msg }

func userErr(format string, args ...any) error { return &UserError{Msg: fmt.Sprintf(format, args...)} }

type video struct {
	name string
	path string
	size int64
	mod  time.Time
}

// Category 是素材根目录下的一个子文件夹。
type Category struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
	Bytes int64  `json:"bytes"`
}

// Destination 是目标根目录下可以放入视频的子文件夹。
type Destination struct {
	Name   string `json:"name"`
	Videos int    `json:"videos"`
}

// 符号链接、目录联接等不作为分类或目标文件夹。
func isPlainDir(fi fs.FileInfo) bool {
	return fi.IsDir() && fi.Mode()&(fs.ModeSymlink|fs.ModeIrregular) == 0
}

func isPlainFile(fi fs.FileInfo) bool { return fi.Mode().IsRegular() }

func isVideo(name string) bool { return VideoExtensions[strings.ToLower(filepath.Ext(name))] }

func fold(s string) string { return strings.ToLower(s) }

func sortNames[T any](items []T, name func(T) string) {
	sort.SliceStable(items, func(i, j int) bool {
		a, b := name(items[i]), name(items[j])
		if fold(a) != fold(b) {
			return fold(a) < fold(b)
		}
		return a < b
	})
}

// listVideos 列出文件夹直接包含的视频文件，不递归。
func listVideos(dir string) ([]video, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []video
	for _, e := range entries {
		if !isVideo(e.Name()) {
			continue
		}
		fi, err := os.Lstat(filepath.Join(dir, e.Name()))
		if err != nil || !isPlainFile(fi) {
			continue
		}
		out = append(out, video{name: e.Name(), path: filepath.Join(dir, e.Name()), size: fi.Size(), mod: fi.ModTime()})
	}
	sortNames(out, func(v video) string { return v.name })
	return out, nil
}

// plainSubdirs 列出直接子文件夹，跳过程序自己的临时文件夹。
func plainSubdirs(root string) ([]string, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), TempPrefix) {
			continue
		}
		fi, err := os.Lstat(filepath.Join(root, e.Name()))
		if err == nil && isPlainDir(fi) {
			out = append(out, e.Name())
		}
	}
	sortNames(out, func(s string) string { return s })
	return out, nil
}

func checkRoot(root, what string) error {
	if root == "" {
		return userErr("请先选择%s", what)
	}
	fi, err := os.Stat(root)
	if err != nil || !fi.IsDir() {
		return userErr("%s不存在或无法访问，请重新选择", what)
	}
	return nil
}

func scan(root string) ([]Category, map[string][]video, error) {
	if err := checkRoot(root, "素材根目录"); err != nil {
		return nil, nil, err
	}
	dirs, err := plainSubdirs(root)
	if err != nil {
		return nil, nil, userErr("无法读取素材根目录：%v", err)
	}
	cats := make([]Category, 0, len(dirs))
	files := make(map[string][]video, len(dirs))
	for _, name := range dirs {
		vs, err := listVideos(filepath.Join(root, name))
		if err != nil {
			return nil, nil, userErr("无法读取分类「%s」：%v", name, err)
		}
		c := Category{Name: name, Count: len(vs)}
		for _, v := range vs {
			c.Bytes += v.size
		}
		cats = append(cats, c)
		files[name] = vs
	}
	return cats, files, nil
}

// ScanCategories 读取素材根目录下的分类及每类视频数量。
func ScanCategories(root string) ([]Category, error) {
	cats, _, err := scan(root)
	return cats, err
}

// ListDestinations 读取目标根目录下的子文件夹及其中已有的视频数。
func ListDestinations(root string) ([]Destination, error) {
	if err := checkRoot(root, "目标根目录"); err != nil {
		return nil, err
	}
	dirs, err := plainSubdirs(root)
	if err != nil {
		return nil, userErr("无法读取目标根目录：%v", err)
	}
	out := make([]Destination, 0, len(dirs))
	for _, name := range dirs {
		vs, _ := listVideos(filepath.Join(root, name))
		out = append(out, Destination{Name: name, Videos: len(vs)})
	}
	return out, nil
}

// Leftovers 返回上次复制意外中断时留下的临时文件夹。
func Leftovers(outputRoot string) []string {
	entries, err := os.ReadDir(outputRoot)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() && strings.HasPrefix(e.Name(), TempPrefix) {
			out = append(out, filepath.Join(outputRoot, e.Name()))
		}
	}
	return out
}

var badChars = regexp.MustCompile(`[<>:"/\\|?*\x00-\x1f]`)
var reserved = regexp.MustCompile(`^(CON|PRN|AUX|NUL|COM[1-9¹²³]|LPT[1-9¹²³])$`)

// ValidateFolderName 按 Windows 的规则检查文件夹名称。
func ValidateFolderName(name string) error {
	switch {
	case strings.TrimSpace(name) == "":
		return userErr("请输入文件夹名称")
	case name == "." || name == ".." || strings.HasSuffix(name, " ") || strings.HasSuffix(name, "."):
		return userErr("文件夹名称不能是 . 或 ..，也不能以空格或句点结尾")
	case badChars.MatchString(name):
		return userErr(`名称不能包含以下字符：< > : " / \ | ? *`)
	case strings.HasPrefix(name, TempPrefix):
		return userErr("这个名称由程序用于临时文件，请换一个")
	case len([]rune(name)) > 120:
		return userErr("文件夹名称太长")
	}
	stem := strings.ToUpper(strings.TrimRight(strings.SplitN(name, ".", 2)[0], " "))
	if reserved.MatchString(stem) {
		return userErr("这个名称是 Windows 保留名称，请换一个")
	}
	return nil
}

func absClean(p string) string {
	if a, err := filepath.Abs(p); err == nil {
		p = a
	}
	if r, err := filepath.EvalSymlinks(p); err == nil {
		p = r
	}
	return filepath.Clean(p)
}

// within 判断 child 是否等于或位于 parent 之内（不区分大小写，与 Windows 一致）。
func within(parent, child string) bool {
	rel, err := filepath.Rel(fold(parent), fold(child))
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// CheckSeparate 要求素材根目录和目标根目录相互独立。
func CheckSeparate(materialRoot, outputRoot string) error {
	if materialRoot == "" || outputRoot == "" {
		return nil
	}
	a, b := absClean(materialRoot), absClean(outputRoot)
	if within(a, b) || within(b, a) {
		return userErr("素材根目录和目标根目录必须是两个独立的文件夹，不能相同或互相包含")
	}
	return nil
}

// ResolveDestination 返回目标根目录下名为 folder 的子文件夹路径，并确认它存在且就在根目录里。
func ResolveDestination(outputRoot, folder string) (string, error) {
	if folder == "" {
		return "", userErr("请选择或新建一个放入视频的文件夹")
	}
	if err := ValidateFolderName(folder); err != nil {
		return "", err
	}
	if err := checkRoot(outputRoot, "目标根目录"); err != nil {
		return "", err
	}
	dest := filepath.Join(outputRoot, folder)
	fi, err := os.Lstat(dest)
	if err != nil || !isPlainDir(fi) {
		return "", userErr("文件夹「%s」不存在了，请重新选择或新建", folder)
	}
	if !strings.EqualFold(absClean(filepath.Dir(dest)), absClean(outputRoot)) {
		return "", userErr("目标文件夹必须直接位于目标根目录中")
	}
	return dest, nil
}

// AccountResolver 预留给以后接入处理程序的账号信息：把账号信息映射为目标文件夹名称。
// 当前界面由用户手动选择文件夹，没有使用它。
type AccountResolver func(info map[string]string) (string, error)

// ResolveAccountDestination 用账号信息找到目标文件夹。
func ResolveAccountDestination(outputRoot string, info map[string]string, resolve AccountResolver) (string, error) {
	if resolve == nil {
		return "", userErr("账号匹配接口尚未配置，请手动选择目标文件夹")
	}
	name, err := resolve(info)
	if err != nil {
		return "", err
	}
	return ResolveDestination(outputRoot, name)
}

// CreateFolder 在目标根目录下新建文件夹；同名（不区分大小写）已存在时报错。
func CreateFolder(outputRoot, name string) error {
	if err := ValidateFolderName(name); err != nil {
		return err
	}
	if err := checkRoot(outputRoot, "目标根目录"); err != nil {
		return err
	}
	entries, err := os.ReadDir(outputRoot)
	if err != nil {
		return userErr("无法读取目标根目录：%v", err)
	}
	for _, e := range entries {
		if strings.EqualFold(e.Name(), name) {
			return userErr("「%s」已经存在，可以直接选择它或换一个名称", e.Name())
		}
	}
	if err := os.Mkdir(filepath.Join(outputRoot, name), 0o755); err != nil {
		if errors.Is(err, fs.ErrExist) {
			return userErr("「%s」已经存在", name)
		}
		return userErr("新建文件夹失败：%v", err)
	}
	return nil
}
