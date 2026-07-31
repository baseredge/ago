// Package transport 实现本进程内 agent 间 channel 通信。
// 无网络协议，直接传指针，零序列化开销。
package transport

import (
	"context"
	"sync"
)

// MessageType 消息类型。
type MessageType string

const (
	// MsgTypeUser 用户输入消息
	MsgTypeUser MessageType = "user"
	// MsgTypeAssistant assistant 输出消息
	MsgTypeAssistant MessageType = "assistant"
	// MsgTypeToolCall 工具调用请求
	MsgTypeToolCall MessageType = "tool_call"
	// MsgTypeToolResult 工具调用结果
	MsgTypeToolResult MessageType = "tool_result"
	// MsgTypeStop 停止信号
	MsgTypeStop MessageType = "stop"
	// MsgTypeError 错误
	MsgTypeError MessageType = "error"
)

// Message 是 agent 间通信的消息结构。
// 本机直接传指针，不序列化。
type Message struct {
	// SrcAgentID 发送方 agent ID
	SrcAgentID string
	// DstAgentID 接收方 agent ID（空表示广播）
	DstAgentID string
	// ReplyTo 回复目标 agent ID（空表示回复到 SrcAgentID）
	// 用于 SendSync 场景：发送方注册临时 replyTo channel，
	// 接收方需把回复发到 ReplyTo 而非 SrcAgentID。
	ReplyTo string
	// Type 消息类型
	Type MessageType
	// Payload 消息内容（文本/工具调用/结果等）
	Payload any
}

// Hub 是 agent 通信中枢，管理所有 agent 的消息通道。
type Hub struct {
	mu    sync.RWMutex
	chans map[string]chan Message // agentID -> 接收 channel
}

// NewHub 创建通信中枢。
func NewHub() *Hub {
	return &Hub{
		chans: make(map[string]chan Message),
	}
}

// Register 注册 agent 的接收 channel，返回该 channel 供 agent 读取。
// buffer 是 channel 缓冲大小。
func (h *Hub) Register(agentID string, buffer int) <-chan Message {
	h.mu.Lock()
	defer h.mu.Unlock()
	if ch, ok := h.chans[agentID]; ok {
		close(ch) // 关闭旧 channel，防止重复注册泄漏
	}
	ch := make(chan Message, buffer)
	h.chans[agentID] = ch
	return ch
}

// Unregister 注销 agent，关闭其接收 channel。
func (h *Hub) Unregister(agentID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if ch, ok := h.chans[agentID]; ok {
		close(ch)
		delete(h.chans, agentID)
	}
}

// Send 发送消息到指定 agent。
// 若 agent 不存在返回 false。
func (h *Hub) Send(ctx context.Context, msg Message) bool {
	h.mu.RLock()
	ch, ok := h.chans[msg.DstAgentID]
	h.mu.RUnlock()
	if !ok {
		return false
	}
	select {
	case ch <- msg:
		return true
	case <-ctx.Done():
		return false
	}
}

// SendSync 同步发送消息并等待回复。
// replyTo 是发送方用于接收回复的 agent ID，会写入 msg.ReplyTo 字段。
// 接收方应将回复发到 ReplyTo（而非 SrcAgentID）。
// 返回回复消息和是否成功。
func (h *Hub) SendSync(ctx context.Context, msg Message, replyTo string) (*Message, bool) {
	// 注册临时回复 channel
	replyCh := h.Register(replyTo, 1)
	defer h.Unregister(replyTo)

	// 设置 ReplyTo，让接收方知道回哪里
	msg.ReplyTo = replyTo

	// 发送消息
	if !h.Send(ctx, msg) {
		return nil, false
	}

	// 等待回复
	select {
	case reply, ok := <-replyCh:
		return &reply, ok
	case <-ctx.Done():
		return nil, false
	}
}
