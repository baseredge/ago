# ago 移植计划：极简通用 Agent 底座（基于原版源码核对）

> **更名说明**：本项目原名 `opencode-go`，已更名为 `ago`。Go module 名、二进制名、banner 均已更新；配置文件名 `opencode.json` 和 provider id `opencode` 为保持与原版兼容未改。本计划文件为历史文档，正文中的 `opencode-go` 字样保留作为历史记录。

- **计划日期**: 2026-07-31
- **计划性质**: 全新项目，移植 opencode（TypeScript 版）核心逻辑到 Go
- **主线**: 极度精简的通用 Agent 底座 + 超高并发 + 最快落地
- **目录标准**: 标准 Go 布局
- **核心原则**: 极度精简、核心功能、超高并发、最快落地
- **移植依据**: opencode 原版源码（已 clone 到 `D:\aiDo\GO\opencode-reference`，dev 分支，commit 见 git log）

---

## 0. 移植依据源码位置

- **源码路径**: `D:\aiDo\GO\opencode-reference`（项目外参考目录，不侵入 opencode-go）
- **版本**: dev 分支最新（2026-07-31 clone）
- **仓库**: github.com/sst/opencode（MIT 协议）
- **关键代码位置**:
  - `packages/core/src/config.ts` + `packages/core/src/config/*.ts`：配置 schema
  - `packages/core/src/provider.ts` + `packages/core/src/catalog.ts`：Provider 抽象
  - `packages/core/src/agent.ts`：Agent 抽象
  - `packages/core/src/tool/{read,write,edit}.ts`：核心工具实现
  - `packages/llm/src/protocols/*.ts`：5 套 LLM 协议实现
  - `packages/llm/src/providers/*.ts`：11 个 provider 适配
  - `packages/schema/src/{provider,agent,model}.ts`：Schema 定义
  - `packages/web/src/content/docs/zen.mdx`：Zen 官方文档（事实依据）

---

## 1. 需求理解

### 一句话
移植 opencode TS 版核心逻辑到 Go，构建**极简通用 Agent 底座**：保留 opencode 的 provider 层（配置兼容 `opencode.json`）+ 主代理/子代理模式（每 agent 一个 goroutine 实现超高并发）+ 仅保留 read/write/edit 三个核心工具，不做 MCP、不做跨机通信、不做自研重试/过载框架。

### 核心取舍（极简原则）
- **保留**：base（logger + errors）、provider 层（opencode.json 配置兼容 + 极简协议实现）、agent 层（主代理 + 子代理配置式）、read/write/edit + task 工具、本进程内 channel 通信。
- **删除**：MCP、bash/glob/grep/list/webfetch 等非核心工具、跨机 TCP 通信、自研 retry/overload 框架（依赖 Go channel 原生 + context）、递归式无限派生（改为 opencode 配置式子代理）、LSP/formatter/watcher/snapshots/permissions 等非核心配置字段、catalog/models-dev 动态拉取（改为静态配置）、Effect 函数式框架（用 Go 原生重写逻辑）。

### 验收标准
1. **配置兼容**：能读取标准 `opencode.json`（极简解析：识别 `model`、`provider`、`agent`（原版标准字段名，也支持旧别名 `agents`）、`subagent_depth`、agent 级 `steps`/`maxSteps`/`task_budget` 等核心字段；其他字段如 `mcp`/`lsp`/`formatter`/`permissions`/`watcher` 等解析后忽略不报错，保持兼容），用户现有 opencode 配置可加载不崩溃。
2. **Provider 可用**：实现 OpenAI 兼容协议 + Anthropic Messages 协议两套，覆盖大部分主流模型。
3. **opencode provider 可用（双模式）**：
   - **免费模式**：未配置 apiKey 时，硬编码 `apiKey=public`，仅暴露 cost.input=0 的免费模型，通过 OpenAI 兼容协议调用 `https://opencode.ai/zen/v1/chat/completions`，用户无需登录即可使用免费模型（对齐原版 `packages/opencode/src/provider/provider.ts` 第 179-201 行逻辑）。
   - **付费模式**：用户配置 apiKey 时，Bearer 鉴权，所有模型可用。
   - 仅覆盖走 chat/completions 端点的 Zen 模型（Grok/DeepSeek/MiniMax/GLM/Kimi/免费模型等）。
4. **主代理 + 子代理**：通过 `opencode.json` 的 `agent` 字段定义子代理（含 model + system prompt + mode + steps + task_budget），主代理通过 `task` 工具调用子代理。`subagent_depth` 控制最大嵌套深度（默认 3）。
5. **核心工具**：read（读文件）、write（写文件）、edit（编辑文件）、task（调用子代理）、bash（执行 shell 命令）五个工具可用。
6. **超高并发**：每个 agent 独立 goroutine + 独立 MsgChan，主代理可并行调用多个子代理，利用 Go GMP 实现高并发。
7. **本进程通信**：主子代理通过 Go channel 通信，无网络开销。
8. **递归防护**：三层防护避免 agent 递归失控 —— `steps`（单 agent 工具调用轮数）、`task_budget`（单 agent 调用子代理次数）、`subagent_depth`（最大嵌套深度）。

> 明确不做：MCP、跨机通信、自研二进制协议、TCP 连接池、自研 retry/overload 框架、glob/grep/list/webfetch 工具、catalog/models-dev 动态拉取、Effect 框架、LSP/formatter/watcher 等非核心配置。

---

## 2. 与原版的关键事实修正（重要）

> 阶段 2 查复用时核对了原版源码 `packages/opencode/src/provider/provider.ts`，确认豆包 thread `xmqvghFn7GwMnGD96` 关于 Zen 的描述基本正确，但需要补充源码级事实细节。

### 修正 1：opencode provider（Zen）真实鉴权逻辑（双模式）
- **thread 描述（正确）**：内置 `apiKey: "public"` 免 Key 调用免费额度。
- **原版事实**（见 `packages/opencode/src/provider/provider.ts` 第 179-201 行 `opencode` provider loader）：
  - provider id 是 `opencode`（catalog 中对应 Zen 网关），不是 `zen`
  - **免费模式**：检测到没有付费 key 时，**硬编码 `options: { apiKey: "public" }`**，并按 `cost.input === 0` 过滤，只保留免费模型
  - **付费模式**：检测到 env var / auth / `opencode.json` 中配置的 apiKey 时，所有模型可用
  - 模型列表由 `models.dev` catalog 动态加载，按 cost 过滤
- **Go 端实现**：opencode provider 双模式鉴权：
  - 免费模式：硬编码 `apiKey=public`（写入请求头 `Authorization: Bearer public` 或 `x-api-key: public`），仅暴露免费模型
  - 付费模式：用户配置 `apiKey` 时用 Bearer 鉴权，所有模型可用
  - 模型列表：极简版不移植 models.dev 动态拉取，改为静态配置（用户在 `opencode.json` 显式声明 models，或内置一份免费模型清单）

### 修正 2：Zen 不同模型走不同协议端点
- **原版事实**（见 `packages/web/src/content/docs/zen.mdx`）：不同模型走不同端点：
  - GPT 系列 → `/zen/v1/responses`（OpenAI 原生 Responses 协议）
  - Claude 系列 → `/zen/v1/messages`（Anthropic Messages 协议）
  - Gemini 系列 → `/zen/v1/models/<model>`（Google 协议）
  - 其他（Grok/DeepSeek/MiniMax/GLM/Kimi/免费模型等）→ `/zen/v1/chat/completions`（OpenAI 兼容协议）
- **Go 端取舍**：极简版只实现 OpenAI 兼容协议调用 opencode provider，仅覆盖走 chat/completions 端点的 Zen 模型（Grok/DeepSeek/MiniMax/GLM/Kimi/免费模型等）。GPT/Claude/Gemini 通过用户自配的对应原生 provider 调用（不走 opencode provider）。

### 修正 3：客户端代码不硬编码 opencode.ai/zen
- **原版事实**：`packages/core/src` 里没有 `opencode.ai/zen` 字符串硬编码，opencode provider 的 baseURL 由 `models.dev` catalog 动态提供。
- **Go 端取舍**：极简版在 `opencode.json` 中显式声明 `provider.opencode.options.baseURL=https://opencode.ai/zen/v1`，不移植 catalog 动态拉取。

### 修正 4：原版用 Effect 函数式框架，Go 无法对应
- **原版事实**：`packages/core/src` 大量使用 `Effect.gen`、`Layer.effect`、`Schema.Class`、`Context.Service` 等 Effect 框架抽象。
- **Go 端取舍**：Effect 框架无法移植，所有逻辑用 Go 原生重写（接口 + struct + goroutine + channel），只移植**业务逻辑**不移植**抽象方式**。

### 修正 5：原版 provider 层是多协议多 provider 适配
- **原版事实**：`packages/llm/src/protocols/` 有 5 套协议实现（OpenAI Chat / OpenAI Responses / OpenAI Compatible / Anthropic Messages / Gemini / Bedrock），`packages/llm/src/providers/` 有 11 个 provider 适配。
- **Go 端取舍**：极简版只实现 OpenAI 兼容协议 + Anthropic Messages 协议两套，覆盖大部分主流模型（OpenAI/Claude/Zen 大部分/OpenRouter/DeepSeek/GLM/Kimi 等都走这两套协议）。

### 修正 6：配置 schema 字段非常多
- **原版事实**（见 `packages/core/src/config.ts`）：`opencode.json` 顶层字段有 `$schema`/`shell`/`model`/`default_agent`/`autoupdate`/`share`/`enterprise`/`username`/`permissions`/`agents`/`snapshots`/`watcher`/`formatter`/`lsp`/`attachments`/`tool_output`/`mcp`/`compaction`/`skills`/`commands`/`instructions`/`references`/... 等二十多个。
- **Go 端取舍**：极简版只解析 `model`/`provider`/`agents` 三个核心字段，其他字段用 `json.RawMessage` 接收后忽略，保持配置加载兼容（用户现有 opencode.json 能加载不报错），但不实现对应功能。

---

## 3. 影响范围

全新项目，无存量代码冲突。涉及以下子系统（均为新建）：

| 子系统 | 职责 | 极简说明 | 原版参考 |
|--------|------|---------|---------|
| base | logger + errors | 不要 retry/overload 自研层 | `packages/core/src/util/error.ts` |
| config | opencode.json 解析 | 仅解析 model/provider/agents 核心字段，其他忽略 | `packages/core/src/config.ts` |
| provider | OpenAI 兼容 + Anthropic Messages 协议 + factory | 不做 catalog 动态拉取 | `packages/llm/src/protocols/*.ts` |
| agent | 主代理 + 子代理 | opencode 配置式，非递归派生 | `packages/core/src/agent.ts` |
| tool | read/write/edit/task/bash | 仅核心工具 + task 调用子代理 + bash 执行命令 | `packages/core/src/tool/{read,write,edit}.ts` |
| transport | 本机 channel | 无网络协议 | 自研 |

---

## 4. 文件目录映射（标准 Go 布局）

> 注：本次只落计划文件，不建项目骨架。下面是阶段 3 执行时的目标目录映射。

```
opencode-go/
├── cmd/
│   └── opencode/
│       └── main.go              # 主入口
├── internal/
│   ├── base/                    # 基础层（极简）
│   │   ├── logger.go            # 全局日志（标准 log 包封装）
│   │   └── errors.go            # 统一错误类型（ErrRateLimited/ErrGateway）
│   ├── config/                  # 配置层
│   │   └── config.go            # opencode.json 解析（兼容原版格式，仅核心字段）
│   ├── provider/                # LLM Provider 子系统
│   │   ├── provider.go          # Provider 接口（对齐原版抽象）
│   │   ├── openai_compatible.go # OpenAI 兼容协议（含 SSE 流式解析）
│   │   ├── anthropic.go         # Anthropic Messages 协议
│   │   └── factory.go           # 从 opencode.json 加载并实例化 provider
│   ├── agent/                   # Agent 运行时（主代理 + 子代理）
│   │   ├── agent.go             # AgentRuntime 结构体、消息循环、goroutine 启动
│   │   └── subagent.go          # 子代理调用逻辑（task 工具入口）
│   ├── tool/                    # 核心工具（仅 read/write/edit/task）
│   │   ├── read.go              # 读文件
│   │   ├── write.go             # 写文件
│   │   ├── edit.go              # 编辑文件（字符串替换）
│   │   └── task.go              # 调用子代理
│   └── transport/               # 本进程内通信层（无网络）
│       └── local.go             # channel 通信（Message 结构体内联）
├── pkg/                         # 对外可复用库（暂留空）
├── docs/
│   └── plans/
│       └── opencode-go-port-2026-07-31.md  # 本计划文件
├── tests/
│   ├── provider/
│   │   └── provider_test.go
│   ├── agent/
│   │   └── agent_test.go
│   ├── tool/
│   │   └── tool_test.go
│   └── config/
│       └── config_test.go
├── tools/                       # 开发期工具（暂留空）
├── go.mod
├── go.sum
└── AGENTS.md                    # 项目目录铁律（阶段 3 建立）
```

### 文件目录映射表

| 新增文件 | 所属目录 | 职责 | 原版参考 |
|---------|---------|------|---------|
| `cmd/opencode/main.go` | cmd/opencode/ | 程序入口 | `packages/opencode/src/index.ts` |
| `internal/base/logger.go` | internal/base/ | 日志 | - |
| `internal/base/errors.go` | internal/base/ | 错误类型 | `packages/core/src/util/error.ts` |
| `internal/config/config.go` | internal/config/ | opencode.json 解析 | `packages/core/src/config.ts` |
| `internal/provider/provider.go` | internal/provider/ | Provider 接口 | `packages/schema/src/provider.ts` |
| `internal/provider/openai_compatible.go` | internal/provider/ | OpenAI 兼容协议 | `packages/llm/src/protocols/openai-compatible-chat.ts` |
| `internal/provider/anthropic.go` | internal/provider/ | Anthropic Messages 协议 | `packages/llm/src/protocols/anthropic-messages.ts` |
| `internal/provider/factory.go` | internal/provider/ | 从 opencode.json 加载 | `packages/core/src/provider.ts` |
| `internal/agent/agent.go` | internal/agent/ | AgentRuntime + goroutine | `packages/core/src/agent.ts` |
| `internal/agent/subagent.go` | internal/agent/ | 子代理调用 | `packages/core/src/agent.ts` |
| `internal/tool/read.go` | internal/tool/ | 读文件工具 | `packages/core/src/tool/read.ts` |
| `internal/tool/write.go` | internal/tool/ | 写文件工具 | `packages/core/src/tool/write.ts` |
| `internal/tool/edit.go` | internal/tool/ | 编辑文件工具 | `packages/core/src/tool/edit.ts` |
| `internal/tool/task.go` | internal/tool/ | 调用子代理工具 | - |
| `internal/transport/local.go` | internal/transport/ | 本机 channel 通信 | - |
| `tests/**/*.go` | tests/ | 测试代码 | - |
| `docs/plans/*.md` | docs/plans/ | 计划文件 | - |

---

## 5. 实施步骤

### 阶段 A：base 基础层（极简）
1. 新建 `internal/base/logger.go`：封装标准 `log` 包，提供全局 logger。
2. 新建 `internal/base/errors.go`：定义 `ErrRateLimited`（429）、`ErrGatewayUnavailable`、`ErrNetworkTimeout`，用于 provider 错误统一识别。

### 阶段 B：config 配置层（opencode.json 兼容）
3. 新建 `internal/config/config.go`：解析 `opencode.json`，对齐原版字段（参考 `packages/core/src/config.ts`）：
   - **核心字段（解析使用）**：
     - `model`：默认模型 ID（如 `opencode/gpt-5.5` 或 `openai/gpt-4o`）
     - `provider`：自定义 provider map（key=provider id，value 含 name/api/options/models）
     - `agents`：子代理 map（key=agent id，value 含 model/system prompt/mode）
   - **兼容字段（用 json.RawMessage 接收后忽略）**：`$schema`/`shell`/`default_agent`/`autoupdate`/`share`/`enterprise`/`username`/`permissions`/`snapshots`/`watcher`/`formatter`/`lsp`/`attachments`/`tool_output`/`mcp`/`compaction`/`skills`/`commands`/`instructions`/`references` 等全部其他字段。
   - provider 配置结构对齐 `packages/core/src/config/provider.ts`：`name`/`env`/`api`(type/package/url/settings)/`request`(headers/body)/`models`。
   - agent 配置结构对齐 `packages/core/src/config/agent.ts`：`model`/`system`/`description`/`mode`(subagent/primary/all)/`hidden`/`steps`。
   - 支持 `{env:VAR}` 占位符解析环境变量。

### 阶段 C：Provider 层（OpenAI 兼容 + Anthropic Messages）
4. 新建 `internal/provider/provider.go`：定义 `Provider` 接口，方法 `Init()`、`StreamComplete(ctx, messages, model)`、`ListModels()`，对齐原版抽象（参考 `packages/schema/src/provider.ts`）。
5. 新建 `internal/provider/openai_compatible.go`：OpenAI 兼容协议实现（参考 `packages/llm/src/protocols/openai-compatible-chat.ts`），重点：
   - POST `/chat/completions` 流式请求。
   - SSE 流式 chunk 解析（`data: ` 前缀、`[DONE]` 结束、chunk JSON 解析）。
   - Bearer 鉴权。
   - 429 限流捕获 → `base.ErrRateLimited`。
   - 覆盖：OpenAI、Zen（chat/completions 端点）、OpenRouter、DeepSeek、GLM、Kimi、MiniMax、Grok 等兼容协议。
6. 新建 `internal/provider/anthropic.go`：Anthropic Messages 协议实现（参考 `packages/llm/src/protocols/anthropic-messages.ts`），重点：
   - POST `/v1/messages` 流式请求。
   - Anthropic SSE 事件解析（`message_start`/`content_block_delta`/`message_stop` 等）。
   - `x-api-key` + `anthropic-version` 鉴权头。
   - 覆盖：Anthropic Claude 系列、Zen 的 Claude 模型（如配 `opencode/claude-sonnet-5` 走 `/zen/v1/messages`）。
7. 新建 `internal/provider/opencode.go`：opencode provider（对应 Zen 网关），基于 openai_compatible.go 复用 SSE 解析，定制（参考 `packages/opencode/src/provider/provider.ts` 第 179-201 行 `opencode` provider loader）：
   - **双模式鉴权**：检测配置中是否有 apiKey/env var，有则付费模式（Bearer 鉴权，所有模型可用），无则免费模式（硬编码 `apiKey=public`，仅暴露 cost.input=0 的免费模型）。
   - 请求头：`Authorization: Bearer <apiKey>` 或 `x-api-key: <apiKey>`（按 OpenAI 兼容协议标准）。
   - 429 限流捕获 → 转换为 `base.ErrRateLimited`。
   - 模型列表：极简版不移植 models.dev 动态拉取，改为静态配置（用户在 `opencode.json` 显式声明 models，或内置一份免费模型清单作为默认值）。
   - baseURL：`https://opencode.ai/zen/v1`（在 opencode.json 中显式声明或作为默认值）。
8. 新建 `internal/provider/factory.go`：根据 `opencode.json` 的 `provider` 配置实例化 provider，识别 provider 类型：
   - `provider=opencode` → opencode provider（Zen 网关，双模式鉴权）
   - `api.type=aisdk` + `api.package=@ai-sdk/openai-compatible` → OpenAI 兼容
   - `api.type=aisdk` + `api.package=@ai-sdk/anthropic` → Anthropic Messages
   - `api.type=aisdk` + `api.package=@ai-sdk/openai` → OpenAI 兼容（极简版归一处理）
   - 其他 → 默认走 OpenAI 兼容（极简降级）

### 阶段 D：tool 工具层（核心工具 + task + bash）
9. 新建 `internal/tool/read.go`：`read(path)` 工具（参考 `packages/core/src/tool/read.ts`），读文件内容返回，支持文本和图片 base64。
10. 新建 `internal/tool/write.go`：`write(path, content)` 工具（参考 `packages/core/src/tool/write.ts`），写文件。
11. 新建 `internal/tool/edit.go`：`edit(path, old_string, new_string)` 工具（参考 `packages/core/src/tool/edit.ts`），字符串替换编辑。
12. 新建 `internal/tool/task.go`：`task(subagent_name, prompt)` 工具，调用配置中定义的子代理，返回子代理执行结果。Depth 字段由 agent 层注入（json:"-"），控制递归深度。
13. 新建 `internal/tool/bash.go`：`bash(command, workdir?, timeout_sec?)` 工具，跨平台执行 shell 命令（Unix 用 sh，Windows 用 cmd），超时保护（Windows 用 taskkill /F /T kill 进程树）。

### 阶段 E：transport 本机通信层
14. 新建 `internal/transport/local.go`：定义 `Message` 结构体（SrcAgentID、DstAgentID、Type、Payload）+ channel 收发接口。本机直接传指针，不序列化。

### 阶段 F：agent 运行时（主代理 + 子代理 + 高并发）
15. 新建 `internal/agent/agent.go`（参考 `packages/core/src/agent.ts` + `packages/schema/src/agent.ts`）：
    - `AgentRuntime` 结构体：AgentID、MsgChan（chan *Message）、Model、SystemPrompt、Mode（primary/subagent/all）、Tools 白名单、Provider 引用、steps（工具调用轮数上限）、depth（嵌套深度）、taskBudget/taskUsed（子代理调用预算）。
    - `Start()` 方法：启动独立 goroutine 跑消息循环（接收消息 → 调用 provider 流式推理 → 调用工具 → 返回结果）。
    - `Stop()` 方法：关闭 MsgChan，退出 goroutine。
16. 新建 `internal/agent/subagent.go`：子代理调用逻辑，主代理通过 `task` 工具触发：
    - 根据子代理名从配置查找定义。
    - 实例化子 AgentRuntime，启动独立 goroutine。
    - 主代理同步等待子代理结果（通过 channel）。
    - 子代理执行完自动 Stop，释放 goroutine。
    - 多个 task 调用可并行（主代理发起多个子代理 goroutine，Go GMP 调度）。

### 阶段 G：入口整合
17. 新建 `cmd/opencode/main.go`：加载 opencode.json → 初始化 provider → 初始化主 agent → 启动主循环（读取用户输入 → 主代理处理 → 输出结果）。
18. 初始化 `go.mod`（module 名 `opencode-go`）。

---

## 6. 测试策略

### 测试代码由业务 agent 自己写（Boss 工作流阶段 4 要求）
- 每个 `internal/<模块>/` 下的实现文件，配套在 `tests/<模块>/` 下写测试。

### 关键测试点
| 模块 | 测试文件 | 测试内容 |
|------|---------|---------|
| config | `tests/config/config_test.go` | opencode.json 核心字段解析、兼容字段忽略不报错、env 占位符、agent 配置 |
| provider | `tests/provider/provider_test.go` | OpenAI 兼容 SSE 解析、Anthropic Messages SSE 解析、429 限流捕获、Zen 通过 OpenAI 兼容调用 |
| tool | `tests/tool/tool_test.go` | read/write/edit 文件操作、task 子代理调用 |
| agent | `tests/agent/agent_test.go` | 主代理启动/停止、子代理并行调用、goroutine 生命周期 |

### 外部资源依赖（阶段 4 链查找优先级）
1. **Provider 调用测试**需要真实 API Key（OpenAI / Anthropic / Zen 任意一个）。
2. 按优先级链查找：
   - 同项目其他测试用过且仍可用的 fixtures（搜索 cookie/session/token/apikey）。
   - 主程序运行时已存的资源（cache/、.session、.port）。
   - 配置文件常驻资源（.env、opencode.json 中的 apiKey 字段）。
   - 都没有 → STOP，列出清单等用户。
3. **不准 mock 假装通过，不准跳过**：限流（429）测试必须真实触发或明确标记为集成测试。

---

## 7. 风险与取舍

### 风险 1：Zen 协议覆盖不完整（中风险）
- **问题**：Zen 的 GPT 系列走 `/zen/v1/responses`（OpenAI Responses 协议）、Claude 走 `/zen/v1/messages`、Gemini 走 `/zen/v1/models/<model>`，极简版只实现 OpenAI 兼容协议，覆盖不到 GPT/Claude/Gemini 通过 Zen 调用。
- **对策**：用户要调 GPT/Claude/Gemini 时，在 `opencode.json` 配置对应原生 provider（openai/anthropic/google）走原生端点，不走 Zen。Zen 只用于调兼容端点的模型（Grok/DeepSeek/MiniMax/GLM/Kimi/免费模型）。
- **取舍**：避免实现 5 套协议的复杂度，牺牲 Zen 全覆盖。

### 风险 2：配置兼容但不功能对齐（低风险）
- **问题**：极简版只解析 `model`/`provider`/`agents` 三个核心字段，其他字段（mcp/lsp/formatter/permissions/watcher 等）解析后忽略，用户期望这些功能时会失效。
- **对策**：明确告知用户极简版不支持的功能；后续按需扩展。

### 风险 3：协议合规（低风险）
- **问题**：opencode 开源为 MIT 协议，直接复制 TS 源码文本有版权问题。
- **对策**：**禁止直接复制 TS 源码文本**，只参考逻辑用 Go 重写。

### 风险 4：子代理 goroutine 泄漏（中风险）
- **问题**：子代理执行异常未正确退出会导致 goroutine 泄漏。
- **对策**：子代理 goroutine 使用 context 超时控制；task 工具调用无论成功失败都触发子代理 Stop；主代理退出时级联停止所有活跃子代理。

### 取舍说明
- **不做 MCP**：极简底座，工具固定为 read/write/edit/task。
- **不做跨机通信**：仅本进程内 channel。
- **不做自研 retry/overload**：依赖 Go channel + context。
- **不做递归式无限派生**：用 opencode 配置式子代理（在 opencode.json 的 `agents` 字段预定义）。
- **不做 catalog/models-dev 动态拉取**：改为静态配置。
- **不移植 Effect 框架**：用 Go 原生重写逻辑。
- **只实现 2 套协议**：OpenAI 兼容 + Anthropic Messages，覆盖大部分主流模型。
- **并发模型**：每 agent 一个 goroutine + 独立 MsgChan。

---

## 8. 工作量预估

| 模块 | 预估 | 说明 |
|------|------|------|
| base（极简版） | 半天 | logger + errors |
| config（opencode.json 兼容） | 半天 | 核心字段解析 + 兼容字段忽略 |
| provider（OpenAI 兼容 + Anthropic） | 1.5 天 | 两套协议 + SSE + factory |
| tool（read/write/edit/task） | 半天 | 三件套 + task |
| transport（本机 channel） | 2 小时 | Message + channel |
| agent（主代理 + 子代理 + goroutine） | 1 天 | 运行时 + 子代理调用 |
| 入口整合 | 半天 | main + go.mod |

---

## 9. 交付检查清单（阶段 7）

- [x] 根目录无散落 .go/.js 文件（根目录仅 AGENTS.md + go.mod）
- [x] 新增文件均在标准目录下（对照 AGENTS.md 目录铁律）
- [x] `tests/fixtures/` 测试数据与测试代码分离（test-config.json 在 fixtures/）
- [x] `tools/` 开发期工具未误放根目录（暂留空，未创建）
- [x] 计划文件路径：`docs/plans/opencode-go-port-2026-07-31.md`
- [x] 移植依据源码位置：`D:\aiDo\GO\opencode-reference`（dev 分支）
- [x] review 通过（修复 shared.go IdleConnTimeout=90ns bug → 90s；build/vet/gofmt/test 全绿）
- [x] 文档检查：AGENTS.md 已完整覆盖目录铁律与模块职责，无需更新

---

## 10. 待用户确认事项

1. **移植依据**：以 `D:\aiDo\GO\opencode-reference`（dev 分支）为只读参考，不复制源码文本只参考逻辑，是否 OK？
2. **opencode provider 双模式**：免费模式硬编码 `apiKey=public` + 按 cost 过滤免费模型（对齐原版源码逻辑）；付费模式 Bearer 鉴权 + 全模型可用；仅覆盖走 chat/completions 端点的 Zen 模型，是否 OK？
3. **协议范围**：极简版只实现 OpenAI 兼容 + Anthropic Messages 两套协议，不实现 OpenAI Responses / Gemini / Bedrock，是否 OK？
4. **配置兼容范围**：只解析 `model`/`provider`/`agents` 三个核心字段，其他字段解析后忽略（保持加载兼容但不实现功能），是否 OK？
5. **极简范围**：base + config + provider + agent + tool(read/write/edit/task) + transport(本机 channel)，删除 MCP/retry/overload/跨机/递归派生/catalog 动态拉取/Effect 框架/LSP/formatter 等，是否 OK？
6. **并发模型**：每 agent 一个 goroutine + 独立 MsgChan，主代理可并行调用多个子代理，是否 OK？
7. **执行顺序**：base → config → provider → tool → transport → agent → 入口，是否 OK？
