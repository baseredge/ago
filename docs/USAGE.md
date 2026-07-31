# ago 使用教程

极简通用 Agent 底座 —— 单可执行文件，配置驱动，超高并发，复用 opencode.json 配置格式。

---

## 1. 编译

### 1.1 前置要求
- Go 1.21+（`go version` 验证）
- Windows / Linux / macOS 任一平台

### 1.2 编译命令

在项目根目录 `d:\aiDo\GO\ago` 执行：

```powershell
# Windows（生成 bin\ago.exe）
go build -o bin\ago.exe ./cmd/ago

# Linux / macOS（生成 bin/opencode）
go build -o bin/opencode ./cmd/ago
```

编译产物位于 `bin/` 目录，约 9.5 MB，零外部依赖（静态编译）。

### 1.3 交叉编译

```powershell
# Windows 上编译 Linux 版
$env:GOOS="linux"; $env:GOARCH="amd64"; go build -o bin/opencode-linux ./cmd/ago

# Windows 上编译 macOS 版
$env:GOOS="darwin"; $env:GOARCH="amd64"; go build -o bin/opencode-mac ./cmd/ago

# 编译完记得清空环境变量，避免影响后续编译
$env:GOOS=""; $env:GOARCH=""
```

---

## 2. 配置文件

### 2.1 查找顺序

启动时按以下顺序查找 `opencode.json`：

1. 命令行 `-config` 指定的路径
2. 当前工作目录的 `opencode.json`
3. 当前工作目录的 `.opencode/opencode.json`
4. 用户目录 `~/opencode.json`
5. 用户目录 `~/.opencode/opencode.json`
6. 用户目录 `~/.config/opencode/opencode.json`

找到即停，找不到报错退出。

### 2.2 最小配置：免费模式（无需登录）

复用 opencode 官方的 Zen 网关，未配置 apiKey 时硬编码 `apiKey=public`，调用免费模型零成本：

```json
{
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
  }
}
```

### 2.3 付费模式：使用自己的 API Key

配置 `apiKey` 或 `env`，Bearer 鉴权，所有模型可用：

```json
{
  "model": "opencode/grok-3",
  "provider": {
    "opencode": {
      "name": "OpenCode Zen",
      "options": {
        "baseURL": "https://opencode.ai/zen/v1",
        "apiKey": "你的-Zen-API-Key"
      },
      "models": {
        "grok-3": { "name": "Grok 3" }
      }
    }
  }
}
```

或用环境变量（推荐，避免硬编码）：

```json
{
  "model": "opencode/grok-3",
  "provider": {
    "opencode": {
      "env": ["OPCODE_API_KEY"],
      "options": { "baseURL": "https://opencode.ai/zen/v1" },
      "models": { "grok-3": { "name": "Grok 3" } }
    }
  }
}
```

```powershell
$env:OPCODE_API_KEY="你的-Zen-API-Key"
.\bin\ago.exe
```

### 2.4 使用 OpenAI 原生 API

```json
{
  "model": "openai/gpt-4o",
  "provider": {
    "openai": {
      "options": { "baseURL": "https://api.openai.com/v1" },
      "env": ["OPENAI_API_KEY"],
      "models": {
        "gpt-4o": { "name": "GPT-4o" }
      }
    }
  }
}
```

### 2.5 使用 Anthropic Claude

```json
{
  "model": "anthropic/claude-sonnet-5",
  "provider": {
    "anthropic": {
      "options": { "baseURL": "https://api.anthropic.com" },
      "env": ["ANTHROPIC_API_KEY"],
      "models": {
        "claude-sonnet-5": { "name": "Claude Sonnet 5" }
      }
    }
  }
}
```

### 2.6 配置子代理（多 Agent 并行）

通过 `agent`（原版标准字段名，也支持旧别名 `agents`）预定义子代理，主代理通过 `task` 工具调用：

```json
{
  "model": "opencode/deepseek-v4-flash-free",
  "provider": {
    "opencode": {
      "options": { "baseURL": "https://opencode.ai/zen/v1" },
      "models": {
        "deepseek-v4-flash-free": { "cost": { "input": 0, "output": 0 } },
        "grok-3": { "name": "Grok 3" }
      }
    }
  },
  "subagent_depth": 3,
  "agent": {
    "research": {
      "model": "opencode/deepseek-v4-flash",
      "system": "你是一个调研助手，负责收集和整理信息",
      "mode": "subagent",
      "steps": 8,
      "task_budget": 3
    },
    "writer": {
      "model": "opencode/grok-4.5",
      "system": "你是一个写作助手，擅长润色和组织文本",
      "mode": "subagent",
      "tools": { "read": true, "write": true, "edit": true }
    },
    "safe_runner": {
      "model": "opencode/glm-5.2",
      "system": "只读分析助手，不修改文件不执行命令",
      "mode": "subagent",
      "tools": { "bash": false, "task": false, "write": false, "edit": false }
    }
  }
}
```

字段说明：

**顶层字段：**
- `subagent_depth`：子代理最大嵌套深度（原版标准字段，默认 1，极简版兜底 3）。主代理 depth=0，每调一层子代理 depth+1，超过则拒绝。防止子 agent 递归失控。

**每个 agent 的字段：**
- `model`：子代理使用的模型（不填则继承主代理）
- `system`：系统提示词
- `mode`：`subagent`（仅可被调用）/ `primary`（仅主用）/ `all`（两者皆可）
- `tools`：工具启用/禁用映射（原版 schema 字段，map 风格）。`nil`/空=继承全部；非空时按 map 控制每个工具，未列出的工具默认禁用。例：`{"bash": false}` 禁用 bash 其余可用；`{"read": true}` 仅启用 read
- `steps`：该 agent 最大工具调用轮数（原版标准字段，默认 10）。达到上限后强制返回纯文本响应
- `maxSteps`：`steps` 的废弃别名（旧版配置兼容，优先级低于 `steps`）
- `task_budget`：该 agent 调用子代理的次数预算（原版标准字段，0=不限制）

### 2.7 配置文件兼容性

ago 复用 opencode 原版 `opencode.json` 格式，**仅解析核心字段**：

| 字段 | 是否解析 | 说明 |
|------|---------|------|
| `model` | ✓ | 默认模型 ID |
| `provider` | ✓ | 自定义 provider map |
| `agent` | ✓ | 子代理配置（原版标准字段名） |
| `agents` | ✓ | 子代理配置（旧别名，`agent` 优先） |
| `subagent_depth` | ✓ | 子代理最大嵌套深度（默认 3） |
| `agent[].steps` / `agent[].maxSteps` | ✓ | agent 最大工具调用轮数 |
| `agent[].task_budget` | ✓ | agent 调用子代理次数预算 |
| `agent[].tools` | ✓ | 工具启用/禁用映射（map 风格，原版 schema 字段） |
| `$schema` / `shell` / `mcp` / `lsp` / `formatter` / `permissions` / `watcher` / 其他 | ✗ | 解析后忽略，不报错，保持加载兼容 |

你的现有 opencode.json 可以**直接加载不崩溃**，但 mcp/lsp 等功能不实现。

---

## 3. 启动运行

### 3.1 基本启动

```powershell
# 在 opencode.json 所在目录启动
.\bin\ago.exe

# 指定配置文件
.\bin\ago.exe -config "D:\myproject\opencode.json"

# 启用调试日志（输出 agent 启停、provider 调用等）
.\bin\ago.exe -debug
```

### 3.2 启动后界面

```
================================
  ago 极简 Agent 底座
================================
可用子代理: [research writer]

>
```

### 3.3 交互命令

| 命令 | 作用 |
|------|------|
| `/help` | 显示帮助 |
| `/agents` | 列出已配置的子代理 |
| `/quit` 或 `/exit` | 退出程序 |
| `Ctrl+C` | 强制中断退出 |
| 其他文本 | 作为用户消息发给主代理 |

### 3.4 对话示例

```
> 帮我读一下当前目录的 README.md
（主代理调用 read 工具，返回文件内容）

> 用 research 子代理调研一下 Go 语言的并发模型，再用 writer 子代理写成一份总结
（主代理并行调用 task 工具触发两个子代理 goroutine，等待结果汇总返回）
```

---

## 4. 核心工具

主代理和子代理（未设白名单时）均可用以下五个工具，由 LLM 自主决定调用：

| 工具 | 入参 | 作用 |
|------|------|------|
| `read` | `path` | 读文件内容或列目录 |
| `write` | `path`, `content` | 写文件（自动创建父目录） |
| `edit` | `path`, `old_string`, `new_string`, `replace_all?` | 字符串精确替换 |
| `task` | `subagent_name`, `prompt` | 调用子代理执行任务（跟随父 agent context 生命周期，无固定超时） |
| `bash` | `command`, `workdir?`, `timeout_sec?` | 执行 shell 命令（Unix 用 sh，Windows 用 cmd，超时由 LLM 传 timeout_sec 控制） |

工具调用轮数上限由 agent 的 `steps` 配置决定（默认 10 轮），防止无限调用。每个工具可在 agent 配置中通过 `tools` 字段禁用。

---

## 5. 并发模型

- **每个 agent 一个 goroutine + 独立消息 channel**，由 Go GMP 调度
- 主代理可通过 `task` 工具**并行**触发多个子代理（每个子代理独立 goroutine）
- 子代理调用同步等待结果（默认 5 分钟超时，防止 goroutine 泄漏）
- 主代理退出时级联停止所有活跃子代理
- 本进程内 channel 通信，**零序列化开销**，无网络 IO

---

## 6. 支持的 Provider 协议

| 协议 | 覆盖 Provider | 端点 |
|------|--------------|------|
| OpenAI 兼容 | OpenAI / Zen(chat 端点) / OpenRouter / DeepSeek / GLM / Kimi / MiniMax / Grok | `POST /chat/completions` |
| Anthropic Messages | Anthropic Claude / Zen(messages 端点) | `POST /v1/messages` |

**opencode provider（Zen 网关）仅覆盖走 `/chat/completions` 端点的模型**（Grok/DeepSeek/MiniMax/GLM/Kimi/免费模型等）。
若要通过 Zen 调用 GPT（走 `/responses`）/Claude（走 `/messages`）/Gemini（走 `/models`），请配置对应原生 provider 走原生端点。

---

## 7. 常见问题

### Q1：启动报错 "加载配置失败"
A：当前目录及用户目录下没有 `opencode.json`。用 `-config` 指定路径，或在工作目录创建配置文件（参考第 2.2 节最小配置）。

### Q2：启动报错 "opencode.json 中未配置 model 字段"
A：配置文件顶层必须有 `model` 字段，格式为 `<provider_id>/<model_id>`，如 `opencode/deepseek-v4-flash-free`。

### Q3：免费模式调用报 401/403
A：Zen 网关的免费模型可能有额度限制或地区限制。换个免费模型试试，或切换到付费模式（配置自己的 apiKey）。

### Q4：调用报 429
A：触发限流。免费模式限流较严，建议降低并发或切换付费模式。程序已捕获 429 并转为 `ErrRateLimited`，不会崩溃。

### Q5：子代理调用超时
A：默认 5 分钟超时。若任务确实耗时更长，修改 `internal/agent/subagent.go` 的 `subagentTimeout` 常量后重新编译。

### Q6：工具调用死循环
A：每个 agent 的工具调用轮数由 `steps` 配置控制（默认 10 轮）。子代理调用次数由 `task_budget` 控制（0=不限制）。子代理嵌套深度由 `subagent_depth` 控制（默认 3 层）。三层防护避免递归失控。若 LLM 反复调用同一工具，检查 system prompt 是否清晰引导任务收敛。

### Q7：如何确认 LLM 真的返回了
A：加 `-debug` 启动，会输出 agent 启停、子代理完成等日志到 stderr。

---

## 8. 项目结构

```
ago/
├── bin/ago.exe        ← 编译产物（本教程的目标）
├── cmd/ago/main.go    ← 程序入口
├── internal/               ← 私有应用代码
│   ├── base/               ← 日志 + 错误类型
│   ├── config/             ← opencode.json 解析
│   ├── provider/           ← LLM Provider（OpenAI 兼容 + Anthropic + opencode）
│   ├── agent/              ← Agent 运行时 + 子代理管理
│   ├── tool/               ← read/write/edit/task/bash 工具
│   └── transport/          ← 本机 channel 通信
├── tests/                  ← 测试代码
├── docs/                   ← 文档（本文件 + 计划文件）
├── AGENTS.md               ← 项目目录铁律
└── go.mod
```

详细设计取舍见 [docs/plans/ago-port-2026-07-31.md](plans/ago-port-2026-07-31.md)。
