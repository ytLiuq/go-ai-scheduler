# Go AI Scheduler 复杂 Demo 测试指南

## 环境准备

```bash
# 清理缓存并启动
go clean -cache && make run-full-stack
```

访问 `http://127.0.0.1:8082/`，用 `admin` / `admin123` 登录。

---

## 当前环境概览

| 资源 | 数量 |
|---|---|
| 任务 | 10（6 启用 / 4 禁用） |
| 实例 | 1800+（含成功/失败/重试） |
| 在线 Worker | 1 |
| DAG 依赖 | invoice-mailer → db-backup, log-cleaner → db-backup |

### 已有任务

| ID | 名称 | 类型 | 调度 | 状态 |
|---|---|---|---|---|
| 1 | ui-test-shell | shell | */5 * * * * | 禁用 |
| 9 | order-sync | shell | */5 * * * * | 启用 |
| 10 | db-backup | shell | 0 2 * * * | 启用 |
| 11 | cache-warmer | shell | 0 * * * * | 启用 |
| 13 | health-check | shell | */1 * * * * | 禁用 |
| 14 | invoice-mailer | shell | 0 8 * * 1-5 | 启用 |
| 15 | log-cleaner | shell | 0 */12 * * * | 启用 |
| 16 | dlq-processor | shell | */30 * * * * | 启用 |

---

## 一、总览页

登录后默认在总览。四个统计卡片展示任务、实例、Worker、失败次数。下方三栏分别展示任务状态分布、Worker 负载、实例统计。

**操作**：无，纯观察。感受全局状态。

---

## 二、任务管理

### 2.1 一句话创建任务

1. 点击「新建任务」
2. 在顶部「一句话创建」输入框输入：

> 每个工作日晚上11点半执行数据库备份检查，用shell类型执行pg_dump校验，超时15分钟，失败后指数退避重试最多5次

3. 点击「AI 解析」
4. **观察点**：
   - cron 是否正确变成 `30 23 * * 1-5`
   - max_retry 是否为 5
   - retry_policy 是否为 exponential_backoff
5. 手动调整后点击「创建任务」

### 2.2 手动填写复杂任务

1. 点击「新建任务」
2. 填写：
   - 任务名称：`data-validator`
   - 任务类型：Shell 脚本
   - Cron 表达式：`0 */6 * * *`（每6小时）
   - 载荷：`python validate_data.py --db production --threshold 0.95`
   - 超时时间：600 秒
   - 最大重试：3
   - 重试策略：指数退避
   - 路由策略：最小负载
3. 点击「创建任务」
4. 在任务列表确认新任务出现

### 2.3 任务操作

- 对 `order-sync` 点击「暂停」→ 状态变为"已暂停" → 再点「恢复」
- 对 `db-backup` 点击「触发」→ 手动执行一次
- 点击「删除」测试删除确认弹窗

---

## 三、实例追踪（分页加载）

1. 点击「实例」标签
2. 第一页加载 20 条
3. 点击「加载更多」翻页，观察按钮显示已加载条数
4. **观察点**：
   - 表头：实例ID、任务ID、触发时间、状态、Worker、重试次数、AI 分析
   - 失败实例有红色徽章，成功有绿色
   - 历史 health-check 实例大量失败（被故意设计为访问 500 端点）
   - 有 AI 分析的实例显示"AI"徽章，hover 可看详情

---

## 四、DAG 依赖图

1. 点击「DAG」标签
2. **当前依赖关系**：
   - `db-backup` → `invoice-mailer`（开发票前必须先备份）
   - `db-backup` → `log-cleaner`（清日志前必须先备份）
3. **观察点**：
   - 箭头从上游指向下游（"A 依赖 B" = 箭头从 B 指向 A）
   - 启用的节点橙色，禁用的灰色
   - 节点显示名称、ID、类型

---

## 五、AI 工具（逐个测试）

### 5.1 故障分析

1. 点击「AI 工具」标签
2. 在故障分析区从下拉框选择一个失败的 health-check 实例（自动填充错误日志）
3. 或手动粘贴一段错误日志到文本框
4. 点击「分析」
5. **观察点**：
   - 严重程度徽章（high=红，medium=黄，low=蓝）
   - 置信度百分比
   - 根因分析和修复建议

### 5.2 自动调度建议

1. 点击「生成建议」
2. AI 自动读取当前系统状态（Worker 负载、任务数、实例数）
3. **观察点**：
   - 建议类型：scale（扩缩容）、throttle（限流）、config（配置调整）
   - 每条建议有标题、描述、置信度、是否可自动执行

### 5.3 时长预测

1. 从下拉框选择 `order-sync`（有大量历史执行数据）
2. 点击「预测」
3. **观察点**：
   - 预测时长（秒）
   - 趋势（stable/increasing/decreasing/volatile）
   - 置信度

### 5.4 趋势分析

1. 输入时间窗口 24（小时）
2. 点击「分析」
3. **观察点**：
   - 综合评估段落
   - 趋势列表（指标名 + 方向）
   - 建议列表（类型 + 紧急程度）

---

## 六、AI 对话（核心功能）

点击「AI 对话」标签，点「新对话」。Agent 内置 12 个工具：查询任务/实例/Worker、创建/暂停/触发/删除任务、分析失败、重试实例、查看系统健康状态、查看 Worker 负载历史。

### 场景 1：系统概览

> 系统整体健康状况怎么样？

Agent 调用 `get_system_health` 工具。观察对话中出现工具调用卡片（⚙ 图标 → ✓ 完成）。

### 场景 2：故障排查

> 查一下最近有没有失败的任务实例？帮我分析失败原因。

Agent 先调用 `query_instances` 查失败列表 → 然后对具体实例调用 `analyze_failure` 深入分析。观察工具调用链的实时展示。

### 场景 3：操作类

> 把 order-sync 先暂停，查一下 dlq-processor 最近10次执行的成功率。

Agent 调用 `pause_task` → `query_instances`。观察操作执行结果。

### 场景 4：创建任务

> 创建一个每天晚上10点执行的日志归档任务，shell类型，把 /var/log 下超过7天的 .log 文件打包压缩到 /backup 目录，超时10分钟，失败重试2次，用固定间隔重试。

Agent 调用 `create_task` → 返回任务 ID → 去任务列表确认任务已创建。

### 场景 5：综合运维分析

> 帮我全面分析一下当前调度系统：有哪些任务配置不合理？哪个任务重试次数过多？有没有任务没设置超时？给出优化建议。

Agent 会依次查所有任务详情 → 对比分析 → 给出报告。

### 场景 6：任务对比

> 对比一下 order-sync 和 dlq-processor 的表现，哪个更稳定？各自失败的主要原因是什么？

Agent 分别查询两个任务的实例数据 → 对比统计 → 给出结论。

### 场景 7：Worker 分析

> 当前 Worker 的负载情况如何？有没有历史负载数据可以看出趋势？

Agent 调用 `query_workers` → `get_worker_load_history` → 分析趋势。

---

## 七、API 直接调用（高级）

```bash
# 登录获取 token
TOKEN=$(curl -s http://127.0.0.1:8082/api/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"admin123"}' | python3 -c "import sys,json; print(json.load(sys.stdin)['token'])")

# 故障分析
curl -s -X POST http://127.0.0.1:8082/api/v1/ai/log-analysis/analyze \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"log":"connection refused: payment-gateway:8443 after 3 retries","error_code":"CONNECTION_REFUSED","task_type":"shell","retry_count":3}'

# 自动调度建议
curl -s -X POST http://127.0.0.1:8082/api/v1/ai/advisor/auto \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' -d '{}'

# 自然语言创建任务
curl -s -X POST http://127.0.0.1:8082/api/v1/ai/task/create \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"input":"每天早上8点备份mysql数据库，mysqldump命令，超时10分钟，重试5次"}'

# 时长预测
curl -s -X POST http://127.0.0.1:8082/api/v1/ai/task/predict-duration \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"task_id":9}'

# 趋势分析
curl -s -X POST http://127.0.0.1:8082/api/v1/ai/trend/analyze \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"time_window_hours":24}'
```

---

## 八、关键观察点汇总

| 功能 | 需要关注 |
|---|---|
| AI 对话 | 工具调用卡片的实时出现（⚙→✓），每个工具执行耗时 |
| DAG 图 | 节点颜色（启用=橙，禁用=灰），箭头方向是否正确 |
| 实例分页 | 「加载更多」按钮 + 已显示计数 |
| 故障分析 | 严重程度颜色 + 置信度百分比 |
| 一句话创建 | Cron 中文化转换准确性（每天早上9点 → 0 9 * * *） |
| 调度建议 | 建议类型 + auto_apply 标志 |
| 时长预测 | 趋势方向 + 置信度 |

---

## 九、推荐测试顺序

1. **总览**（2分钟）→ 了解全局
2. **任务**（5分钟）→ 创建复杂任务 + 暂停/恢复/触发
3. **实例**（3分钟）→ 分页翻看 + 观察失败模式
4. **DAG**（2分钟）→ 确认依赖箭头
5. **AI 工具**（10分钟）→ 逐个测试四个功能
6. **AI 对话**（15分钟）→ 七个场景按顺序对话
