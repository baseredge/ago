// Package tool 实现核心工具：read/write/edit/task。
// 极简版去掉 location/permission/file-diff 等复杂度，保留核心 IO 逻辑。
package tool

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ReadInput 是 read 工具的输入参数。
type ReadInput struct {
	Path string `json:"path"`
}

// ReadResult 是 read 工具的结果。
type ReadResult struct {
	// Content 文本内容（文本文件）
	Content string `json:"content,omitempty"`
	// ImageBase64 图片 base64（图片文件）
	ImageBase64 string `json:"image_base64,omitempty"`
	// MIME 文件 MIME 类型
	MIME string `json:"mime,omitempty"`
	// Entries 目录条目列表（目录）
	Entries []string `json:"entries,omitempty"`
	// IsDir 是否为目录
	IsDir bool `json:"is_dir"`
}

// Read 读取文件或列出目录。
// 参考 packages/core/src/tool/read.ts，极简版去掉分页/权限/location 解析。
func Read(input ReadInput) (*ReadResult, error) {
	path := input.Path
	if path == "" {
		return nil, fmt.Errorf("path is required")
	}

	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("stat %s: %w", path, err)
	}

	if info.IsDir() {
		entries, err := os.ReadDir(path)
		if err != nil {
			return nil, fmt.Errorf("read dir %s: %w", path, err)
		}
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			name := e.Name()
			if e.IsDir() {
				name += "/"
			}
			names = append(names, name)
		}
		return &ReadResult{Entries: names, IsDir: true}, nil
	}

	// 文件：判断是否为支持的图片
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read file %s: %w", path, err)
	}

	mime := detectMIME(path, data)
	if strings.HasPrefix(mime, "image/") {
		return &ReadResult{
			ImageBase64: base64.StdEncoding.EncodeToString(data),
			MIME:        mime,
		}, nil
	}

	// 文本文件
	return &ReadResult{Content: string(data), MIME: mime}, nil
}

// detectMIME 根据扩展名和文件头检测 MIME 类型。
func detectMIME(path string, data []byte) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".go":
		return "text/x-go"
	case ".js":
		return "text/javascript"
	case ".ts":
		return "text/typescript"
	case ".json":
		return "application/json"
	case ".md":
		return "text/markdown"
	default:
		return "text/plain"
	}
}
