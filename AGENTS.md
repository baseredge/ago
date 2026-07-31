# ago 项目目录铁律

## 根目录原则
**根目录只留入口和配置**，代码/测试/文档/工具各归其位。

## 标准目录结构

```
ago/
├── cmd/                # 程序入口（仅放 main.go）
│   └── ago/
├── internal/           # 私有应用代码（不对外暴露）
│   ├── base/           # 基础层（logger + errors）
│   ├── config/         # 配置层（opencode.json 解析）
│   ├── provider/       # LLM Provider 子系统
│   ├── agent/          # Agent 运行时
│   ├── tool/           # 核心工具（read/write/edit/task/bash）
│   └── transport/      # 本进程内通信层
├── pkg/                # 对外可复用库（暂留空，按需扩展）
├── docs/
│   └── plans/          # 计划文件
├── tests/              # 测试代码
│   ├── provider/
│   ├── agent/
│   ├── tool/
│   ├── config/
│   └── fixtures/       # 测试数据文件（与测试代码分离）
├── tools/              # 开发期工具（暂留空）
├── go.mod
├── go.sum
└── AGENTS.md           # 本文件
```

## 铁律

1. **根目录禁止散落 .go/.js/.ts 文件**，所有代码必须在 `cmd/`、`internal/`、`pkg/`、`tests/` 之一
2. **新增文件必须放在标准目录下**，对照本文件的结构
3. **测试数据与测试代码分离**：fixtures 放 `tests/fixtures/`，测试代码放 `tests/<模块>/`
4. **开发期工具放 `tools/`**，禁止误放根目录
5. **`internal/` 下的模块不对外暴露**，对外复用代码放 `pkg/`
6. **计划文件统一放 `docs/plans/`**，命名格式 `<功能名>-<日期>.md`

## 模块职责

| 模块 | 职责 | 禁止 |
|------|------|------|
| base | 日志、错误类型 | 业务逻辑 |
| config | opencode.json 解析 | LLM 调用、工具执行 |
| provider | LLM Provider 接口与实现 | Agent 调度 |
| agent | AgentRuntime、主子代理 | 文件 IO、网络协议 |
| tool | read/write/edit/task/bash 工具 | LLM 调用 |
| transport | 本机 channel 通信 | 网络协议、文件 IO |
