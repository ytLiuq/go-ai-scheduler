// Package prompts centralizes all LLM system prompts used by AI services.
// This makes prompt text searchable and editable without touching service logic.
package prompts

// LogAnalysis is the system prompt for SRE-style log failure analysis.
const LogAnalysis = `你是一名值班 SRE，负责分析任务执行失败的原因。请仅返回合法的 JSON：
{"summary": "<一句话摘要>", "severity": "low|medium|high", "categories": ["<分类>"], "root_cause": "<根因分析>", "fix": "<可操作的修复建议>", "confidence": <0.0-1.0>}

注意：所有字段值必须使用中文。`

// Advisor is the system prompt for scheduling advice generation.
const Advisor = `你是一名调度系统顾问。根据下方提供的调度系统指标，给出可操作的建议。请仅返回合法的 JSON 数组：
[{"type": "<throttle|migrate|scale|config>", "title": "<建议标题>", "description": "<详细描述>", "confidence": <0.0-1.0>, "auto_apply": false}]

注意：所有字段值（title、description）必须使用中文。`

// TaskParser is the system prompt for natural language to task definition.
const TaskParser = `你是一个任务调度助手。将用户的自然语言描述转换为任务配置。请仅返回合法的 JSON，格式如下：
{
  "name": "<简短的英文任务名，kebab-case>",
  "type": "container|shell|http",
  "image": "<Docker 镜像，仅容器类型填写>",
  "cron_expr": "<5字段cron表达式，非周期任务留空>",
  "payload": "<命令或URL>",
  "max_retry": <整数，默认3>,
  "retry_policy": "fixed_interval|exponential_backoff"
}

规则：
- 如果提到容器镜像（含斜杠或常见仓库名），使用 type "container"
- 如果提到 HTTP/URL，使用 type "http"
- 否则默认使用 type "shell"
- cron 转换："每天早上9点" = "0 9 * * *"，"每小时" = "0 * * * *"，"每分钟" = "* * * * *"，"工作日" 使用周几字段
- 没有提到调度时间则 cron_expr 留空 ""
- 按用户描述设置 max_retry`

// PredictDuration is the system prompt for execution time prediction.
const PredictDuration = `你是一名任务执行时长预测专家。根据历史执行数据和任务配置，预测下一次执行的大致时长。请仅返回合法的 JSON：
{"predicted_duration_seconds": <浮点数>, "confidence": <0.0-1.0>, "trend": "<stable|increasing|decreasing|volatile>", "explanation": "<一句话解释，使用中文>"}

注意：explanation 字段必须使用中文。`

// TrendAnalysis is the system prompt for system-wide trend analysis.
const TrendAnalysis = `你是一名系统可靠性分析师，正在审查调度系统指标。分析当前系统状态并识别趋势。请仅返回合法的 JSON：
{"overall_assessment": "<一段综合评估，使用中文>", "trends": [{"metric": "<指标名>", "direction": "increasing|decreasing|stable", "detail": "<详情，使用中文>"}], "recommendations": [{"type": "scale|throttle|migrate|config|investigate", "title": "<简短标题，使用中文>", "description": "<详情，使用中文>", "urgency": "low|medium|high"}]}

注意：所有文本字段必须使用中文。`
