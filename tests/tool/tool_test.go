package tool_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"ago/internal/tool"
)

// TestReadFile 测试读取文本文件
func TestReadFile(t *testing.T) {
	// 创建临时文件
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	content := "hello world\n你好世界"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := tool.Read(tool.ReadInput{Path: path})
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	if result.Content != content {
		t.Errorf("Content = %q, want %q", result.Content, content)
	}
	if result.IsDir {
		t.Error("IsDir should be false for file")
	}
}

// TestReadDirectory 测试读取目录
func TestReadDirectory(t *testing.T) {
	dir := t.TempDir()
	// 创建几个文件
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a"), 0644)
	os.Mkdir(filepath.Join(dir, "subdir"), 0755)

	result, err := tool.Read(tool.ReadInput{Path: dir})
	if err != nil {
		t.Fatalf("Read dir failed: %v", err)
	}
	if !result.IsDir {
		t.Error("IsDir should be true for directory")
	}
	if len(result.Entries) != 2 {
		t.Errorf("Entries count = %d, want 2", len(result.Entries))
	}
}

// TestReadNonExistent 测试读取不存在的文件
func TestReadNonExistent(t *testing.T) {
	_, err := tool.Read(tool.ReadInput{Path: "/nonexistent/path/file.txt"})
	if err == nil {
		t.Error("expected error for non-existent file")
	}
}

// TestWriteNewFile 测试写新文件
func TestWriteNewFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "subdir", "new.txt") // 父目录不存在
	content := "new content"

	result, err := tool.Write(tool.WriteInput{Path: path, Content: content})
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	if !result.Created {
		t.Error("Created should be true for new file")
	}
	if result.Bytes != len(content) {
		t.Errorf("Bytes = %d, want %d", result.Bytes, len(content))
	}

	// 验证内容
	data, _ := os.ReadFile(path)
	if string(data) != content {
		t.Errorf("File content = %q, want %q", string(data), content)
	}
}

// TestWriteOverwrite 测试覆盖写
func TestWriteOverwrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "exists.txt")
	os.WriteFile(path, []byte("old"), 0644)

	result, err := tool.Write(tool.WriteInput{Path: path, Content: "new"})
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	if result.Created {
		t.Error("Created should be false for existing file")
	}
}

// TestEditReplace 测试字符串替换
func TestEditReplace(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "edit.txt")
	os.WriteFile(path, []byte("hello world\nfoo bar"), 0644)

	result, err := tool.Edit(tool.EditInput{
		Path:      path,
		OldString: "foo",
		NewString: "baz",
	})
	if err != nil {
		t.Fatalf("Edit failed: %v", err)
	}
	if result.Replacements != 1 {
		t.Errorf("Replacements = %d, want 1", result.Replacements)
	}

	data, _ := os.ReadFile(path)
	if string(data) != "hello world\nbaz bar" {
		t.Errorf("After edit = %q, want %q", string(data), "hello world\nbaz bar")
	}
}

// TestEditMultipleMatches 测试多次匹配报错
func TestEditMultipleMatches(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "multi.txt")
	os.WriteFile(path, []byte("foo foo foo"), 0644)

	_, err := tool.Edit(tool.EditInput{
		Path:      path,
		OldString: "foo",
		NewString: "bar",
	})
	if err == nil {
		t.Error("expected error for multiple matches without replace_all")
	}
}

// TestEditReplaceAll 测试替换全部
func TestEditReplaceAll(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "all.txt")
	os.WriteFile(path, []byte("foo foo foo"), 0644)

	result, err := tool.Edit(tool.EditInput{
		Path:       path,
		OldString:  "foo",
		NewString:  "bar",
		ReplaceAll: true,
	})
	if err != nil {
		t.Fatalf("Edit failed: %v", err)
	}
	if result.Replacements != 3 {
		t.Errorf("Replacements = %d, want 3", result.Replacements)
	}

	data, _ := os.ReadFile(path)
	if string(data) != "bar bar bar" {
		t.Errorf("After edit = %q, want %q", string(data), "bar bar bar")
	}
}

// TestEditNotFound 测试未找到
func TestEditNotFound(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "notfound.txt")
	os.WriteFile(path, []byte("hello"), 0644)

	_, err := tool.Edit(tool.EditInput{
		Path:      path,
		OldString: "nonexistent",
		NewString: "x",
	})
	if err == nil {
		t.Error("expected error for not found")
	}
}

// TestTaskHandlerNotSet 测试 task 处理器未设置
func TestTaskHandlerNotSet(t *testing.T) {
	// 保存原始值并恢复
	original := tool.TaskHandler
	tool.TaskHandler = nil
	defer func() { tool.TaskHandler = original }()

	_, err := tool.Task(context.Background(), tool.TaskInput{
		SubagentName: "test",
		Prompt:       "test",
	})
	if err != tool.ErrTaskHandlerNotSet {
		t.Errorf("error = %v, want %v", err, tool.ErrTaskHandlerNotSet)
	}
}

// TestTaskInputDepthNotSerialized 测试 Depth 字段不参与 JSON 序列化
// （LLM 不感知该字段，由 agent 层内部注入）
func TestTaskInputDepthNotSerialized(t *testing.T) {
	input := tool.TaskInput{
		SubagentName: "research",
		Prompt:       "do something",
		Depth:        5,
	}
	data, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}
	s := string(data)
	if strings.Contains(s, "depth") || strings.Contains(s, "Depth") {
		t.Errorf("Depth should not be serialized, got %s", s)
	}
	// 反序列化后 Depth 应为 0
	var decoded tool.TaskInput
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if decoded.Depth != 0 {
		t.Errorf("decoded Depth = %d, want 0 (not serialized)", decoded.Depth)
	}
}

// TestBashSuccess 测试执行成功命令
func TestBashSuccess(t *testing.T) {
	var cmd string
	if runtime.GOOS == "windows" {
		cmd = "echo hello"
	} else {
		cmd = "echo hello"
	}
	result, err := tool.Bash(tool.BashInput{Command: cmd})
	if err != nil {
		t.Fatalf("Bash failed: %v", err)
	}
	if result.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0", result.ExitCode)
	}
	if !strings.Contains(result.Stdout, "hello") {
		t.Errorf("Stdout = %q, want contains 'hello'", result.Stdout)
	}
}

// TestBashFailure 测试非零退出码
func TestBashFailure(t *testing.T) {
	var cmd string
	if runtime.GOOS == "windows" {
		cmd = "cmd /c exit 42"
	} else {
		cmd = "exit 42"
	}
	result, err := tool.Bash(tool.BashInput{Command: cmd})
	if err != nil {
		t.Fatalf("Bash should not return error for non-zero exit: %v", err)
	}
	if result.ExitCode != 42 {
		t.Errorf("ExitCode = %d, want 42", result.ExitCode)
	}
}

// TestBashEmptyCommand 测试空命令报错
func TestBashEmptyCommand(t *testing.T) {
	_, err := tool.Bash(tool.BashInput{Command: ""})
	if err == nil {
		t.Error("expected error for empty command")
	}
}

// TestBashTimeout 测试超时
func TestBashTimeout(t *testing.T) {
	var cmd string
	if runtime.GOOS == "windows" {
		cmd = "ping -n 10 127.0.0.1 > nul"
	} else {
		cmd = "sleep 10"
	}
	_, err := tool.Bash(tool.BashInput{Command: cmd, TimeoutSec: 1})
	if err == nil {
		t.Error("expected timeout error")
	}
}

// TestBashWorkDir 测试工作目录
func TestBashWorkDir(t *testing.T) {
	dir := t.TempDir()
	var cmd string
	if runtime.GOOS == "windows" {
		cmd = "cd"
	} else {
		cmd = "pwd"
	}
	result, err := tool.Bash(tool.BashInput{Command: cmd, WorkDir: dir})
	if err != nil {
		t.Fatalf("Bash failed: %v", err)
	}
	// 输出应包含目标目录路径（不同平台大小写/斜杠可能不同，用 ToLower 比较）
	if !strings.Contains(strings.ToLower(result.Stdout), strings.ToLower(dir)) {
		t.Errorf("Stdout = %q, want contains %q", result.Stdout, dir)
	}
}
