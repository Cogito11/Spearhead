package protocol

import (
	"encoding/json"
	"errors"
	"path"
	"strings"
	"unicode"
)

const KindFile = "file"
const MaxFileSize int64 = 256 << 20
const FileChunkSize = 64 << 10

type FileInfo struct {
	Name   string `json:"name"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

func CleanFileName(name string) string {
	name = path.Base(strings.ReplaceAll(name, "\\", "/"))
	name = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, name)
	name = strings.TrimSpace(name)
	for len(name) > 240 {
		name = string([]rune(name)[:len([]rune(name))-1])
	}
	if name == "" || name == "." || name == ".." {
		return "attachment"
	}
	return name
}

func ParseFile(body string) (FileInfo, error) {
	var f FileInfo
	if json.Unmarshal([]byte(body), &f) != nil || f.Name != CleanFileName(f.Name) || f.Size < 0 || f.Size > MaxFileSize || !hashRe.MatchString(f.SHA256) {
		return FileInfo{}, errors.New("invalid file information")
	}
	return f, nil
}
