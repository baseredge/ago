# ago

> A minimalist, high-concurrency AI agent runtime in Go.
> 极简、高并发、配置兼容 opencode.json 的 Agent 底座。

[![Go](https://img.shields.io/badge/Go-1.21+-00ADD8?logo=go)]()
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![Built with TRAE](https://img.shields.io/badge/Built%20with-TRAE%20%2B%20GLM--5.2-blueviolet)]()
[![Inspired by opencode](https://img.shields.io/badge/Inspired%20by-opencode-orange)](https://github.com/sst/opencode)
[![Inspired by Pi](https://img.shields.io/badge/Inspired%20by-Pi-green)](https://github.com/badlogic/pi-mono)

---

## 这是什么

`ago` 是一个极简的通用 AI Agent 底座，用 Go 编写。每个 agent 独立 goroutine + 独立消息 channel，实现超高并发。复用 [opencode](https://github.com/sst/opencode) 的 `opencode.json` 配置格式，开箱即用。

适合作为 **coding agent** / **agentic workflow** / **LLM tool-use** 项目的底层 runtime，也可直接当作 **CLI coding assistant** 使用。

极简工具集的设计思路受 [Pi](https://github.com/badlogic/pi-mono)（minimal terminal coding harness）启发 —— Pi 用 4 个默认工具（read/write/edit/bash）覆盖日常编码，搜索文件/代码全部通过 bash 完成，高度可定制。ago 在此基础上增加 `task` 工具实现子代理并发调用，发挥 Go goroutine 的高并发优势。

**特点：**

- **极简**：单二进制文件，零外部依赖，核心代码精简
- **高并发**：每 agent 一个 goroutine，主代理可并行调用多个子代理
- **配置兼容**：直接复用你现有的 `opencode.json`，无需修改
- **免费可用**：内置 opencode Zen 网关双模式鉴权，未配置 apiKey 时硬编码 `apiKey=public`，零成本调用免费模型
- **本机通信**：agent 间通过 Go channel 通信，零序列化开销

**明确不做**（极简取舍）：MCP、跨机通信、自研 retry/overload 框架、glob/grep 等非核心工具、递归式无限派生、catalog 动态拉取、LSP/formatter/watcher。

---

## 快速开始

### 1. 编译

```powershell
go build -o bin\ago.exe ./cmd/ago
```

### 2. 创建配置（免费模式，无需 API Key）

在工作目录创建 `opencode.json`：

```json
{
  "model": "opencode/deepseek-v4-flash-free",
  "provider": {
    "opencode": {
      "options": { "baseURL": "https://opencode.ai/zen/v1" },
      "models": {
        "deepseek-v4-flash-free": { "cost": { "input": 0, "output": 0 } }
      }
    }
  }
}
```

### 3. 启动

```powershell
.\bin\ago.exe
```

完整使用教程见 [docs/USAGE.md](docs/USAGE.md)。

---

## 核心工具

| 工具 | 作用 |
|------|------|
| `read` | 读文件内容或列目录 |
| `write` | 写文件（自动创建父目录） |
| `edit` | 字符串精确替换（支持多匹配保护 + replace_all） |
| `task` | 调用子代理执行任务（独立 goroutine，跟随父 agent context 生命周期） |
| `bash` | 执行 shell 命令（跨平台，超时由 LLM 通过 timeout_sec 参数控制，Windows 用 taskkill /F /T kill 进程树） |

每个工具均可在配置文件中按 agent 禁用（`tools: {"bash": false}`），详见 [docs/USAGE.md](docs/USAGE.md)。

---

## 支持的 Provider 协议

| 协议 | 覆盖 |
|------|------|
| OpenAI 兼容 | OpenAI / Zen(chat 端点) / OpenRouter / DeepSeek / GLM / Kimi / MiniMax / Grok |
| Anthropic Messages | Anthropic Claude / Zen(messages 端点) |

支持调用的模型系列（通过配置切换，以下为各系列截至 2026 年 7 月的主流版本示例，实际可用模型以 provider 网关为准）：
- **Claude** 系列（Anthropic Messages 协议）：claude-sonnet-5、claude-opus-5 等（Sonnet 5 于 2026-06-30 发布）
- **GPT** 系列（OpenAI 协议）：gpt-5.6 等（2026-07-09 发布）
- **Gemini** 系列（OpenAI 兼容）：gemini-3.1-pro 等
- **DeepSeek** 系列（OpenAI 兼容）：deepseek-v4 等（2026-07-20 发布）
- **GLM** 系列（OpenAI 兼容）：glm-5、glm-5.2 等（2026-02 发布）
- **Grok** 系列（OpenAI 兼容）：grok-4.5 等（2026-07-08 发布）
- **Kimi** 系列（OpenAI 兼容）：kimi-k3 等（2026-07-16 发布）
- **MiniMax** 系列（OpenAI 兼容）：MiniMax-M3 等
- **免费模型**（opencode Zen 网关，零成本）：deepseek-v4-flash 等

---

## 项目结构

```
ago/
├── cmd/ago/main.go        ← 程序入口
├── internal/              ← 私有应用代码
│   ├── base/              ← 日志 + 错误类型
│   ├── config/            ← opencode.json 解析
│   ├── provider/          ← LLM Provider（OpenAI 兼容 + Anthropic + opencode Zen）
│   ├── agent/             ← Agent 运行时 + 子代理管理
│   ├── tool/              ← read/write/edit/task/bash 工具
│   └── transport/         ← 本机 channel 通信
├── tests/                 ← 测试代码
├── docs/                  ← 文档
└── AGENTS.md              ← 项目目录铁律
```

---

## Acknowledgments

本项目站在巨人的肩膀上：

- **[opencode](https://github.com/sst/opencode)** (MIT License, © sst)
  — 本项目的配置文件格式（`opencode.json`）、Provider 抽象设计、核心工具接口
  （read/write/edit）、以及 opencode Zen 网关双模式鉴权逻辑均参考自 opencode 原版。
  原版为 TypeScript 实现，本项目用 Go 重新实现，保留配置兼容性，未直接复制源代码。
  感谢 sst 团队开源这一优秀项目。

- **[Pi / pi-mono](https://github.com/badlogic/pi-mono)** (MIT License, © Mario Zechner)
  — 极简工具集的设计思路受 Pi 启发。Pi 是一个「minimal terminal coding harness」，
  默认仅 4 个工具（read/write/edit/bash），文件搜索与代码搜索全部通过 bash 完成，
  理念为 "Adapt pi to your workflows, not the other way around"。
  ago 延续这一极简哲学，并在此基础上增加 `task` 工具实现子代理并发调用。
  感谢 Mario Zechner 开源这一优秀项目。

- **[TRAE IDE](https://www.trae.cn/)** + **GLM-5.2**
  — 本项目的初始代码由 TRAE IDE 中的 GLM-5.2 模型辅助编写，
  人工负责架构设计、代码审查、测试验证与调试。

---

## License

[MIT](LICENSE) © 2026 ago contributors

本项目基于 opencode 的设计思路用 Go 重新实现，未直接复制其源代码。
opencode 原项目版权归属 sst 团队，详见 https://github.com/sst/opencode

---

## Keywords

<!-- SEO 关键词段，便于 GitHub 搜索和搜索引擎索引 -->

`ai agent` `agent runtime` `agentic` `agentic workflow` `multi-agent` `subagent` `coding agent` `coding assistant` `cli coding assistant` `code agent` `tool use` `tool-use` `function calling` `llm agent` `llm runtime` `coding harness` `terminal coding`

`go` `golang` `gopher` `goroutine` `channel` `concurrent` `high-concurrency` `minimalist` `single binary` `zero dependency`

`llm` `large language model` `chatgpt` `openai` `anthropic` `claude` `deepseek` `glm` `grok` `kimi` `minimax` `openrouter` `zen` `free model` `free llm`

`streaming` `sse` `server-sent events` `stream complete` `opencode compatible` `opencode.json` `config compatible`

`pi` `pi-mono` `trae` `glm-5.2` `ai-assisted development` `clean room reimplementation` `hacktoberfest`
