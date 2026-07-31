# ago

> A minimalist, high-concurrency AI agent runtime in Go.
> 极简、高并发、配置兼容 opencode.json 的 Agent 底座。

[![Go](https://img.shields.io/badge/Go-1.21+-00ADD8?logo=go)]()
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![Built with TRAE](https://img.shields.io/badge/Built%20with-TRAE%20%2B%20GLM--5.2-blueviolet)]()
[![Inspired by opencode](https://img.shields.io/badge/Inspired%20by-opencode-orange)](https://github.com/sst/opencode)

---

## 这是什么

`ago` 是一个极简的通用 AI Agent 底座，用 Go 编写。每个 agent 独立 goroutine + 独立消息 channel，实现超高并发。复用 [opencode](https://github.com/sst/opencode) 的 `opencode.json` 配置格式，开箱即用。

**特点：**

- **极简**：单二进制文件，零外部依赖，核心代码精简
- **高并发**：每 agent 一个 goroutine，主代理可并行调用多个子代理
- **配置兼容**：直接复用你现有的 `opencode.json`，无需修改
- **免费可用**：内置 opencode Zen 网关双模式鉴权，未配置 apiKey 时硬编码 `apiKey=public`，零成本调用免费模型
- **本机通信**：agent 间通过 Go channel 通信，零序列化开销

**明确不做**（极简取舍）：MCP、跨机通信、自研 retry/overload 框架、bash/glob/grep 等非核心工具、递归式无限派生、catalog 动态拉取、LSP/formatter/watcher。

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
| `task` | 调用子代理执行任务（独立 goroutine，5 分钟超时） |

---

## 支持的 Provider 协议

| 协议 | 覆盖 |
|------|------|
| OpenAI 兼容 | OpenAI / Zen(chat 端点) / OpenRouter / DeepSeek / GLM / Kimi / MiniMax / Grok |
| Anthropic Messages | Anthropic Claude / Zen(messages 端点) |

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
│   ├── tool/              ← read/write/edit/task 工具
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

- **[TRAE IDE](https://www.trae.cn/)** + **GLM-5.2**
  — 本项目的初始代码由 TRAE IDE 中的 GLM-5.2 模型辅助编写，
  人工负责架构设计、代码审查、测试验证与调试。

---

## License

[MIT](LICENSE) © 2026 ago contributors

本项目基于 opencode 的设计思路用 Go 重新实现，未直接复制其源代码。
opencode 原项目版权归属 sst 团队，详见 https://github.com/sst/opencode
