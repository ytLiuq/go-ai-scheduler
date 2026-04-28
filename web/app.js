const API = '';

const { createApp, ref, reactive, onMounted, watch } = Vue;

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
    const editingTask = reactive({ name: '', type: 'shell', cron_expr: '', payload: '', timeout_seconds: 300, max_retry: 3, retry_policy: 'fixed_interval', route_strategy: 'least_loaded' });

    const headers = () => token.value ? { 'Authorization': 'Bearer ' + token.value, 'Content-Type': 'application/json' } : { 'Content-Type': 'application/json' };

    async function api(path, method = 'GET', body = null) {
      const opts = { method, headers: headers() };
      if (body) opts.body = JSON.stringify(body);
      const resp = await fetch(API + path, opts);
      if (resp.status === 401) { logout(); throw new Error('unauthorized'); }
      if (!resp.ok) { const e = await resp.json().catch(() => ({})); throw new Error(e.error || resp.statusText); }
      return resp.json();
    }

    async function doLogin() {
      loginError.value = '';
      try {
        const resp = await api('/api/auth/login', 'POST', { username: loginUser.value, password: loginPass.value });
        token.value = resp.token;
        role.value = resp.role;
        localStorage.setItem('scheduler_token', resp.token);
        localStorage.setItem('scheduler_role', resp.role);
      } catch (e) { loginError.value = e.message; }
    }

    function logout() {
      token.value = '';
      role.value = '';
      localStorage.removeItem('scheduler_token');
      localStorage.removeItem('scheduler_role');
    }

    async function loadTasks() {
      try { const data = await api('/api/v1/tasks'); tasks.value = data || []; stats.tasks = tasks.value.length; stats.enabled = tasks.value.filter(t => t.status === 'enabled').length; } catch (e) { console.error(e); }
    }

    async function loadWorkers() {
      try { const data = await api('/api/v1/workers'); workers.value = data || []; stats.workers = workers.value.filter(w => w.status === 'online').length; } catch (e) { console.error(e); }
    }

    async function loadInstances() {
      try { const data = await api('/api/v1/task-instances'); instances.value = (data || []).slice(0, 50); stats.instances = instances.value.length; } catch (e) { console.error(e); }
    }

    function showTaskModal(task) {
      if (task) {
        Object.assign(editingTask, {
          id: task.id, name: task.name, type: task.type, cron_expr: task.cron_expr || '',
          payload: task.payload || '', timeout_seconds: task.timeout_seconds || 300,
          max_retry: task.max_retry || 3, retry_policy: task.retry_policy || 'fixed_interval',
          route_strategy: task.route_strategy || 'least_loaded'
        });
      } else {
        Object.assign(editingTask, { id: 0, name: '', type: 'shell', cron_expr: '', payload: '', timeout_seconds: 300, max_retry: 3, retry_policy: 'fixed_interval', route_strategy: 'least_loaded' });
      }
      taskModal.value = true;
    }

    async function saveTask() {
      try {
        const body = { name: editingTask.name, type: editingTask.type, cron_expr: editingTask.cron_expr, payload: editingTask.payload, timeout_seconds: editingTask.timeout_seconds, max_retry: editingTask.max_retry, retry_policy: editingTask.retry_policy, route_strategy: editingTask.route_strategy };
        if (editingTask.id) {
          await api('/api/v1/tasks/' + editingTask.id, 'PUT', body);
        } else {
          await api('/api/v1/tasks', 'POST', body);
        }
        taskModal.value = null;
        loadTasks();
      } catch (e) { alert(e.message); }
    }

    async function toggleTask(task) {
      const action = task.status === 'enabled' ? 'pause' : 'resume';
      await api('/api/v1/tasks/' + task.id + '/' + action, 'POST');
      loadTasks();
    }

    async function triggerTask(task) {
      await api('/api/v1/tasks/' + task.id + '/trigger', 'POST');
      alert('Task triggered');
    }

    onMounted(() => {
      if (token.value) {
        loadTasks();
        loadWorkers();
        loadInstances();
      }
    });

    return { token, role, loginUser, loginPass, loginError, tab, stats, tasks, workers, instances, taskModal, editingTask, doLogin, logout, loadTasks, loadWorkers, loadInstances, showTaskModal, saveTask, toggleTask, triggerTask };
  }
}).mount('#app');
