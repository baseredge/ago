package transport_test

import (
	"context"
	"testing"
	"time"

	"ago/internal/transport"
)

// TestRegisterAndSend 测试注册 agent 并发送消息
func TestRegisterAndSend(t *testing.T) {
	hub := transport.NewHub()
	defer hub.Unregister("agent-a")

	ch := hub.Register("agent-a", 8)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	ok := hub.Send(ctx, transport.Message{
		SrcAgentID: "sender",
		DstAgentID: "agent-a",
		Type:       transport.MsgTypeUser,
		Payload:    "hello",
	})
	if !ok {
		t.Fatal("Send returned false")
	}

	select {
	case msg := <-ch:
		if msg.Payload != "hello" {
			t.Errorf("Payload = %v, want %q", msg.Payload, "hello")
		}
	case <-ctx.Done():
		t.Fatal("timeout waiting for message")
	}
}

// TestSendToUnknown 测试发送到不存在的 agent
func TestSendToUnknown(t *testing.T) {
	hub := transport.NewHub()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	ok := hub.Send(ctx, transport.Message{
		SrcAgentID: "sender",
		DstAgentID: "nonexistent",
		Type:       transport.MsgTypeUser,
		Payload:    "hello",
	})
	if ok {
		t.Error("Send to unknown agent should return false")
	}
}

// TestSendSync 测试同步发送并等待回复
func TestSendSync(t *testing.T) {
	hub := transport.NewHub()
	defer hub.Unregister("agent-b")

	// 启动 agent-b 接收并回复到 ReplyTo
	go func() {
		ch := hub.Register("agent-b", 8)
		for msg := range ch {
			// 回复到 ReplyTo（与 agent.handleMessage 逻辑一致）
			replyDst := msg.ReplyTo
			if replyDst == "" {
				replyDst = msg.SrcAgentID
			}
			hub.Send(context.Background(), transport.Message{
				SrcAgentID: "agent-b",
				DstAgentID: replyDst,
				Type:       transport.MsgTypeAssistant,
				Payload:    "reply: " + msg.Payload.(string),
			})
		}
	}()

	// 等待 agent-b 注册
	time.Sleep(50 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	reply, ok := hub.SendSync(ctx, transport.Message{
		SrcAgentID: "caller",
		DstAgentID: "agent-b",
		Type:       transport.MsgTypeUser,
		Payload:    "ping",
	}, "caller-reply")
	if !ok || reply == nil {
		t.Fatal("SendSync returned false or nil reply")
	}
	if reply.Payload != "reply: ping" {
		t.Errorf("Reply = %v, want %q", reply.Payload, "reply: ping")
	}
}

// TestUnregister 测试注销后 channel 关闭
func TestUnregister(t *testing.T) {
	hub := transport.NewHub()
	ch := hub.Register("agent-c", 8)
	hub.Unregister("agent-c")

	// channel 应该关闭
	_, ok := <-ch
	if ok {
		t.Error("channel should be closed after Unregister")
	}
}

// TestReregister 测试重复注册关闭旧 channel
func TestReregister(t *testing.T) {
	hub := transport.NewHub()
	ch1 := hub.Register("agent-d", 8)
	ch2 := hub.Register("agent-d", 8) // 重复注册
	defer hub.Unregister("agent-d")

	// ch1 应该关闭
	_, ok := <-ch1
	if ok {
		t.Error("old channel should be closed after re-register")
	}

	// ch2 应该可用
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	hub.Send(ctx, transport.Message{
		SrcAgentID: "x",
		DstAgentID: "agent-d",
		Type:       transport.MsgTypeUser,
		Payload:    "test",
	})
	select {
	case <-ch2:
		// 收到消息
	case <-ctx.Done():
		t.Error("timeout waiting on re-registered channel")
	}
}
