package tool

import (
	"fmt"
	"os"
	"path/filepath"
)

// WriteInput 是 write 工具的输入参数。
type WriteInput struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

// WriteResult 是 write 工具的结果。
type WriteResult struct {
	Path    string `json:"path"`
	Bytes   int    `json:"bytes"`
	Created bool   `json:"created"`
}

// Write 写文件，自动创建父目录。
// 参考 packages/core/src/tool/write.ts，极简版去掉权限/快照/location。
func Write(input WriteInput) (*WriteResult, error) {
	path := input.Path
	if path == "" {
		return nil, fmt.Errorf("path is required")
	}

	// 确保父目录存在
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("mkdir %s: %w", dir, err)
	}

	// 判断是否为新文件
	_, err := os.Stat(path)
	created := os.IsNotExist(err)

	// 写文件
	data := []byte(input.Content)
	if err := os.WriteFile(path, data, 0644); err != nil {
		return nil, fmt.Errorf("write %s: %w", path, err)
	}

	return &WriteResult{
		Path:    path,
		Bytes:   len(data),
		Created: created,
	}, nil
}
