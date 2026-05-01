package agent

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/example/go-ai-scheduler/internal/ai/adapter"
	"github.com/example/go-ai-scheduler/internal/ai/stream"
	"github.com/example/go-ai-scheduler/internal/ai/tools"
)

const maxToolIterations = 10

// RunResult captures the complete assistant response for storage.
type RunResult struct {
	Content   string   // final text response
	ToolCalls []string // names of tools called
}

// Run executes a single user message through the agent loop and streams
// the response through the SSE writer. It also returns the final text for storage.
func Run(ctx context.Context, llm *adapter.LLMAdapter, registry *tools.Registry, systemPrompt string, history []adapter.Message, userMessage string, sw *stream.Writer) (*RunResult, error) {
	if llm == nil || !llm.Enabled() {
		sw.Error(fmt.Errorf("LLM not configured"))
		return nil, fmt.Errorf("LLM not configured")
	}

	// Build initial messages.
	messages := []adapter.Message{
		{Role: "system", Content: systemPrompt},
	}
	messages = append(messages, history...)
	messages = append(messages, adapter.Message{Role: "user", Content: userMessage})

	toolDefs := registry.Definitions()
	var finalContent strings.Builder
	toolCallsMade := make([]string, 0)

	// Use a longer timeout HTTP client for the agent loop.
	agentClient := &http.Client{Timeout: 120 * time.Second}

	for iteration := 0; iteration < maxToolIterations; iteration++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		ch := llm.CompleteStreamWithClient(ctx, agentClient, messages, toolDefs)
		var currentContent strings.Builder
		var pendingToolCalls []adapter.ToolCall
		var reasoningContent string
		streamErr := error(nil)

		for ev := range ch {
			if ev.Error != nil {
				streamErr = ev.Error
				break
			}
			if ev.DeltaContent != "" {
				currentContent.WriteString(ev.DeltaContent)
				sw.Text(ev.DeltaContent)
			}
			if ev.ReasoningContent != "" {
				reasoningContent = ev.ReasoningContent
			}
			if len(ev.ToolCalls) > 0 {
				pendingToolCalls = ev.ToolCalls
			}
		}

		if streamErr != nil {
			sw.Error(streamErr)
			return nil, streamErr
		}

		// If LLM wants to call tools.
		if len(pendingToolCalls) > 0 {
			assistantMsg := adapter.Message{
				Role:             "assistant",
				Content:          currentContent.String(),
				ReasoningContent: reasoningContent,
				ToolCalls:        pendingToolCalls,
			}
			messages = append(messages, assistantMsg)

			for _, tc := range pendingToolCalls {
				sw.ToolCall(tc.Function.Name, []byte(tc.Function.Arguments))

				result, err := registry.Execute(ctx, tc.Function.Name, tc.Function.Arguments)
				if err != nil {
					errMsg := fmt.Sprintf("tool execution failed: %v", err)
					sw.ToolResult(tc.Function.Name, map[string]string{"error": errMsg})
					messages = append(messages, adapter.Message{
						Role:       "tool",
						ToolCallID: tc.ID,
						Content:    errMsg,
					})
				} else {
					sw.ToolResult(tc.Function.Name, result)
					resultJSON := fmt.Sprintf("%v", result)
					messages = append(messages, adapter.Message{
						Role:       "tool",
						ToolCallID: tc.ID,
						Content:    resultJSON,
					})
					toolCallsMade = append(toolCallsMade, tc.Function.Name)
				}
			}
			continue // feed tool results back to LLM
		}

		// No tool calls — final response.
		finalContent.WriteString(currentContent.String())
		sw.Done()
		return &RunResult{
			Content:   finalContent.String(),
			ToolCalls: toolCallsMade,
		}, nil
	}

	sw.Error(fmt.Errorf("max tool iterations exceeded"))
	return nil, fmt.Errorf("max tool iterations exceeded")
}

// SystemPrompt is the default system prompt for the agent.
const SystemPrompt = `你是一个任务调度系统的 AI 运维助手。你可以：
1. 查询任务、实例、worker 的状态
2. 分析失败原因并给出修复建议
3. 创建、暂停、恢复、触发任务
4. 提供系统健康概览

你的回复风格：
- 使用中文，简洁明了
- 当查询结果很多时，只展示关键数据并总结
- 如果发现问题，给出具体的行动建议
- 不要编造你没有查询到的数据

当用户要创建一个任务但信息不全时，主动询问缺失的信息（名称、类型、cron表达式、执行内容）。
当用户询问系统状态时，先调用 get_system_health 获取概览数据。
当用户询问具体任务时，先调用 query_tasks 查找，再用 get_task_detail 查看详情。`

// BuildChatMessages converts stored messages to adapter messages for the LLM.
func BuildChatMessages(history []StoredMessage) []adapter.Message {
	msgs := make([]adapter.Message, 0, len(history))
	for _, h := range history {
		msgs = append(msgs, adapter.Message{
			Role:    h.Role,
			Content: h.Content,
		})
	}
	return msgs
}

// StoredMessage is a conversation message from the database.
type StoredMessage struct {
	Role    string
	Content string
}

// Logf is a package-level logger override for debugging.
var Logf = log.Printf
