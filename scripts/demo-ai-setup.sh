#!/usr/bin/env bash
# AI Demo Setup — creates a complex e-commerce monitoring scenario.
set -euo pipefail

API="http://127.0.0.1:8082"

echo "=== Step 1: Login ==="
LOGIN=$(curl -sf "$API/api/auth/login" \
  -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"admin123"}')
TOKEN=$(python3 -c "import sys,json; print(json.loads(sys.stdin.read())['token'])" <<<"$LOGIN")
echo "Token obtained: ${TOKEN:0:20}..."
H="-H Content-Type:application/json -H Authorization:Bearer $TOKEN"

echo ""
echo "=== Step 2: Create 8 realistic tasks (e-commerce platform) ==="

# Task 1: Order sync — HTTP, runs every 5 min
ID1=$(curl -sf -X POST $API/api/v1/tasks $H \
  -d '{"name":"order-sync","type":"shell","cron_expr":"*/5 * * * *","payload":"curl -s https://httpbin.org/status/200","timeout_seconds":30,"max_retry":3,"retry_policy":"fixed_interval","route_strategy":"least_loaded"}' | python3 -c "import sys,json;d=json.load(sys.stdin);print(d.get('ID',d.get('id','?')))")
echo "  Created task $ID1: order-sync (sync orders every 5min, HTTP)"

# Task 2: Data backup — Shell, daily at 2am
ID2=$(curl -sf -X POST $API/api/v1/tasks $H \
  -d '{"name":"db-backup","type":"shell","cron_expr":"0 2 * * *","payload":"pg_dump production > /backup/db_$(date +%Y%m%d).sql","timeout_seconds":600,"max_retry":2,"retry_policy":"exponential_backoff","route_strategy":"least_loaded"}' | python3 -c "import sys,json;d=json.load(sys.stdin);print(d.get('ID',d.get('id','?')))")
echo "  Created task $ID2: db-backup (daily DB backup, 10min timeout)"

# Task 3: Cache warmer — Shell, hourly
ID3=$(curl -sf -X POST $API/api/v1/tasks $H \
  -d '{"name":"cache-warmer","type":"shell","cron_expr":"0 * * * *","payload":"redis-cli KEYS '*' | xargs -I{} redis-cli EXPIRE {} 3600","timeout_seconds":120,"max_retry":2,"retry_policy":"fixed_interval","route_strategy":"least_loaded"}' | python3 -c "import sys,json;d=json.load(sys.stdin);print(d.get('ID',d.get('id','?')))")
echo "  Created task $ID3: cache-warmer (warm up Redis cache, hourly)"

# Task 4: Report generator — container, daily at 6am
ID4=$(curl -sf -X POST $API/api/v1/tasks $H \
  -d '{"name":"daily-report","type":"container","cron_expr":"0 6 * * *","payload":"python generate_report.py --date yesterday","image":"myregistry/reports:latest","timeout_seconds":300,"max_retry":1,"retry_policy":"exponential_backoff","route_strategy":"round_robin"}' | python3 -c "import sys,json;d=json.load(sys.stdin);print(d.get('ID',d.get('id','?')))")
echo "  Created task $ID4: daily-report (container task, daily reports)"

# Task 5: Health checker — HTTP, every 1 min
ID5=$(curl -sf -X POST $API/api/v1/tasks $H \
  -d '{"name":"health-check","type":"shell","cron_expr":"*/1 * * * *","payload":"curl -sf https://httpbin.org/status/500 && echo OK || echo FAIL","timeout_seconds":10,"max_retry":1,"retry_policy":"fixed_interval","route_strategy":"least_loaded"}' | python3 -c "import sys,json;d=json.load(sys.stdin);print(d.get('ID',d.get('id','?')))")
echo "  Created task $ID5: health-check (will FAIL — hits 500 endpoint, every 1min)"

# Task 6: Invoice mailer — Shell, weekdays at 8am
ID6=$(curl -sf -X POST $API/api/v1/tasks $H \
  -d '{"name":"invoice-mailer","type":"shell","cron_expr":"0 8 * * 1-5","payload":"python send_invoices.py --batch-size 500","timeout_seconds":180,"max_retry":5,"retry_policy":"exponential_backoff","route_strategy":"round_robin"}' | python3 -c "import sys,json;d=json.load(sys.stdin);print(d.get('ID',d.get('id','?')))")
echo "  Created task $ID6: invoice-mailer (weekdays, 5 retries, batch 500)"

# Task 7: Log cleaner — Shell, every 12 hours
ID7=$(curl -sf -X POST $API/api/v1/tasks $H \
  -d '{"name":"log-cleaner","type":"shell","cron_expr":"0 */12 * * *","payload":"find /var/log -name '*.log' -mtime +7 -delete","timeout_seconds":60,"max_retry":1,"retry_policy":"fixed_interval","route_strategy":"least_loaded"}' | python3 -c "import sys,json;d=json.load(sys.stdin);print(d.get('ID',d.get('id','?')))")
echo "  Created task $ID7: log-cleaner (clean old logs every 12h)"

# Task 8: Dead letter queue processor — Shell, every 30 min
ID8=$(curl -sf -X POST $API/api/v1/tasks $H \
  -d '{"name":"dlq-processor","type":"shell","cron_expr":"*/30 * * * *","payload":"python process_dlq.py --limit 100","timeout_seconds":90,"max_retry":3,"retry_policy":"error_code","route_strategy":"least_loaded"}' | python3 -c "import sys,json;d=json.load(sys.stdin);print(d.get('ID',d.get('id','?')))")
echo "  Created task $ID8: dlq-processor (process dead letters, 30min)"

echo ""
echo "=== Step 3: Enable all tasks ==="
for tid in $ID1 $ID2 $ID3 $ID4 $ID5 $ID6 $ID7 $ID8; do
  curl -sf -X POST "$API/api/v1/tasks/$tid/resume" $H >/dev/null
  echo "  Task $tid resumed (enabled)"
done

echo ""
echo "=== Step 4: Trigger tasks to generate instances (some will fail) ==="
# Trigger health-check 5 times (will all fail — hits /status/500)
for i in $(seq 1 5); do
  curl -sf -X POST "$API/api/v1/tasks/$ID5/trigger" $H >/dev/null
  echo "  Triggered health-check (attempt $i — expected failure)"
done

# Trigger order-sync 10 times (should succeed)
for i in $(seq 1 10); do
  curl -sf -X POST "$API/api/v1/tasks/$ID1/trigger" $H >/dev/null
done
echo "  Triggered order-sync 10x"

# Trigger cache-warmer 5 times
for i in $(seq 1 5); do
  curl -sf -X POST "$API/api/v1/tasks/$ID3/trigger" $H >/dev/null
done
echo "  Triggered cache-warmer 5x"

# Trigger dlq-processor 3 times
for i in $(seq 1 3); do
  curl -sf -X POST "$API/api/v1/tasks/$ID8/trigger" $H >/dev/null
done
echo "  Triggered dlq-processor 3x"

# Trigger daily-report 2 times
for i in $(seq 1 2); do
  curl -sf -X POST "$API/api/v1/tasks/$ID4/trigger" $H >/dev/null
done
echo "  Triggered daily-report 2x"

echo ""
echo "=== Step 5: Wait for execution and check instances ==="
sleep 8

echo ""
echo "--- Instance Summary ---"
curl -sf "$API/api/v1/task-instances?limit=50" $H | python3 -c "
import sys, json
data = json.load(sys.stdin)
by_status = {}
for i in data:
    s = i.get('Status', i.get('status', 'unknown'))
    by_status[s] = by_status.get(s, 0) + 1
print('Total instances:', len(data))
for s, c in sorted(by_status.items()):
    print(f'  {s}: {c}')
"

echo ""
echo "--- Failed Instances (sample) ---"
curl -sf "$API/api/v1/task-instances?status=failed&limit=3" $H | python3 -c "
import sys, json
data = json.load(sys.stdin)
for i in data[:3]:
    print(f\"  Instance #{i.get('ID',i.get('id','?'))} task={i.get('task_id',i.get('TaskID','?'))} status={i.get('Status',i.get('status','?'))} error={i.get('error_message',i.get('ErrorMessage','--'))}\")
"

# Print DAG setup instructions
echo ""
echo "=== Step 6: Set up DAG dependency ==="
# Make daily-report depend on db-backup
curl -sf -X PUT "$API/api/v1/tasks/$ID4" $H \
  -d "{\"name\":\"daily-report\",\"type\":\"container\",\"cron_expr\":\"0 6 * * *\",\"payload\":\"python generate_report.py --date yesterday\",\"image\":\"myregistry/reports:latest\",\"timeout_seconds\":300,\"max_retry\":1,\"retry_policy\":\"exponential_backoff\",\"route_strategy\":\"round_robin\",\"depends_on\":[$ID2]}" >/dev/null
echo "  daily-report now depends on db-backup (DAG relationship)"

echo ""
echo "=== DONE — Demo scenario ready ==="
echo ""
echo "Created tasks:"
echo "  $ID1: order-sync      (HTTP sync, every 5min, 10 runs done)"
echo "  $ID2: db-backup       (DB backup, daily 2am)"
echo "  $ID3: cache-warmer    (Redis warmer, hourly, 5 runs done)"
echo "  $ID4: daily-report    (Container reports, daily 6am, depends on db-backup)"
echo "  $ID5: health-check    (Health endpoint — DELIBERATELY FAILING, 5 failures)"
echo "  $ID6: invoice-mailer  (Invoice email, weekdays 8am)"
echo "  $ID7: log-cleaner     (Log rotation, every 12h)"
echo "  $ID8: dlq-processor   (Dead letter queue, 30min, 3 runs done)"
echo ""
echo "Now open http://127.0.0.1:8082/ and try the AI features!"
