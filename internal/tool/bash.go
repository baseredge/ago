package tool

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strconv"
	"time"
)

// BashInput 是 bash 工具的输入参数。
type BashInput struct {
	Command string `json:"command"`
	// WorkDir 工作目录（可选，默认当前目录）
	WorkDir string `json:"workdir,omitempty"`
	// TimeoutSec 超时秒数（可选，默认 60）
	TimeoutSec int `json:"timeout_sec,omitempty"`
}

// BashResult 是 bash 工具的结果。
type BashResult struct {
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	ExitCode int    `json:"exit_code"`
}

// defaultBashTimeout bash 工具默认超时，防止阻塞 agent goroutine。
const defaultBashTimeout = 60 * time.Second

// Bash 执行 shell 命令并返回输出。
// 跨平台：Unix 用 sh -c，Windows 用 cmd /c。
// 参考 opencode packages/core/src/tool/bash.ts，极简版去掉权限/持久会话/TTY。
func Bash(input BashInput) (*BashResult, error) {
	if input.Command == "" {
		return nil, fmt.Errorf("command is required")
	}

	timeout := defaultBashTimeout
	if input.TimeoutSec > 0 {
		timeout = time.Duration(input.TimeoutSec) * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "cmd", "/c", input.Command)
		// Windows 上 exec.CommandContext 默认只 kill cmd.exe，孙进程（如 ping）
		// 仍持有 stdio 句柄导致 cmd.Wait() 卡住。用 taskkill /F /T kill 整个进程树。
		cmd.Cancel = func() error {
			return exec.Command("taskkill", "/F", "/T", "/PID", strconv.Itoa(cmd.Process.Pid)).Run()
		}
	} else {
		cmd = exec.CommandContext(ctx, "sh", "-c", input.Command)
	}
	if input.WorkDir != "" {
		cmd.Dir = input.WorkDir
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	// 优先检查 context：cmd.Cancel 可能 kill 进程后 cmd.Run() 返回 nil error
	if ctx.Err() != nil {
		return nil, fmt.Errorf("command timeout after %s", timeout)
	}
	exitCode := 0
	if err != nil {
		// 非零退出码不算致命错误，提取 exit code 返回给 agent
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			return nil, fmt.Errorf("run command: %w", err)
		}
	}

	return &BashResult{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: exitCode,
	}, nil
}
