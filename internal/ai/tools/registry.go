package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/example/go-ai-scheduler/internal/ai/adapter"
	"github.com/example/go-ai-scheduler/internal/repo"
)

// Tool is a callable function exposed to the LLM agent.
type Tool interface {
	Definition() adapter.Tool
	Execute(ctx context.Context, args json.RawMessage) (any, error)
}

// Registry manages the set of available tools.
type Registry struct {
	tools map[string]Tool
}

// NewRegistry creates a tool registry with the given tools.
func NewRegistry(tools ...Tool) *Registry {
	r := &Registry{tools: make(map[string]Tool)}
	for _, t := range tools {
		def := t.Definition()
		r.tools[def.Function.Name] = t
	}
	return r
}

// Definitions returns the function-calling schemas for all registered tools.
func (r *Registry) Definitions() []adapter.Tool {
	defs := make([]adapter.Tool, 0, len(r.tools))
	for _, t := range r.tools {
		defs = append(defs, t.Definition())
	}
	return defs
}

// Execute runs a tool by name and returns its result.
func (r *Registry) Execute(ctx context.Context, name string, argsJSON string) (any, error) {
	t, ok := r.tools[name]
	if !ok {
		return nil, fmt.Errorf("unknown tool: %s", name)
	}
	return t.Execute(ctx, json.RawMessage(argsJSON))
}

// AllTools returns all tools backed by the repository bundle.
func AllTools(bundle *repo.Bundle) []Tool {
	return []Tool{
		&queryTasksTool{bundle: bundle},
		&queryInstancesTool{bundle: bundle},
		&queryWorkersTool{bundle: bundle},
		&getTaskDetailTool{bundle: bundle},
		&getSystemHealthTool{bundle: bundle},
		&analyzeFailureTool{bundle: bundle},
		&createTaskTool{bundle: bundle},
		&triggerTaskTool{bundle: bundle},
		&pauseTaskTool{bundle: bundle},
		&retryFailedInstanceTool{bundle: bundle},
		&deleteTaskAgentTool{bundle: bundle},
		&getWorkerLoadHistoryTool{bundle: bundle},
	}
}
