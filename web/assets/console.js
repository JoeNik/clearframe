/* 用户控制台：API 密钥 / 用量 / 调用记录 */
(function () {
  const { $, icon, esc, api, renderHeader, renderFooter, renderAds, copy, ms, toast, timeAgo } = window.UI;

  let BOOT = null;
  let ME = null;
  let BASE = '';
  let KEYS = [];

  function el(id) { return document.getElementById(id); }
  function show(node, on) { node.classList.toggle('hidden', !on); }

  // ---------- 登录 / 注册（双模式表单） ----------
  let MODE = 'login'; // login | register

  function setMode(m) {
    MODE = m;
    const reg = m === 'register';
    el('formTitle').textContent = reg ? '创建新账号' : '登录';
    show(el('confirmField'), reg);
    el('btnLogin').textContent = reg ? '注册并登录' : '登录';
    el('modeHint').innerHTML = reg
      ? '已有账号？<a href="#" id="toLogin" style="color:var(--accent)">返回登录</a>'
      : '还没有账号？<a href="#" id="toRegister" style="color:var(--accent)">立即注册</a>';
    el('lgMsg').textContent = '';
    const link = el(reg ? 'toLogin' : 'toRegister');
    if (link) link.addEventListener('click', function (e) { e.preventDefault(); setMode(reg ? 'login' : 'register'); });
  }

  function bindLogin() {
    const submit = async function () {
      const u = el('lgUser').value.trim();
      const p = el('lgPass').value;
      if (!u || !p) { el('lgMsg').textContent = '请填写用户名和密码'; return; }
      const btn = el('btnLogin');
      btn.disabled = true;
      try {
        if (MODE === 'register') {
          const p2 = el('lgPass2').value;
          if (p2 !== p) { el('lgMsg').textContent = '两次输入的密码不一致'; btn.disabled = false; return; }
          if (p.length < 8) { el('lgMsg').textContent = '密码至少 8 位'; btn.disabled = false; return; }
          btn.textContent = '注册中…';
          await api.post('/api/auth/register', { username: u, password: p });
          await api.post('/api/auth/login', { username: u, password: p });
        } else {
          btn.textContent = '登录中…';
          await api.post('/api/auth/login', { username: u, password: p });
        }
        location.reload();
      } catch (e) {
        el('lgMsg').textContent = e.message || '操作失败';
        btn.disabled = false;
        btn.textContent = MODE === 'register' ? '注册并登录' : '登录';
      }
    };
    el('btnLogin').addEventListener('click', submit);
    el('lgPass').addEventListener('keydown', function (e) { if (e.key === 'Enter') submit(); });
    el('lgPass2').addEventListener('keydown', function (e) { if (e.key === 'Enter') submit(); });
    el('toRegister').addEventListener('click', function (e) {
      e.preventDefault();
      setMode('register');
    });
  }

  // ---------- 密钥 ----------
  function keyRow(k) {
    const used = k.quota > 0 ? Math.min(100, Math.round(k.usedToday / k.quota * 100)) : 0;
    const over = k.quota > 0 && k.usedToday >= k.quota;
    return '<div class="key-row" data-id="' + k.id + '">' +
      '<div class="key-main">' +
        '<div class="key-name">' + esc(k.name || '未命名') +
          (k.status === 'disabled' ? '<span class="tag tag-off" style="margin-left:8px">已禁用</span>' : '') + '</div>' +
        '<div class="key-mask mono">' + esc(k.prefix) + '••••••••••••••••</div>' +
      '</div>' +
      '<div class="key-usage">' +
        '<div class="key-usage-txt">今日 <b>' + k.usedToday + '</b> / ' + (k.quota > 0 ? k.quota : '不限') + '</div>' +
        (k.quota > 0 ? '<div class="key-bar"><i style="width:' + used + '%;' + (over ? 'background:var(--red)' : '') + '"></i></div>' : '') +
      '</div>' +
      '<div class="key-ops">' +
        '<button class="btn btn-ghost btn-sm" data-act="toggle" data-id="' + k.id + '">' +
          (k.status === 'disabled' ? '启用' : '禁用') + '</button>' +
        '<button class="btn btn-ghost btn-sm" data-act="rename" data-id="' + k.id + '">改名</button>' +
        '<button class="btn btn-ghost btn-sm" data-act="del" data-id="' + k.id + '" style="color:var(--red)">删除</button>' +
      '</div>' +
    '</div>';
  }

  function renderKeys() {
    const box = el('keyList');
    if (!KEYS.length) {
      box.innerHTML = '<div class="empty" style="padding:28px 0;text-align:center">' +
        '<div style="color:var(--text-3);font-size:13px;margin-bottom:14px">还没有密钥，创建一个开始调用</div>' +
        '<button class="btn btn-primary btn-sm" id="btnNewKey2">新建密钥</button></div>';
      const b = el('btnNewKey2');
      if (b) b.addEventListener('click', newKey);
      return;
    }
    box.innerHTML = KEYS.map(keyRow).join('');
  }

  async function loadKeys() {
    const res = await api.get('/api/keys');
    KEYS = res.items || [];
    renderKeys();
  }

  async function newKey() {
    const name = window.prompt('给这个密钥起个名字（例如：生产环境）', '默认密钥');
    if (name === null) return;
    try {
      const res = await api.post('/api/keys', { name: name });
      showKeyModal(res.plain);
      await loadKeys();
    } catch (e) {
      toast(e.message || '创建失败', 'error');
    }
  }

  function showKeyModal(plain) {
    const root = el('modalRoot');
    root.innerHTML = '<div class="modal-mask"><div class="modal" style="max-width:520px">' +
      '<div class="modal-head"><h3>密钥已创建</h3></div>' +
      '<div class="modal-body">' +
        '<p style="font-size:13px;color:var(--text-2);line-height:1.7;margin:0 0 14px">' +
          '这是唯一一次显示完整密钥，关闭后无法再次查看，请立即复制保存。</p>' +
        '<div class="selbar" style="margin-bottom:0">' +
          '<code class="mono" id="plainKey" style="flex:1;word-break:break-all;font-size:12.5px;color:var(--accent)">' + esc(plain) + '</code>' +
          '<button class="btn btn-primary btn-sm" id="btnCopyKey">复制</button>' +
        '</div>' +
      '</div>' +
      '<div class="modal-foot"><button class="btn btn-ghost" id="btnCloseKey">我已保存</button></div>' +
    '</div></div>';
    el('btnCopyKey').addEventListener('click', function () { copy(plain); });
    el('btnCloseKey').addEventListener('click', function () { root.innerHTML = ''; });
  }

  function bindKeyActions() {
    el('keyList').addEventListener('click', async function (e) {
      const b = e.target.closest('button[data-act]');
      if (!b) return;
      const id = b.getAttribute('data-id');
      const act = b.getAttribute('data-act');
      const k = KEYS.find(function (x) { return String(x.id) === id; });
      if (!k) return;
      try {
        if (act === 'toggle') {
          await api.patch('/api/keys/' + id, { status: k.status === 'disabled' ? 'active' : 'disabled' });
          await loadKeys();
        } else if (act === 'rename') {
          const n = window.prompt('新的名称', k.name);
          if (n === null) return;
          await api.patch('/api/keys/' + id, { name: n });
          await loadKeys();
        } else if (act === 'del') {
          if (!window.confirm('删除后使用该密钥的调用会立即失效，确定删除？')) return;
          await api.del('/api/keys/' + id);
          await loadKeys();
        }
      } catch (err) {
        toast(err.message || '操作失败', 'error');
      }
    });
  }

  // ---------- 用量 ----------
  function renderUsage(data) {
    const s = data.summary || {};
    el('usageStats').innerHTML = [
      ['今日调用', (s.today || 0).toLocaleString('en-US')],
      ['累计调用', (s.total || 0).toLocaleString('en-US')],
      ['成功率', s.total ? Math.round((s.success / s.total) * 100) + '%' : '—'],
      ['平均耗时', ms(s.avgMs)]
    ].map(function (p) {
      return '<div class="stat"><div class="k">' + p[0] + '</div><div class="v">' + p[1] + '</div></div>';
    }).join('');

    // 近 7 天柱状图
    const days = [];
    for (let i = 6; i >= 0; i--) {
      const d = new Date(Date.now() - i * 86400000);
      const key = String(d.getMonth() + 1).padStart(2, '0') + '-' + String(d.getDate()).padStart(2, '0');
      days.push({ key: key, n: (s.byDay && s.byDay[key]) || 0 });
    }
    const max = Math.max(1, Math.max.apply(null, days.map(function (d) { return d.n; })));
    el('chart').innerHTML = days.map(function (d) {
      const h = Math.max(2, Math.round((d.n / max) * 82));
      return '<div style="flex:1;display:flex;flex-direction:column;justify-content:flex-end;align-items:center;gap:5px">' +
        '<span style="font-size:10px;color:var(--text-3)">' + (d.n || '') + '</span>' +
        '<i style="display:block;width:100%;max-width:38px;height:' + h + 'px;border-radius:3px;' +
        'background:' + (d.n ? 'var(--accent)' : 'var(--bg-4)') + ';opacity:' + (d.n ? 0.85 : 1) + '"></i>' +
      '</div>';
    }).join('');
    el('chartAxis').innerHTML = days.map(function (d) {
      return '<span style="flex:1;text-align:center">' + d.key + '</span>';
    }).join('');

    const list = data.items || [];
    el('usageHint').textContent = '最近 ' + list.length + ' 条';
    if (!list.length) {
      el('usageList').innerHTML = '<div class="empty" style="padding:34px 0;text-align:center;color:var(--text-3);font-size:13px">' +
        '还没有调用记录</div>';
      return;
    }
    el('usageList').innerHTML = '<div class="table-wrap" style="border:0;border-radius:0"><table class="tbl">' +
      '<thead><tr><th style="width:130px">时间</th><th>接口</th><th style="width:80px">状态</th>' +
      '<th style="width:80px">耗时</th><th style="width:120px">来源 IP</th></tr></thead><tbody>' +
      list.map(function (u) {
        const ok = u.status < 400;
        return '<tr>' +
          '<td class="mono" style="font-size:11.5px;color:var(--text-3)">' + timeAgo(u.ts) + '</td>' +
          '<td class="mono" style="font-size:12px">' + esc(u.method) + ' ' + esc(u.path) +
            (u.stream ? '<span class="tag" style="margin-left:6px">流式</span>' : '') + '</td>' +
          '<td><span class="tag ' + (ok ? 'tag-free' : 'tag-off') + '">' + u.status + '</span></td>' +
          '<td class="mono" style="font-size:11.5px;color:var(--text-3)">' + ms(u.ms) + '</td>' +
          '<td class="mono" style="font-size:11.5px;color:var(--text-3)">' + esc(u.ip || '—') + '</td>' +
        '</tr>';
      }).join('') + '</tbody></table></div>';
  }

  // ---------- 账号信息 ----------
  function renderAccount(usage) {
    if (!ME) return;
    el('accName').textContent = ME.username || '—';
    const isAdmin = ME.role === 'admin';
    const roleEl = el('accRole');
    roleEl.textContent = isAdmin ? '管理员' : '普通用户';
    roleEl.className = 'tag ' + (isAdmin ? 'tag-accent' : 'tag-free');
    // 仍在使用默认密码：醒目提示并展开修改密码表单
    if (ME.mustChangePass) {
      const accCard = el('accName').closest('.panel') || el('accName').closest('section') || el('accName').parentElement;
      if (accCard && !document.getElementById('accWarn')) {
        const warn = document.createElement('div');
        warn.id = 'accWarn';
        warn.style.cssText = 'margin:10px 0 2px;padding:9px 12px;border:1px solid rgba(161,98,7,.45);background:rgba(161,98,7,.08);border-radius:8px;font-size:12.5px;color:var(--amber)';
        warn.textContent = '⚠ 当前仍在使用默认密码，请立即修改';
        accCard.prepend(warn);
      }
      const det = document.getElementById('acc-pass');
      if (det) det.open = true;
    }
    if (ME.createdAt) {
      // 兼容 RFC3339 字符串与 unix 秒两种格式
      const t = ME.createdAt;
      const d = new Date(typeof t === 'number' ? t * 1000 : t);
      if (!isNaN(d.getTime())) {
        el('accCreated').textContent = d.getFullYear() + '-' +
          String(d.getMonth() + 1).padStart(2, '0') + '-' + String(d.getDate()).padStart(2, '0');
      }
    }
    const s = (usage && usage.summary) || {};
    el('accToday').textContent = String(s.today || 0) + ' 次';
    el('accTotal').textContent = String(s.total || 0) + ' 次';
  }

  function bindPassword() {
    el('btnChangePass').addEventListener('click', async function () {
      const o = el('pwOld').value, n = el('pwNew').value, n2 = el('pwNew2').value;
      const msg = el('pwMsg');
      if (!o || !n) { msg.textContent = '请填写原密码和新密码'; msg.style.color = 'var(--red)'; return; }
      if (n.length < 8) { msg.textContent = '新密码至少 8 位'; msg.style.color = 'var(--red)'; return; }
      if (n !== n2) { msg.textContent = '两次输入的新密码不一致'; msg.style.color = 'var(--red)'; return; }
      try {
        await api.post('/api/auth/password', { old: o, new: n });
        msg.textContent = '密码已更新';
        msg.style.color = 'var(--accent)';
        el('pwOld').value = ''; el('pwNew').value = ''; el('pwNew2').value = '';
        toast('密码修改成功', 'ok');
      } catch (e) {
        msg.textContent = e.message || '修改失败';
        msg.style.color = 'var(--red)';
      }
    });
  }

  async function loadUsage() {
    const data = await api.get('/api/usage?limit=50');
    renderUsage(data);
    renderAccount(data);
  }

  // ---------- 接入信息 ----------
  function renderBase() {
    el('baseUrl').value = BASE;
    const py = [
      'from openai import OpenAI',
      '',
      'client = OpenAI(',
      '    base_url="' + BASE + '/v1",',
      '    api_key="sk-你的密钥"',
      ')',
      '',
      'resp = client.chat.completions.create(',
      '    model="gpt-4o-mini",',
      '    messages=[{"role": "user", "content": "你好"}],',
      ')',
      'print(resp.choices[0].message.content)'
    ].join('\n');
    el('pyCode').textContent = py;
    el('copyBase').addEventListener('click', function () { copy(BASE); });
    el('copyPy').addEventListener('click', function () { copy(py); });
  }

  // ---------- 启动 ----------
  async function boot() {
    try {
      BOOT = await api.get('/api/public/bootstrap');
    } catch (e) {
      document.body.innerHTML = '<div style="padding:60px;text-align:center;color:#888">服务暂不可用</div>';
      return;
    }
    ME = BOOT.me;
    renderHeader(el('header'), BOOT.site, 'console', ME);
    renderFooter(el('footer'), BOOT.site);
    renderAds(BOOT.ads);
    BASE = BOOT.site.publicBase || location.origin;

    if (!ME) {
      show(el('loginBox'), true);
      // 后台关闭「开放注册」时，隐藏注册入口
      const regRow = el('regRow');
      if (regRow && BOOT.site && BOOT.site.registerOpen === false) {
        regRow.classList.add('hidden');
      }
      bindLogin();
      return;
    }

    show(el('main'), true);
    renderBase();
    bindPassword();
    el('btnNewKey').addEventListener('click', newKey);
    el('btnRefresh').addEventListener('click', function () { loadUsage().then(function () { toast('已刷新'); }); });
    bindKeyActions();

    try { await loadKeys(); } catch (e) { toast(e.message || '读取密钥失败', 'error'); }
    try { await loadUsage(); } catch (e) { /* 忽略 */ }
  }

  document.addEventListener('DOMContentLoaded', boot);
})();
