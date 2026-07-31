package agent

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"ago/internal/base"
	"ago/internal/config"
	"ago/internal/provider"
	"ago/internal/tool"
	"ago/internal/transport"
)

// subagentTimeout 子代理调用默认超时，防止 LLM 异常时 goroutine 泄漏。
const subagentTimeout = 5 * time.Minute

// SubagentManager 管理子代理的创建和调用。
// 实现主代理通过 task 工具调用子代理的逻辑。
type SubagentManager struct {
	hub       *transport.Hub
	factory   *provider.Factory
	cfg       *config.Config
	parentID  string                   // 主代理 ID
	subagents map[string]*AgentRuntime // 活跃子代理
	mu        sync.Mutex
	counter   atomic.Int64 // 子代理 ID 计数器
}

// NewSubagentManager 创建子代理管理器。
func NewSubagentManager(hub *transport.Hub, factory *provider.Factory, cfg *config.Config, parentID string) *SubagentManager {
	m := &SubagentManager{
		hub:       hub,
		factory:   factory,
		cfg:       cfg,
		parentID:  parentID,
		subagents: make(map[string]*AgentRuntime),
	}
	// 注入 task 工具处理器
	tool.TaskHandler = m.executeTask
	return m
}

// executeTask 是 task 工具的实际执行函数（注入到 tool.TaskHandler）。
// 根据子代理名查找配置，创建独立 goroutine 的子代理，同步等待结果。
func (m *SubagentManager) executeTask(input tool.TaskInput) (*tool.TaskResult, error) {
	// 查找子代理配置
	agentCfg, ok := m.cfg.Agents[input.SubagentName]
	if !ok {
		return nil, fmt.Errorf("subagent %q not found in config", input.SubagentName)
	}

	// 确定模型：子代理配置优先，否则用主代理模型
	model := agentCfg.Model
	if model == "" {
		model = m.cfg.Model
	}
	if model == "" {
		return nil, fmt.Errorf("no model configured for subagent %q", input.SubagentName)
	}

	// 生成唯一 agent ID
	subID := fmt.Sprintf("%s-sub-%s-%d", m.parentID, input.SubagentName, m.counter.Add(1))

	// 创建子代理
	subAgent, err := New(
		subID,
		model,
		m.hub,
		m.factory,
		m.cfg,
		WithSystem(agentCfg.System),
		WithMode(agentCfg.Mode),
		WithTools(agentCfg.Tools),
	)
	if err != nil {
		return nil, fmt.Errorf("create subagent: %w", err)
	}

	// 注册到管理器
	m.mu.Lock()
	m.subagents[subID] = subAgent
	m.mu.Unlock()

	// 确保退出时清理
	defer func() {
		subAgent.Stop()
		m.mu.Lock()
		delete(m.subagents, subID)
		m.mu.Unlock()
	}()

	// 启动子代理（带超时，防止 LLM 异常时 goroutine 泄漏）
	ctx, cancel := context.WithTimeout(context.Background(), subagentTimeout)
	defer cancel()
	if err := subAgent.Start(ctx); err != nil {
		return nil, fmt.Errorf("start subagent: %w", err)
	}

	// 通过 hub 发送任务并等待回复
	reply, ok := m.hub.SendSync(ctx, transport.Message{
		SrcAgentID: m.parentID,
		DstAgentID: subID,
		Type:       transport.MsgTypeUser,
		Payload:    input.Prompt,
	}, subID+"-reply")
	if !ok || reply == nil {
		return nil, fmt.Errorf("subagent %s did not respond", input.SubagentName)
	}

	if reply.Type == transport.MsgTypeError {
		errMsg, _ := reply.Payload.(string)
		return nil, fmt.Errorf("subagent error: %s", errMsg)
	}

	output, _ := reply.Payload.(string)
	base.Debugf("subagent %s completed (parent=%s)", subID, m.parentID)
	return &tool.TaskResult{
		SubagentName: input.SubagentName,
		Output:       output,
	}, nil
}

// StopAll 停止所有活跃子代理（主代理退出时调用，防止 goroutine 泄漏）。
func (m *SubagentManager) StopAll() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, sa := range m.subagents {
		sa.Stop()
		delete(m.subagents, id)
	}
}
