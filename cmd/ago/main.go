// Package main 是 ago 程序入口。
package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"ago/internal/agent"
	"ago/internal/base"
	"ago/internal/config"
	"ago/internal/provider"
	"ago/internal/transport"
)

func main() {
	// 命令行参数
	configPath := flag.String("config", "opencode.json", "配置文件路径")
	debug := flag.Bool("debug", false, "启用调试日志")
	flag.Parse()

	base.Debug = *debug

	// 加载配置
	cfg, err := config.Load(*configPath)
	if err != nil {
		// 配置文件不存在时，尝试在工作目录或用户目录查找
		if os.IsNotExist(err) {
			altPath := findConfigFile()
			if altPath != "" {
				cfg, err = config.Load(altPath)
			}
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "加载配置失败: %v\n", err)
			fmt.Fprintf(os.Stderr, "请创建 opencode.json 配置文件，示例:\n%s\n", exampleConfig)
			os.Exit(1)
		}
	}

	if cfg.Model == "" {
		fmt.Fprintln(os.Stderr, "错误: opencode.json 中未配置 model 字段")
		fmt.Fprintf(os.Stderr, "示例配置:\n%s\n", exampleConfig)
		os.Exit(1)
	}

	base.Logf("配置加载成功: model=%s, agents=%d", cfg.Model, len(cfg.Agents))

	// 创建 context，监听中断信号
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		base.Logf("收到中断信号，正在退出...")
		cancel()
	}()

	// 初始化通信中枢
	hub := transport.NewHub()

	// 初始化 provider 工厂
	factory := provider.NewFactory(cfg)

	// 创建主代理
	const mainAgentID = "main"
	mainAgent, err := agent.New(
		mainAgentID,
		cfg.Model,
		hub,
		factory,
		cfg,
		agent.WithMode("primary"),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "创建主代理失败: %v\n", err)
		os.Exit(1)
	}

	// 初始化子代理管理器（注入 task 工具处理器）
	subManager := agent.NewSubagentManager(hub, factory, cfg, mainAgentID)
	defer subManager.StopAll()

	// 启动主代理
	if err := mainAgent.Start(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "启动主代理失败: %v\n", err)
		os.Exit(1)
	}
	defer mainAgent.Stop()

	base.Logf("ago 已启动，输入消息开始对话（Ctrl+C 退出）")
	fmt.Println("================================")
	fmt.Println("  ago — 极简高并发 Agent 底座")
	fmt.Println("================================")
	if len(cfg.Agents) > 0 {
		fmt.Printf("可用子代理: %v\n", agentKeys(cfg.Agents))
	}
	fmt.Println()

	// 主循环：读取用户输入 → 发送到主代理 → 等待回复
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	// 注册主代理回复 channel
	replyCh := hub.Register(mainAgentID+"-console", 1)
	defer hub.Unregister(mainAgentID + "-console")

	for {
		fmt.Print("> ")
		if !scanner.Scan() {
			break
		}
		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			continue
		}
		if input == "/quit" || input == "/exit" {
			break
		}
		if input == "/agents" {
			fmt.Printf("配置的子代理: %v\n", agentKeys(cfg.Agents))
			continue
		}
		if input == "/help" {
			printHelp()
			continue
		}

		// 发送到主代理
		if !hub.Send(ctx, transport.Message{
			SrcAgentID: mainAgentID + "-console",
			DstAgentID: mainAgentID,
			Type:       transport.MsgTypeUser,
			Payload:    input,
		}) {
			fmt.Fprintln(os.Stderr, "错误: 无法发送消息到主代理")
			continue
		}

		// 等待回复
		select {
		case reply, ok := <-replyCh:
			if !ok {
				fmt.Fprintln(os.Stderr, "错误: 主代理 channel 已关闭")
				return
			}
			switch reply.Type {
			case transport.MsgTypeAssistant:
				fmt.Printf("\n%s\n\n", reply.Payload.(string))
			case transport.MsgTypeError:
				fmt.Fprintf(os.Stderr, "\n错误: %s\n\n", reply.Payload.(string))
			}
		case <-ctx.Done():
			return
		}
	}

	base.Logf("ago 已退出")
}

// findConfigFile 在常见位置查找 opencode.json。
func findConfigFile() string {
	candidates := []string{
		"opencode.json",
		".opencode/opencode.json",
	}
	// 用户目录
	if home, err := os.UserHomeDir(); err == nil {
		candidates = append(candidates,
			filepath.Join(home, "opencode.json"),
			filepath.Join(home, ".opencode", "opencode.json"),
			filepath.Join(home, ".config", "opencode", "opencode.json"),
		)
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return ""
}

func agentKeys(m map[string]config.AgentConfig) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

func printHelp() {
	fmt.Println("命令:")
	fmt.Println("  /agents - 列出可用子代理")
	fmt.Println("  /help   - 显示帮助")
	fmt.Println("  /quit   - 退出")
	fmt.Println()
}

const exampleConfig = `{
  "model": "opencode/deepseek-v4-flash-free",
  "provider": {
    "opencode": {
      "name": "OpenCode Zen",
      "options": {
        "baseURL": "https://opencode.ai/zen/v1"
      },
      "models": {
        "deepseek-v4-flash-free": {
          "name": "DeepSeek V4 Flash Free",
          "cost": { "input": 0, "output": 0 }
        }
      }
    }
  },
  "agents": {
    "research": {
      "model": "opencode/deepseek-v4-flash-free",
      "system": "你是一个调研助手",
      "mode": "subagent"
    }
  }
}`
