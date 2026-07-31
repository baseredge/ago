package tool

import (
	"fmt"
	"os"
	"strings"
)

// EditInput 是 edit 工具的输入参数。
type EditInput struct {
	Path       string `json:"path"`
	OldString  string `json:"old_string"`
	NewString  string `json:"new_string"`
	ReplaceAll bool   `json:"replace_all,omitempty"`
}

// EditResult 是 edit 工具的结果。
type EditResult struct {
	Path         string `json:"path"`
	Replacements int    `json:"replacements"`
}

// Edit 通过字符串替换编辑文件。
// 参考 packages/core/src/tool/edit.ts，极简版去掉 diff/BOM/行尾规范化。
func Edit(input EditInput) (*EditResult, error) {
	path := input.Path
	if path == "" {
		return nil, fmt.Errorf("path is required")
	}
	if input.OldString == input.NewString {
		return nil, fmt.Errorf("old_string and new_string must differ")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	content := string(data)

	var newContent string
	var count int
	if input.ReplaceAll {
		newContent = strings.ReplaceAll(content, input.OldString, input.NewString)
		count = strings.Count(content, input.OldString)
	} else {
		count = strings.Count(content, input.OldString)
		if count == 0 {
			return nil, fmt.Errorf("old_string not found in %s", path)
		}
		if count > 1 {
			return nil, fmt.Errorf("old_string appears %d times in %s, set replace_all=true or provide more context", count, path)
		}
		newContent = strings.Replace(content, input.OldString, input.NewString, 1)
	}

	if count == 0 {
		return nil, fmt.Errorf("old_string not found in %s", path)
	}

	if err := os.WriteFile(path, []byte(newContent), 0644); err != nil {
		return nil, fmt.Errorf("write %s: %w", path, err)
	}

	return &EditResult{
		Path:         path,
		Replacements: count,
	}, nil
}
