const API = '';

const { createApp, ref, reactive, onMounted, computed } = Vue;

createApp({
  setup() {
    const token = ref(localStorage.getItem('scheduler_token') || '');
    const role = ref(localStorage.getItem('scheduler_role') || '');
    const loginUser = ref('');
    const loginPass = ref('');
    const loginError = ref('');
    const tab = ref('dashboard');

    const stats = reactive({ tasks: 0, workers: 0, instances: 0, enabled: 0 });
    const tasks = ref([]);
    const workers = ref([]);
    const instances = ref([]);
    const taskModal = ref(null);
    const nlTaskInput = ref('');
    const nlTaskLoading = ref(false);
    const editingTask = reactive({
      id: 0,
      name: '',
      type: 'shell',
      cron_expr: '',
      payload: '',
      image: '',
      timeout_seconds: 300,
      max_retry: 3,
      retry_policy: 'fixed_interval',
      route_strategy: 'least_loaded'
    });
    const aiLoading = reactive({ log: false, advisor: false, autoAdvisor: false, status: false, predict: false, trend: false });
    const aiStatus = reactive({
      status: 'unknown',
      service: 'ai-service',
      llm_enabled: false,
      model: '',
      endpoint: '',
      api_key_present: false,
      server_time: '',
      error: ''
    });

    const aiLog = reactive({
      log: 'dial tcp 10.0.0.8:443: connection refused',
      error_code: 'conn_refused',
      task_type: 'http',
      retry_count: 1,
      result: '',
      resultObj: null,
      instanceId: null,
      failedOptions: []
    });
    const aiAdvisor = reactive({
      avg_worker_load: 0.82,
      total_workers: 12,
      online_workers: 10,
      pending_instances: 950,
      failed_last_hour: 14,
      avg_dispatch_latency_ms: 126,
      max_pending_config: 1000,
      result: '',
      resultObj: null
    });
    const aiAutoAdvisor = reactive({ result: '', resultObj: null });
    const aiPredict = reactive({ taskId: null, result: '', resultObj: null });
    const aiTrend = reactive({ timeWindowHours: 24, result: '', resultObj: null });

    // --- Chat state ---
    const conversations = ref([]);
    const currentConversationId = ref('');
    const chatMessages = ref([]);
    const chatInput = ref('');
    const chatStreaming = ref(false);
    const chatStreamContent = ref('');
    const chatToolCalls = ref([]);
    const chatError = ref('');
    const chatMessagesEl = ref(null);

    const headers = () => token.value
      ? { Authorization: 'Bearer ' + token.value, 'Content-Type': 'application/json' }
      : { 'Content-Type': 'application/json' };

    async function api(path, method = 'GET', body = null) {
      const opts = { method, headers: headers() };
      if (body) opts.body = JSON.stringify(body);
      const resp = await fetch(API + path, opts);
      if (resp.status === 401) {
        logout();
        throw new Error('unauthorized');
      }
      if (!resp.ok) {
        const e = await resp.json().catch(() => ({}));
        throw new Error(e.error || resp.statusText);
      }
      if (resp.status === 204) return null;
      return resp.json();
    }

    function pick(obj, ...keys) {
      for (const key of keys) {
        if (obj && obj[key] !== undefined && obj[key] !== null) return obj[key];
      }
      return undefined;
    }

    function normalizeTask(task) {
      return {
        ...task,
        id: pick(task, 'id', 'ID'),
        name: pick(task, 'name', 'Name'),
        type: pick(task, 'type', 'Type'),
        cron_expr: pick(task, 'cron_expr', 'CronExpr'),
        payload: pick(task, 'payload', 'Payload'),
        image: pick(task, 'image', 'Image'),
        status: pick(task, 'status', 'Status'),
        timeout_seconds: pick(task, 'timeout_seconds', 'TimeoutSeconds'),
        max_retry: pick(task, 'max_retry', 'MaxRetry'),
        retry_policy: pick(task, 'retry_policy', 'RetryPolicy'),
        route_strategy: pick(task, 'route_strategy', 'RouteStrategy'),
        next_trigger_time: pick(task, 'next_trigger_time', 'NextTriggerTime')
      };
    }

    function normalizeWorker(worker) {
      return {
        ...worker,
        id: pick(worker, 'id', 'ID'),
        hostname: pick(worker, 'hostname', 'Hostname'),
        ip: pick(worker, 'ip', 'IP'),
        callback_url: pick(worker, 'callback_url', 'CallbackURL'),
        grpc_addr: pick(worker, 'grpc_addr', 'GRPCAddr'),
        protocol: pick(worker, 'protocol', 'Protocol'),
        status: pick(worker, 'status', 'Status'),
        labels: pick(worker, 'labels', 'Labels'),
        max_concurrency: pick(worker, 'max_concurrency', 'MaxConcurrency'),
        current_load: pick(worker, 'current_load', 'CurrentLoad'),
        last_heartbeat_at: pick(worker, 'last_heartbeat_at', 'LastHeartbeatAt')
      };
    }

    function normalizeInstance(instance) {
      return {
        ...instance,
        id: pick(instance, 'id', 'ID'),
        task_id: pick(instance, 'task_id', 'TaskID'),
        schedule_instance_id: pick(instance, 'schedule_instance_id', 'ScheduleInstanceID'),
        worker_id: pick(instance, 'worker_id', 'WorkerID'),
        status: pick(instance, 'status', 'Status'),
        retry_count: pick(instance, 'retry_count', 'RetryCount'),
        error_code: pick(instance, 'error_code', 'ErrorCode'),
        error_message: pick(instance, 'error_message', 'ErrorMessage'),
        trigger_time: pick(instance, 'trigger_time', 'TriggerTime'),
        ai_analysis: pick(instance, 'ai_analysis', 'AnalysisJSON')
      };
    }

    function safePercent(part, total) {
      if (!total) return '0%';
      return Math.round((part / total) * 100) + '%';
    }

    function ratioLabel(current, max) {
      if (!max) return '0%';
      return Math.round((current / max) * 100) + '%';
    }

    const roleLabel = computed(() => {
      const labels = { admin: 'Admin', operator: 'Operator', viewer: 'Viewer' };
      return labels[role.value] || role.value || 'Guest';
    });

    const failedInstances = computed(() => instances.value.filter(i => i.status === 'failed').length);
    const runningInstances = computed(() => instances.value.filter(i => !['failed', 'success'].includes(i.status)).length);
    const successfulInstances = computed(() => instances.value.filter(i => i.status === 'success').length);
    const offlineWorkers = computed(() => workers.value.filter(w => w.status !== 'online').length);

    const avgWorkerLoadValue = computed(() => {
      const online = workers.value.filter(w => w.status === 'online');
      if (!online.length) return 0;
      const total = online.reduce((sum, worker) => {
        if (!worker.max_concurrency) return sum;
        return sum + (worker.current_load || 0) / worker.max_concurrency;
      }, 0);
      return total / online.length;
    });

    const highestWorkerLoadValue = computed(() => workers.value.reduce((max, worker) => {
      if (!worker.max_concurrency) return max;
      const value = (worker.current_load || 0) / worker.max_concurrency;
      return value > max ? value : max;
    }, 0));

    const avgWorkerLoadLabel = computed(() => Math.round(avgWorkerLoadValue.value * 100) + '%');
    const highestWorkerLoadLabel = computed(() => Math.round(highestWorkerLoadValue.value * 100) + '%');
    const taskEnableRate = computed(() => safePercent(stats.enabled, stats.tasks));
    const workerOnlineRate = computed(() => safePercent(stats.workers, workers.value.length));
    const instanceFailureRate = computed(() => safePercent(failedInstances.value, instances.value.length));

    const dashboardStats = computed(() => [
      { label: 'Total Tasks', value: stats.tasks, note: '控制台已加载的任务总数', icon: 'T' },
      { label: 'Recent Instances', value: stats.instances, note: '最近实例数据样本', icon: 'I' },
      { label: 'Online Workers', value: stats.workers, note: workers.value.length ? '节点在线状态已汇总' : '尚未发现 Worker', icon: 'W' },
      { label: 'Failed Runs', value: failedInstances.value, note: '最近实例中的失败次数', icon: '!' }
    ]);

    async function doLogin() {
      loginError.value = '';
      try {
        const resp = await api('/api/auth/login', 'POST', {
          username: loginUser.value,
          password: loginPass.value
        });
        token.value = resp.token;
        role.value = resp.role;
        localStorage.setItem('scheduler_token', resp.token);
        localStorage.setItem('scheduler_role', resp.role);
        await Promise.all([loadTasks(), loadWorkers(), loadInstances(), loadAIStatus()]);
      } catch (e) {
        loginError.value = e.message;
      }
    }

    function logout() {
      token.value = '';
      role.value = '';
      localStorage.removeItem('scheduler_token');
      localStorage.removeItem('scheduler_role');
    }

    async function loadTasks() {
      try {
        const data = await api('/api/v1/tasks');
        tasks.value = (data || []).map(normalizeTask);
        stats.tasks = tasks.value.length;
        stats.enabled = tasks.value.filter(t => t.status === 'enabled').length;
      } catch (e) {
        console.error(e);
      }
    }

    async function loadWorkers() {
      try {
        const data = await api('/api/v1/workers');
        workers.value = (data || []).map(normalizeWorker);
        stats.workers = workers.value.filter(w => w.status === 'online').length;
      } catch (e) {
        console.error(e);
      }
    }

    async function loadInstances() {
      try {
        const data = await api('/api/v1/task-instances?limit=50');
        instances.value = (data || []).map(normalizeInstance);
        stats.instances = instances.value.length;
      } catch (e) {
        console.error(e);
      }
    }

    async function loadAIStatus() {
      aiLoading.status = true;
      aiStatus.error = '';
      try {
        const data = await api('/api/v1/ai/status');
        Object.assign(aiStatus, data || {}, { error: '' });
      } catch (e) {
        Object.assign(aiStatus, {
          status: 'error',
          mode: '',
          llm_enabled: false,
          model: '',
          endpoint: '',
          api_key_present: false,
          server_time: '',
          error: e.message
        });
      } finally {
        aiLoading.status = false;
      }
    }

    function resetEditingTask() {
      Object.assign(editingTask, {
        id: 0,
        name: '',
        type: 'shell',
        cron_expr: '',
        payload: '',
        image: '',
        timeout_seconds: 300,
        max_retry: 3,
        retry_policy: 'fixed_interval',
        route_strategy: 'least_loaded'
      });
    }

    function showTaskModal(task) {
      if (task) {
        Object.assign(editingTask, {
          id: task.id,
          name: task.name,
          type: task.type,
          cron_expr: task.cron_expr || '',
          payload: task.payload || '',
          image: task.image || '',
          timeout_seconds: task.timeout_seconds || 300,
          max_retry: task.max_retry || 3,
          retry_policy: task.retry_policy || 'fixed_interval',
          route_strategy: task.route_strategy || 'least_loaded'
        });
      } else {
        resetEditingTask();
      }
      taskModal.value = true;
    }

    async function parseNLTask() {
      if (!nlTaskInput.value.trim()) return;
      nlTaskLoading.value = true;
      try {
        const data = await api('/api/v1/ai/task/create', 'POST', { input: nlTaskInput.value });
        Object.assign(editingTask, {
          name: data.name || '',
          type: data.type || 'shell',
          image: data.image || '',
          cron_expr: data.cron_expr || '',
          payload: data.payload || '',
          max_retry: data.max_retry || 0,
          retry_policy: data.retry_policy || 'fixed_interval'
        });
        nlTaskInput.value = '';
      } catch (e) {
        alert('AI 解析失败: ' + e.message);
      } finally {
        nlTaskLoading.value = false;
      }
    }

    async function saveTask() {
      try {
        const body = {
          name: editingTask.name,
          type: editingTask.type,
          cron_expr: editingTask.cron_expr,
          payload: editingTask.payload,
          image: editingTask.image,
          timeout_seconds: editingTask.timeout_seconds,
          max_retry: editingTask.max_retry,
          retry_policy: editingTask.retry_policy,
          route_strategy: editingTask.route_strategy
        };
        if (editingTask.id) {
          await api('/api/v1/tasks/' + editingTask.id, 'PUT', body);
        } else {
          await api('/api/v1/tasks', 'POST', body);
        }
        taskModal.value = null;
        await loadTasks();
      } catch (e) {
        alert(e.message);
      }
    }

    async function toggleTask(task) {
      const action = task.status === 'enabled' ? 'pause' : 'resume';
      await api('/api/v1/tasks/' + task.id + '/' + action, 'POST');
      await loadTasks();
    }

    async function triggerTask(task) {
      await api('/api/v1/tasks/' + task.id + '/trigger', 'POST');
      alert('Task triggered');
    }

    async function deleteTask(task) {
      if (!confirm('Delete task #' + task.id + ' ' + task.name + '?')) return;
      await api('/api/v1/tasks/' + task.id, 'DELETE');
      await Promise.all([loadTasks(), loadInstances()]);
    }

    function formatResult(data) {
      return JSON.stringify(data, null, 2);
    }

    function formatTime(value) {
      if (!value) return '--';
      const d = new Date(value);
      if (isNaN(d.getTime())) return value;
      return d.toLocaleString('zh-CN', { hour12: false });
    }

    function formatAIHint(raw) {
      if (!raw) return '';
      try {
        const a = typeof raw === 'string' ? JSON.parse(raw) : raw;
        return (a.summary || '') +
          '\nSeverity: ' + (a.severity || '--') +
          '\nRoot cause: ' + (a.root_cause || '--') +
          '\nFix: ' + (a.fix || '--') +
          '\nConfidence: ' + (typeof a.confidence === 'number' ? Math.round(a.confidence * 100) + '%' : '--');
      } catch { return raw; }
    }

    function formatPercent(value) {
      if (typeof value !== 'number' || Number.isNaN(value)) return '--';
      return Math.round(value * 100) + '%';
    }

    function taskStatusClass(status) {
      return status === 'enabled' ? 'badge-success' : 'badge-warning';
    }

    function taskStatusText(status) {
      return status === 'enabled' ? 'Enabled' : status || '--';
    }

    function workerStatusClass(status) {
      return status === 'online' ? 'badge-success' : 'badge-error';
    }

    function workerStatusText(status) {
      return status === 'online' ? 'Online' : status || '--';
    }

    function instanceStatusClass(status) {
      if (status === 'success') return 'badge-success';
      if (status === 'failed') return 'badge-error';
      if (status === 'running') return 'badge-info';
      return 'badge-neutral';
    }

    function instanceStatusText(status) {
      return status || '--';
    }

    function workerLoadLabel(worker) {
      return ratioLabel(worker.current_load || 0, worker.max_concurrency || 0);
    }

    async function runLogAnalysis() {
      aiLoading.log = true;
      aiLog.resultObj = null;
      try {
        const body = {
          log: aiLog.log,
          error_code: aiLog.error_code,
          task_type: aiLog.task_type,
          retry_count: aiLog.retry_count
        };
        if (aiLog.instanceId) body.instance_id = aiLog.instanceId;
        const data = await api('/api/v1/ai/log-analysis/analyze', 'POST', body);
        aiLog.resultObj = data;
        aiLog.result = formatResult(data);
      } catch (e) {
        aiLog.resultObj = null;
        aiLog.result = 'Error: ' + e.message;
      } finally {
        aiLoading.log = false;
      }
    }

    function onSelectFailedInstance() {
      if (!aiLog.instanceId) return;
      const inst = aiLog.failedOptions.find(i => i.id === aiLog.instanceId);
      if (inst) {
        aiLog.log = inst.error_message || aiLog.log;
        aiLog.error_code = inst.error_code || '';
        aiLog.retry_count = inst.retry_count || 0;
      }
    }

    async function loadFailedInstances() {
      try {
        const data = await api('/api/v1/task-instances?status=failed&limit=20');
        aiLog.failedOptions = (data || []).map(normalizeInstance).slice(0, 20);
      } catch (e) {
        console.error('load failed instances:', e);
        aiLog.failedOptions = [];
      }
    }

    async function runAdvisor() {
      aiLoading.advisor = true;
      aiAdvisor.resultObj = null;
      try {
        const data = await api('/api/v1/ai/advisor/generate', 'POST', {
          avg_worker_load: aiAdvisor.avg_worker_load,
          total_workers: aiAdvisor.total_workers,
          online_workers: aiAdvisor.online_workers,
          pending_instances: aiAdvisor.pending_instances,
          failed_last_hour: aiAdvisor.failed_last_hour,
          avg_dispatch_latency_ms: aiAdvisor.avg_dispatch_latency_ms,
          max_pending_config: aiAdvisor.max_pending_config
        });
        aiAdvisor.resultObj = data;
        aiAdvisor.result = formatResult(data);
      } catch (e) {
        aiAdvisor.resultObj = null;
        aiAdvisor.result = 'Error: ' + e.message;
      } finally {
        aiLoading.advisor = false;
      }
    }

    async function runAutoAdvisor() {
      aiLoading.autoAdvisor = true;
      aiAutoAdvisor.resultObj = null;
      try {
        const data = await api('/api/v1/ai/advisor/auto', 'POST', {});
        aiAutoAdvisor.resultObj = data;
        aiAutoAdvisor.result = formatResult(data);
      } catch (e) {
        aiAutoAdvisor.resultObj = null;
        aiAutoAdvisor.result = 'Error: ' + e.message;
      } finally {
        aiLoading.autoAdvisor = false;
      }
    }

    async function runPredictDuration() {
      if (!aiPredict.taskId) return;
      aiLoading.predict = true;
      aiPredict.resultObj = null;
      try {
        const data = await api('/api/v1/ai/task/predict-duration', 'POST', { task_id: aiPredict.taskId });
        aiPredict.resultObj = data;
        aiPredict.result = formatResult(data);
      } catch (e) {
        aiPredict.resultObj = null;
        aiPredict.result = 'Error: ' + e.message;
      } finally {
        aiLoading.predict = false;
      }
    }

    async function runTrendAnalysis() {
      aiLoading.trend = true;
      aiTrend.resultObj = null;
      try {
        const data = await api('/api/v1/ai/trend/analyze', 'POST', {
          time_window_hours: aiTrend.timeWindowHours || 24
        });
        aiTrend.resultObj = data;
        aiTrend.result = formatResult(data);
      } catch (e) {
        aiTrend.resultObj = null;
        aiTrend.result = 'Error: ' + e.message;
      } finally {
        aiLoading.trend = false;
      }
    }

    // --- Chat functions ---

    function renderMarkdown(text) {
      if (!text) return '';
      // Escape HTML first, then convert basic markdown.
      let html = text
        .replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
        .replace(/\*\*(.+?)\*\*/g, '<strong>$1</strong>')
        .replace(/\*(.+?)\*/g, '<em>$1</em>')
        .replace(/`([^`]+)`/g, '<code>$1</code>')
        .replace(/\n/g, '<br>');
      // Convert markdown links: [text](url)
      html = html.replace(/\[([^\]]+)\]\(([^)]+)\)/g, '<a href="$2" target="_blank">$1</a>');
      return html;
    }

    async function loadConversations() {
      try {
        const data = await api('/api/v1/ai/conversations');
        conversations.value = (data && data.conversations) ? data.conversations : [];
      } catch (e) {
        console.error('load conversations:', e);
        conversations.value = [];
      }
    }

    async function selectConversation(convId) {
      currentConversationId.value = convId;
      chatMessages.value = [];
      chatError.value = '';
      try {
        const data = await api('/api/v1/ai/conversations/' + convId + '/messages');
        chatMessages.value = (data.messages || []).map(m => ({
          role: m.role, content: m.content, time: m.created_at
        }));
        scrollChatToBottom();
      } catch (e) {
        console.error('load messages:', e);
      }
    }

    function newConversation() {
      currentConversationId.value = '';
      chatMessages.value = [];
      chatStreamContent.value = '';
      chatToolCalls.value = [];
      chatError.value = '';
      chatInput.value = '';
    }

    function scrollChatToBottom() {
      setTimeout(() => {
        const el = chatMessagesEl.value;
        if (el) el.scrollTop = el.scrollHeight;
      }, 50);
    }

    async function sendChatMessage() {
      const msg = chatInput.value.trim();
      if (!msg || chatStreaming.value) return;
      chatInput.value = '';
      chatError.value = '';
      chatStreaming.value = true;
      chatStreamContent.value = '';
      chatToolCalls.value = [];

      chatMessages.value.push({ role: 'user', content: msg });
      scrollChatToBottom();

      const proto = location.protocol === 'https:' ? 'wss:' : 'ws:';
      const wsUrl = proto + '//' + location.host + '/api/v1/ai/chat/ws';

      let assistantContent = '';
      const toolCallNames = [];

      const ws = new WebSocket(wsUrl);
      let wsOpened = false;

      ws.onopen = () => {
        wsOpened = true;
        ws.send(JSON.stringify({
          message: msg,
          conversation_id: currentConversationId.value || ''
        }));
      };

      ws.onmessage = (e) => {
        try {
          const msg = JSON.parse(e.data);
          const event = msg.event;
          const payload = msg.data;
          if (!event) return;

          switch (event) {
            case 'text':
              assistantContent += payload.delta || '';
              chatStreamContent.value = assistantContent;
              scrollChatToBottom();
              break;
            case 'tool_call':
              chatToolCalls.value.push({
                name: payload.name,
                args: payload.args,
                done: false, result: null, resultStr: ''
              });
              scrollChatToBottom();
              break;
            case 'tool_result':
              const lastPending = chatToolCalls.value.filter(tc => !tc.done).pop();
              if (lastPending) {
                lastPending.done = true;
                lastPending.result = payload.result;
                lastPending.resultStr = JSON.stringify(payload.result, null, 2);
              }
              if (payload.name) toolCallNames.push(payload.name);
              scrollChatToBottom();
              break;
            case 'done':
              chatStreaming.value = false;
              chatStreamContent.value = '';
              chatToolCalls.value = [];
              ws.close();
              break;
            case 'conversation_id':
              if (payload.id && !currentConversationId.value) {
                currentConversationId.value = payload.id;
                loadConversations();
              }
              break;
            case 'error':
              chatError.value = payload.message || 'Unknown error';
              chatStreaming.value = false;
              ws.close();
              break;
          }
        } catch(ex) { /* skip malformed */ }
      };

      ws.onerror = () => {
        if (!wsOpened) {
          chatError.value = 'WebSocket connection failed, falling back to SSE';
          chatStreaming.value = false;
        }
      };

      ws.onclose = () => {
        chatStreaming.value = false;
        chatStreamContent.value = '';
        if (assistantContent || toolCallNames.length) {
          chatMessages.value.push({
            role: 'assistant',
            content: assistantContent,
            toolCalls: chatToolCalls.value.map(tc => ({
              name: tc.name,
              result: tc.result,
              resultStr: tc.result ? JSON.stringify(tc.result, null, 2) : ''
            }))
          });
        }
      };
      scrollChatToBottom();
    }
    function sendQuickMsg(msg) {
      chatInput.value = msg;
      sendChatMessage();
    }

    onMounted(() => {
      if (token.value) {
        Promise.all([loadTasks(), loadWorkers(), loadInstances(), loadAIStatus(), loadFailedInstances()]);
      }
    });

    return {
      token,
      role,
      roleLabel,
      loginUser,
      loginPass,
      loginError,
      tab,
      stats,
      tasks,
      workers,
      instances,
      taskModal,
      editingTask,
      aiLoading,
      aiStatus,

      aiLog,
      aiAdvisor,
      aiAutoAdvisor,
      aiPredict,
      aiTrend,
      failedInstances,
      runningInstances,
      successfulInstances,
      offlineWorkers,
      avgWorkerLoadLabel,
      highestWorkerLoadLabel,
      taskEnableRate,
      workerOnlineRate,
      instanceFailureRate,
      dashboardStats,
      doLogin,
      logout,
      loadTasks,
      loadWorkers,
      loadInstances,
      loadAIStatus,
      showTaskModal,
      nlTaskInput,
      nlTaskLoading,
      parseNLTask,
      saveTask,
      toggleTask,
      triggerTask,
      deleteTask,

      runLogAnalysis,
      onSelectFailedInstance,
      loadFailedInstances,
      runAdvisor,
      runAutoAdvisor,
      runPredictDuration,
      runTrendAnalysis,
      renderMarkdown,
      conversations,
      currentConversationId,
      chatMessages,
      chatInput,
      chatStreaming,
      chatStreamContent,
      chatToolCalls,
      chatError,
      chatMessagesEl,
      loadConversations,
      selectConversation,
      newConversation,
      sendChatMessage,
      sendQuickMsg,
      formatTime,
      formatAIHint,
      formatPercent,
      taskStatusClass,
      taskStatusText,
      workerStatusClass,
      workerStatusText,
      instanceStatusClass,
      instanceStatusText,
      workerLoadLabel
    };
  }
}).mount('#app');
