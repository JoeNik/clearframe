/* Admin console */
(function () {
  const { $, $$, icon, esc, api, toast, money, bytes, ms, timeAgo, copy } = window.UI;

  let ME = null;
  let CATS = [];
  let SETTINGS = null;
  let CUR_VIEW = 'overview';
  const IFACES = [];

  const TITLES = {
    overview: '概览', interfaces: '能力管理', categories: '分类管理',
    tts: '音色工坊', settings: '全局设置', ads: '广告位',
    users: '账号管理', logs: '访问日志', data: '导入导出'
  };

  // ============ 通用 UI ============
  function modal(title, body, footHTML) {
    const root = $('#modalRoot');
    root.innerHTML =
      '<div class="modal-mask" id="mask"><div class="modal">' +
      '<div class="modal-head"><h3>' + esc(title) + '</h3><span class="sp"></span>' +
      '<button class="iconbtn" id="mClose">' + icon('x', 14) + '</button></div>' +
      '<div class="modal-body">' + body + '</div>' +
      '<div class="modal-foot">' + (footHTML || '') + '</div>' +
      '</div></div>';
    $('#mClose').addEventListener('click', closeModal);
    $('#mask').addEventListener('mousedown', function (e) {
      if (e.target.id === 'mask') closeModal();
    });
    document.addEventListener('keydown', escClose);
    return root;
  }
  function escClose(e) { if (e.key === 'Escape') closeModal(); }
  function closeModal() {
    $('#modalRoot').innerHTML = '';
    document.removeEventListener('keydown', escClose);
  }
  window.__closeModal = closeModal;

  function field(label, inner, hint) {
    return '<div class="field"><label>' + label + '</label>' + inner +
      (hint ? '<div class="field-hint">' + hint + '</div>' : '') + '</div>';
  }
  function input(k, val, ph, type) {
    return '<input class="input" data-k="' + k + '" type="' + (type || 'text') + '" value="' +
      esc(val == null ? '' : val) + '" placeholder="' + esc(ph || '') + '">';
  }
  function num(k, val, ph) {
    return '<input class="input" data-k="' + k + '" type="number" step="any" value="' +
      (val == null ? '' : val) + '" placeholder="' + esc(ph || '') + '">';
  }
  function sw(k, on, label) {
    return '<label class="switchline" style="margin-bottom:14px"><input type="checkbox" data-k="' + k + '"' +
      (on ? ' checked' : '') + '> ' + esc(label) + '</label>';
  }
  function ta(k, val, ph, rows) {
    return '<textarea class="textarea" data-k="' + k + '" rows="' + (rows || 3) + '" placeholder="' +
      esc(ph || '') + '" style="font-family:var(--mono);font-size:12.5px">' + esc(val || '') + '</textarea>';
  }

  // 收集表单值为对象，支持 a.b 嵌套
  function collect(root) {
    const out = {};
    root.querySelectorAll('[data-k]').forEach(function (el) {
      const k = el.getAttribute('data-k');
      let v;
      if (el.type === 'checkbox') v = el.checked;
      else if (el.type === 'number') v = el.value === '' ? 0 : Number(el.value);
      else v = el.value;
      const parts = k.split('.');
      let cur = out;
      for (let i = 0; i < parts.length - 1; i++) {
        cur[parts[i]] = cur[parts[i]] || {};
        cur = cur[parts[i]];
      }
      cur[parts[parts.length - 1]] = v;
    });
    return out;
  }

  // ============ 登录 ============
  function showLogin() {
    $('#loginView').classList.remove('hidden');
    $('#adminView').classList.add('hidden');
    setTimeout(function () { $('#loginUser').focus(); }, 100);
  }

  function showAdmin() {
    $('#loginView').classList.add('hidden');
    $('#adminView').classList.remove('hidden');
    $('#whoami').innerHTML = '<i class="dot dot-live"></i>' + esc(ME.username);
    $$('#adminNav a').forEach(function (a) {
      const i = a.getAttribute('data-i');
      a.insertAdjacentHTML('afterbegin', icon(i, 15));
    });
    route();
  }

  async function doLogin(e) {
    e.preventDefault();
    const btn = $('#loginBtn');
    const err = $('#loginErr');
    err.textContent = '';
    btn.disabled = true;
    btn.innerHTML = '<span class="spinner"></span> 登录中';
    try {
      const u = await api.post('/api/auth/login', {
        username: $('#loginUser').value.trim(),
        password: $('#loginPass').value
      });
      if (u.role !== 'admin') {
        await api.post('/api/auth/logout', {});
        throw new Error('该账号没有管理权限');
      }
      ME = u;
      showAdmin();
      if (u.mustChangePass) showForcePass();
    } catch (ex) {
      err.textContent = ex.message;
      btn.disabled = false;
      btn.textContent = '登录';
    }
  }

  // ============ 首次登录强制改密 ============
  function showForcePass() {
    if ($('#forcePass')) return;
    const el = document.createElement('div');
    el.id = 'forcePass';
    el.style.cssText = 'position:fixed;inset:0;z-index:999;background:rgba(8,10,14,.75);display:grid;place-items:center;backdrop-filter:blur(4px)';
    el.innerHTML =
      '<div class="panel" style="width:min(430px,92vw);padding:26px">' +
      '<h3 style="margin:0 0 6px;font-size:16px">请先修改默认密码</h3>' +
      '<p style="margin:0 0 16px;font-size:12.5px;color:var(--text-3)">当前账号仍在使用默认密码。修改完成前，后台功能暂不可用。</p>' +
      '<div style="display:grid;gap:10px">' +
      '<input id="fpOld" type="password" class="input" placeholder="当前密码" autocomplete="current-password">' +
      '<input id="fpNew" type="password" class="input" placeholder="新密码（至少 8 位）" autocomplete="new-password">' +
      '<input id="fpNew2" type="password" class="input" placeholder="再次输入新密码" autocomplete="new-password">' +
      '<div id="fpErr" style="font-size:12px;color:var(--red);min-height:16px"></div>' +
      '<button id="fpSave" class="btn btn-primary">确认修改并继续</button>' +
      '</div></div>';
    document.body.appendChild(el);
    setTimeout(function () { $('#fpOld').focus(); }, 100);
    $('#fpSave').addEventListener('click', async function () {
      const o = $('#fpOld').value, n = $('#fpNew').value, n2 = $('#fpNew2').value;
      const errEl = $('#fpErr');
      if (!o || !n) { errEl.textContent = '请填写当前密码与新密码'; return; }
      if (n.length < 8) { errEl.textContent = '新密码至少 8 位'; return; }
      if (n !== n2) { errEl.textContent = '两次输入的新密码不一致'; return; }
      const btn = $('#fpSave');
      btn.disabled = true; btn.innerHTML = '<span class="spinner"></span> 提交中';
      try {
        await api.post('/api/auth/password', { old: o, new: n });
        toast('密码已修改，正在进入后台…', 'ok');
        setTimeout(function () { location.reload(); }, 700);
      } catch (ex) {
        errEl.textContent = ex.message || '修改失败';
        btn.disabled = false; btn.textContent = '确认修改并继续';
      }
    });
  }

  // ============ 路由 ============
  function route() {
    let v = location.hash.replace(/^#\/?/, '') || 'overview';
    if (!TITLES[v]) v = 'overview';
    CUR_VIEW = v;
    $('#pageTitle').textContent = TITLES[v];
    $$('#adminNav a').forEach(function (a) {
      a.classList.toggle('on', a.getAttribute('data-v') === v);
    });
    $('#adminContent').innerHTML = '<div class="skeleton" style="height:120px"></div>';
    VIEWS[v]();
  }

  // ============ 概览 ============
  async function viewOverview() {
    let d;
    try { d = await api.get('/api/admin/overview'); }
    catch (e) { $('#adminContent').innerHTML = '<div class="empty">' + esc(e.message) + '</div>'; return; }
    const s = d.stats || {};
    $('#adminContent').innerHTML =
      '<div class="stat-grid">' +
      stat('接口总数', s.total, s.online + ' 个在线', false) +
      stat('免费接口', s.free, s.paid + ' 个收费', true) +
      stat('今日调用', (s.today || 0).toLocaleString('en-US'), '累计 ' + (s.calls || 0).toLocaleString('en-US'), false) +
      stat('账号数', d.users, '含管理员', false) +
      '</div>' +
      '<div style="display:grid;grid-template-columns:1.6fr 1fr;gap:14px;align-items:start" class="ov-grid">' +
      '<div class="panel"><div class="panel-head"><h3>最近请求</h3><span class="sp"></span>' +
      '<a class="btn btn-quiet btn-sm" href="#/logs">全部</a></div>' +
      '<div class="table-wrap" style="border:0;border-radius:0">' +
      '<table class="tbl"><thead><tr><th>时间</th><th>方法</th><th>路径</th><th>状态</th><th>耗时</th></tr></thead><tbody>' +
      (d.recentLogs || []).map(function (l) {
        return '<tr><td class="mono" style="font-size:11.5px;color:var(--text-3)">' +
          new Date(l.ts).toLocaleTimeString('zh-CN') + '</td>' +
          '<td class="mono" style="font-size:11.5px">' + esc(l.method) + '</td>' +
          '<td class="mono" style="font-size:11.5px;color:var(--text-2)">' + esc(l.path) + '</td>' +
          '<td><span style="font-family:var(--mono);font-size:11.5px;color:' +
          (l.status < 400 ? 'var(--green)' : 'var(--red)') + '">' + l.status + '</span></td>' +
          '<td class="mono" style="font-size:11.5px;color:var(--text-3)">' + l.ms + 'ms</td></tr>';
      }).join('') + '</tbody></table></div></div>' +
      '<div style="display:flex;flex-direction:column;gap:14px">' +
      '<div class="panel"><div class="panel-head"><h3>音色工坊</h3></div><div class="panel-body">' +
      '<div class="kv"><span>音色样本</span><b>' + d.voices + ' 个 · ' + bytes(d.voiceBytes) + '</b></div>' +
      '<div class="kv"><span>合成记录</span><b>' + d.renders + ' 条 · ' + bytes(d.renderBytes) + '</b></div>' +
      '<div class="kv"><span>服务密钥</span><b>' +
      (SETTINGS && SETTINGS.tts.apiKey ? '<span style="color:var(--green)">已配置</span>' : '<span style="color:var(--red)">未配置</span>') +
      '</b></div>' +
      '<a class="btn btn-ghost btn-sm btn-block" href="#/tts" style="margin-top:14px">前往配置</a>' +
      '</div></div>' +
      '<div class="panel"><div class="panel-head"><h3>快捷操作</h3></div><div class="panel-body" ' +
      'style="display:flex;flex-direction:column;gap:8px">' +
      '<button class="btn btn-ghost btn-sm" id="qaAdd">新增能力</button>' +
      '<a class="btn btn-ghost btn-sm" href="#/settings">修改站点信息</a>' +
      '<a class="btn btn-ghost btn-sm" href="#/ads">管理广告位</a>' +
      '</div></div>' +
      '</div></div>';

    $('#qaAdd').addEventListener('click', function () { openIfaceEditor(null); });
  }

  function stat(k, v, s, accent) {
    return '<div class="stat"><div class="k">' + esc(k) + '</div>' +
      '<div class="v' + (accent ? ' accent' : '') + '">' + (v == null ? '—' : v) + '</div>' +
      '<div class="s">' + esc(s || '') + '</div></div>';
  }

  // ============ 接口管理 ============
  async function viewInterfaces() {
    let res;
    try { res = await api.get('/api/admin/interfaces?limit=500'); }
    catch (e) { $('#adminContent').innerHTML = '<div class="empty">' + esc(e.message) + '</div>'; return; }
    IFACES.length = 0;
    (res.items || []).forEach(function (x) { IFACES.push(x); });

    $('#adminContent').innerHTML =
      '<div class="toolbar">' +
      '<div class="searchbox"><svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><circle cx="11" cy="11" r="7"/><path d="m20 20-3.2-3.2"/></svg>' +
      '<input id="admSearch" placeholder="搜索名称 / 路径 / 标签"></div>' +
      '<select class="select" id="admStatus" style="width:130px"><option value="">全部状态</option>' +
      '<option value="online">已上线</option><option value="offline">已下线</option>' +
      '<option value="maintenance">维护中</option></select>' +
      '<select class="select" id="admCat" style="width:150px"><option value="">全部分类</option>' +
      CATS.map(function (c) { return '<option value="' + esc(c.slug) + '">' + esc(c.name) + '</option>'; }).join('') +
      '</select>' +
      '<span style="margin-left:auto;font-family:var(--mono);font-size:12px;color:var(--text-3)" id="admCount"></span>' +
      '<button class="btn btn-ghost btn-sm" id="admSync" title="从上游接口市场拉取最新清单">' +
        icon('download', 14) + ' 从上游同步</button>' +
      '<button class="btn btn-primary btn-sm" id="admAdd">' + icon('plus', 14) + ' 新增能力</button>' +
      '</div>' +
      '<div class="table-wrap"><table class="tbl" id="admTable"><thead><tr>' +
      '<th>接口</th><th style="width:100px">分类</th><th style="width:180px">路径</th>' +
      '<th style="width:168px">计费</th><th style="width:72px">需登录</th>' +
      '<th style="width:76px">上线</th><th style="width:92px"></th>' +
      '</tr></thead><tbody id="admBody"></tbody></table></div>';

    renderIfaceRows();
    $('#admSearch').addEventListener('input', renderIfaceRows);
    $('#admStatus').addEventListener('change', renderIfaceRows);
    $('#admCat').addEventListener('change', renderIfaceRows);
    const syncBtn = $('#admSync');
    if (syncBtn) {
      syncBtn.addEventListener('click', async function () {
        if (!window.confirm('将从上游接口市场拉取接口清单，同名接口的内容与上下线状态会被覆盖；上游已收费的能力会自动开启本地计费。继续？')) return;
        syncBtn.disabled = true;
        syncBtn.innerHTML = '<span class="spinner"></span> 同步中';
        try {
          const r = await api.post('/api/admin/sync', {});
          const warns = (r.warnings || []).filter(Boolean);
          if (warns.length) {
            // 上游转收费等联动提醒需要管理员看到，直接展示内容
            toast('已同步 ' + r.synced + ' 个接口 · ' + warns.slice(0, 3).join('；') + (warns.length > 3 ? ' 等 ' + warns.length + ' 条' : ''), 'warn', 6000);
          } else {
            toast('已同步 ' + r.synced + ' 个接口', 'ok');
          }
          viewInterfaces();
        } catch (e) {
          toast(e.message || '同步失败', 'error');
          syncBtn.disabled = false;
          syncBtn.innerHTML = '从上游同步';
        }
      });
    }

    $('#admAdd').addEventListener('click', function () { openIfaceEditor(null); });
    bindIfaceTable();
  }

  function filteredIfaces() {
    const q = ($('#admSearch') ? $('#admSearch').value : '').trim().toLowerCase();
    const st = $('#admStatus') ? $('#admStatus').value : '';
    const cat = $('#admCat') ? $('#admCat').value : '';
    return IFACES.filter(function (x) {
      if (st && x.status !== st) return false;
      if (cat && x.category !== cat) return false;
      if (q) {
        const hay = (x.name + ' ' + x.slug + ' ' + x.path + ' ' + (x.tags || []).join(' ')).toLowerCase();
        if (hay.indexOf(q) < 0) return false;
      }
      return true;
    });
  }

  function renderIfaceRows() {
    const list = filteredIfaces();
    $('#admCount').textContent = list.length + ' / ' + IFACES.length;
    const body = $('#admBody');
    if (!list.length) {
      body.innerHTML = '<tr><td colspan="7"><div class="empty" style="border:0;padding:40px">没有匹配的接口</div></td></tr>';
      return;
    }
    body.innerHTML = list.map(function (it) {
      const paid = it.pricingMode === 'paid';
      // 上游计费状态快照（由「从上游同步」写入，仅供对照，不影响本地计费）
      const upTxt = it.upstreamPricing === 'paid'
        ? '收费 ¥' + money(it.upstreamPrice) + '/' + esc(it.upstreamPriceUnit || '次')
        : it.upstreamPricing === 'free' ? '免费' : '未同步';
      const upColor = it.upstreamPricing === 'paid' ? 'var(--amber)'
        : it.upstreamPricing === 'free' ? 'var(--green)' : 'var(--text-3)';
      return '<tr data-id="' + it.id + '">' +
        '<td><div style="font-size:13.5px;color:var(--text);font-weight:500">' + esc(it.name) + '</div>' +
        '<div class="mono" style="font-size:11px;color:var(--text-3)">' + esc(it.slug) + '</div></td>' +
        '<td style="font-size:12.5px">' + esc(catName(it.category)) + '</td>' +
        '<td><div class="mono" style="font-size:11px;color:var(--text-2)"><span style="color:var(--accent)">' +
        esc(it.method) + '</span> ' + esc(it.path) + '</div></td>' +
        '<td><div style="display:flex;flex-direction:column;gap:3px;align-items:flex-start">' +
        '<div style="display:flex;align-items:center;gap:7px">' +
        '<label class="switch" title="切换免费 / 收费"><input type="checkbox" data-t="paid"' + (paid ? ' checked' : '') + '><i></i></label>' +
        '<span class="mono" data-price-span style="font-size:10.5px;white-space:nowrap;color:' + (paid ? 'var(--amber)' : 'var(--green)') + '">' +
        (paid ? '¥' + money(it.price) + '/' + esc(it.priceUnit) : '免费') + '</span></div>' +
        '<span class="mono" style="font-size:10px;white-space:nowrap;color:' + upColor + '" title="上游计费状态，由「从上游同步」更新，不影响本地设置">上游: ' + upTxt + '</span>' +
        '</div></td>' +
        '<td><label class="switch"><input type="checkbox" data-t="login"' + (it.requiresLogin ? ' checked' : '') + '><i></i></label></td>' +
        '<td><label class="switch"><input type="checkbox" data-t="online"' + (it.status === 'online' ? ' checked' : '') + '><i></i></label></td>' +
        '<td><div class="tbl-actions">' +
        '<button class="iconbtn" data-act="edit" title="编辑">' + icon('edit', 12) + '</button>' +
        '<button class="iconbtn" data-act="view" title="查看文档">' + icon('link', 12) + '</button>' +
        '<button class="iconbtn danger" data-act="del" title="删除">' + icon('trash', 12) + '</button>' +
        '</div></td></tr>';
    }).join('');
  }

  function bindIfaceTable() {
    const body = $('#admBody');
    body.addEventListener('change', async function (e) {
      const cb = e.target.closest('input[data-t]');
      if (!cb) return;
      const tr = cb.closest('tr');
      const id = Number(tr.getAttribute('data-id'));
      const kind = cb.getAttribute('data-t');
      const patch = {};
      if (kind === 'online') patch.status = cb.checked ? 'online' : 'offline';
      else if (kind === 'paid') patch.pricingMode = cb.checked ? 'paid' : 'free';
      else if (kind === 'login') patch.requiresLogin = cb.checked;

      cb.disabled = true;
      try {
        const res = await api.patch('/api/admin/interfaces/' + id, patch);
        const idx = IFACES.findIndex(function (x) { return x.id === id; });
        if (idx >= 0) IFACES[idx] = res.interface;
        const it = res.interface;
        if (kind === 'paid') {
          const span = tr.querySelector('[data-price-span]');
          span.textContent = it.pricingMode === 'paid' ? '¥' + money(it.price) + '/' + it.priceUnit : '免费';
          span.style.color = it.pricingMode === 'paid' ? 'var(--amber)' : 'var(--green)';
        }
        toast('已更新', 'ok');
      } catch (ex) {
        cb.checked = !cb.checked;
        toast(ex.message, 'err');
      }
      cb.disabled = false;
    });

    body.addEventListener('click', async function (e) {
      const b = e.target.closest('[data-act]');
      if (!b) return;
      const tr = b.closest('tr');
      const id = Number(tr.getAttribute('data-id'));
      const it = IFACES.find(function (x) { return x.id === id; });
      const act = b.getAttribute('data-act');
      if (act === 'edit') openIfaceEditor(it);
      else if (act === 'view') window.open('/interface/' + encodeURIComponent(it.slug), '_blank');
      else if (act === 'del') {
        if (!confirm('确定删除接口「' + it.name + '」？此操作不可恢复。')) return;
        try {
          await api.del('/api/admin/interfaces/' + id);
          const idx = IFACES.findIndex(function (x) { return x.id === id; });
          IFACES.splice(idx, 1);
          renderIfaceRows();
          toast('已删除', 'ok');
        } catch (ex) { toast(ex.message, 'err'); }
      }
    });
  }

  function paramRowHTML(p) {
    const types = ['string', 'integer', 'number', 'boolean', 'array', 'object'];
    return '<div class="param-row" data-row>' +
      '<input data-f="name" placeholder="参数名" value="' + esc(p.name || '') + '">' +
      '<select data-f="type">' + types.map(function (t) {
        return '<option value="' + t + '"' + ((p.type || 'string') === t ? ' selected' : '') + '>' + t + '</option>';
      }).join('') + '</select>' +
      '<label style="display:flex;align-items:center;justify-content:center;height:32px">' +
      '<input type="checkbox" data-f="required"' + (p.required ? ' checked' : '') + ' title="必填"></label>' +
      '<input data-f="default" placeholder="默认值" value="' + esc(p.default || '') + '">' +
      '<input data-f="desc" placeholder="说明" value="' + esc(p.desc || '') + '">' +
      '<button class="iconbtn danger" data-delrow>' + icon('x', 11) + '</button>' +
      '</div>';
  }

  function openIfaceEditor(it) {
    const isNew = !it;
    const d = it || {
      name: '', slug: '', category: CATS.length ? CATS[0].slug : 'chat', method: 'POST',
      path: '/v1/', summary: '', description: '', params: [], bodyExample: '', responseExample: '',
      pricingMode: 'free', price: 0, priceUnit: '次', freeQuota: 0, requiresLogin: false,
      status: 'online', latencyMs: 0, popularity: 0, tags: [], sort: 0
    };

    const body =
      '<div class="form-grid">' +
      '<div class="full">' + field('接口名称 <span class="req">*</span>', input('name', d.name, '例如：GPT-4o mini 对话')) + '</div>' +
      field('接口标识 slug <span class="req">*</span>', input('slug', d.slug, 'chat-gpt-4o-mini'),
        '用于 URL，只能包含字母、数字与短横线') +
      field('分类', '<select class="select" data-k="category">' + CATS.map(function (c) {
        return '<option value="' + esc(c.slug) + '"' + (d.category === c.slug ? ' selected' : '') + '>' + esc(c.name) + '</option>';
      }).join('') + '</select>') +
      field('请求方法', '<select class="select" data-k="method">' +
        ['POST', 'GET', 'PUT', 'PATCH', 'DELETE'].map(function (m) {
          return '<option' + (d.method === m ? ' selected' : '') + '>' + m + '</option>';
        }).join('') + '</select>') +
      '<div class="full">' + field('对外路径 <span class="req">*</span>', input('path', d.path, '/v1/chat/completions'),
        '客户端调用的地址，必须以 /v1/ 开头，网关按此路径匹配') + '</div>' +
      field('上游路径', input('upstreamPath', d.upstreamPath, '留空自动推导'),
        '留空 = 上游地址 + 上游前缀 + 去掉 /v1 的路径') +
      field('上游方法', input('upstreamMethod', d.upstreamMethod, '留空同对外方法')) +
      '<div class="full">' + field('一句话简介', input('summary', d.summary, '展示在卡片与列表上，建议 20~40 字')) + '</div>' +
      '<div class="full">' + field('详细说明', ta('description', d.description, '支持空行分段，会渲染为多个段落', 4)) + '</div>' +
      '</div>' +
      '<div style="border-top:1px solid var(--line);margin:6px 0 18px"></div>' +
      '<div class="form-grid">' +
      field('计费模式', '<select class="select" data-k="pricingMode">' +
        '<option value="free"' + (d.pricingMode === 'free' ? ' selected' : '') + '>免费</option>' +
        '<option value="paid"' + (d.pricingMode === 'paid' ? ' selected' : '') + '>收费</option></select>') +
      field('单价', num('price', d.price, '0.0015')) +
      field('计价单位', input('priceUnit', d.priceUnit, '千 tokens / 次 / 张 / 条')) +
      (d.upstreamPricing
        ? '<div class="full">' + field('上游计费状态（只读，随「从上游同步」更新）',
          '<div style="font-size:12.5px;color:var(--text-2);padding:7px 0">上游' +
          (d.upstreamPricing === 'paid'
            ? '收费 ¥' + money(d.upstreamPrice) + ' / ' + esc(d.upstreamPriceUnit || '次')
            : '免费') + '　<span class="mono" style="font-size:11px;color:var(--text-3)">本站计费设置独立于此，同步不会覆盖</span></div>') + '</div>'
        : '') +
      field('免费额度', num('freeQuota', d.freeQuota, '0 表示无免费额度')) +
      field('平均延迟 (ms)', num('latencyMs', d.latencyMs, '620')) +
      field('排序权重', num('sort', d.sort, '数字越小越靠前')) +
      '<div class="full">' + field('标签（英文逗号分隔）', input('tagsRaw', (d.tags || []).join(','), 'OpenAI 兼容, 流式')) + '</div>' +
      '</div>' +
      '<div style="display:flex;gap:28px;flex-wrap:wrap;margin-bottom:8px">' +
      '<label class="switchline"><input type="checkbox" data-k="requiresLogin"' + (d.requiresLogin ? ' checked' : '') + '> 需要登录才能调用</label>' +
      '<label class="switchline">状态：<select class="select" data-k="status" style="width:110px;height:30px">' +
      '<option value="online"' + (d.status === 'online' ? ' selected' : '') + '>已上线</option>' +
      '<option value="offline"' + (d.status === 'offline' ? ' selected' : '') + '>已下线</option>' +
      '<option value="maintenance"' + (d.status === 'maintenance' ? ' selected' : '') + '>维护中</option></select></label>' +
      '</div>' +
      '<div style="border-top:1px solid var(--line);margin:18px 0"></div>' +
      '<div style="display:flex;align-items:center;gap:10px;margin-bottom:10px">' +
      '<h4 style="font-size:13px">请求参数</h4><span class="sp" style="flex:1"></span>' +
      '<button class="btn btn-ghost btn-xs" id="addParam">' + icon('plus', 12) + ' 添加参数</button></div>' +
      '<div id="paramList">' + (d.params || []).map(paramRowHTML).join('') + '</div>' +
      '<div style="border-top:1px solid var(--line);margin:18px 0"></div>' +
      field('请求体示例 (JSON)', ta('bodyExample', d.bodyExample, '{\n  "model": "gpt-4o-mini"\n}', 6)) +
      field('响应示例', ta('responseExample', d.responseExample, '{\n  "id": "chatcmpl-..."\n}', 6));

    modal(isNew ? '新增能力' : '编辑接口', body,
      '<button class="btn btn-ghost" onclick="__closeModal()">取消</button>' +
      '<button class="btn btn-primary" id="mSave">保存</button>');

    $('#addParam').addEventListener('click', function () {
      $('#paramList').insertAdjacentHTML('beforeend', paramRowHTML({}));
    });
    $('#paramList').addEventListener('click', function (e) {
      const b = e.target.closest('[data-delrow]');
      if (b) b.closest('.param-row').remove();
    });

    $('#mSave').addEventListener('click', async function () {
      const root = $('#modalRoot');
      const v = collect(root);
      if (!v.name || !v.slug || !v.path) { toast('名称、标识与路径不能为空', 'err'); return; }

      const params = [];
      root.querySelectorAll('#paramList .param-row').forEach(function (row) {
        const name = row.querySelector('[data-f="name"]').value.trim();
        if (!name) return;
        params.push({
          name: name,
          type: row.querySelector('[data-f="type"]').value,
          required: row.querySelector('[data-f="required"]').checked,
          default: row.querySelector('[data-f="default"]').value,
          desc: row.querySelector('[data-f="desc"]').value
        });
      });

      const payload = {
        name: v.name, slug: v.slug.trim(), category: v.category, method: v.method,
        path: v.path, summary: v.summary, description: v.description,
        params: params, bodyExample: v.bodyExample, responseExample: v.responseExample,
        pricingMode: v.pricingMode, price: v.price, priceUnit: v.priceUnit,
        freeQuota: v.freeQuota, requiresLogin: v.requiresLogin, status: v.status,
        latencyMs: v.latencyMs, sort: v.sort,
        tags: (v.tagsRaw || '').split(',').map(function (s) { return s.trim(); }).filter(Boolean)
      };

      const btn = $('#mSave');
      btn.disabled = true;
      btn.innerHTML = '<span class="spinner"></span> 保存中';
      try {
        if (isNew) await api.post('/api/admin/interfaces', payload);
        else await api.put('/api/admin/interfaces/' + it.id, payload);
        closeModal();
        toast('已保存', 'ok');
        viewInterfaces();
      } catch (e) {
        toast(e.message, 'err');
        btn.disabled = false;
        btn.textContent = '保存';
      }
    });
  }

  function catName(slug) {
    const c = CATS.find(function (x) { return x.slug === slug; });
    return c ? c.name : (slug || '未分类');
  }

  // ============ 分类 ============
  async function viewCategories() {
    let res;
    try { res = await api.get('/api/admin/categories'); }
    catch (e) { $('#adminContent').innerHTML = '<div class="empty">' + esc(e.message) + '</div>'; return; }
    const counts = res.counts || {};
    $('#adminContent').innerHTML =
      '<div style="display:grid;grid-template-columns:1fr 320px;gap:14px;align-items:start" class="cat-grid">' +
      '<div class="panel"><div class="panel-head"><h3>分类列表</h3><span class="sp"></span>' +
      '<span class="hint">删除分类不会删除其下接口</span></div>' +
      '<div class="table-wrap" style="border:0"><table class="tbl"><thead><tr>' +
      '<th>名称</th><th style="width:120px">标识</th><th style="width:90px">接口数</th>' +
      '<th style="width:80px">排序</th><th style="width:60px"></th></tr></thead><tbody>' +
      (res.items || []).map(function (c) {
        return '<tr data-slug="' + esc(c.slug) + '"><td style="color:var(--text)">' + esc(c.name) + '</td>' +
          '<td class="mono" style="font-size:11.5px;color:var(--text-3)">' + esc(c.slug) + '</td>' +
          '<td class="mono">' + (counts[c.slug] || 0) + '</td>' +
          '<td class="mono" style="color:var(--text-3)">' + c.sort + '</td>' +
          '<td><button class="iconbtn danger" data-delcat="' + esc(c.slug) + '">' + icon('trash', 11) + '</button></td></tr>';
      }).join('') + '</tbody></table></div></div>' +
      '<div class="panel"><div class="panel-head"><h3>新增 / 更新</h3></div><div class="panel-body">' +
      field('名称', '<input class="input" id="cName" placeholder="例如：对话模型">') +
      field('标识 slug', '<input class="input" id="cSlug" placeholder="chat">', '英文小写，用于接口归类') +
      field('图标', '<select class="select" id="cIcon">' +
      ['chat', 'image', 'video', 'audio', 'eye', 'tool', 'vector', 'grid', 'folder'].map(function (i) {
        return '<option value="' + i + '">' + i + '</option>';
      }).join('') + '</select>') +
      field('排序', '<input class="input" id="cSort" type="number" value="100">') +
      '<button class="btn btn-primary btn-block" id="cSave">保存分类</button>' +
      '</div></div></div>';

    $('#cSave').addEventListener('click', async function () {
      const name = $('#cName').value.trim();
      const slug = $('#cSlug').value.trim();
      if (!name || !slug) { toast('名称与标识不能为空', 'err'); return; }
      try {
        await api.post('/api/admin/categories', {
          slug: slug, name: name, icon: $('#cIcon').value, sort: Number($('#cSort').value) || 100
        });
        toast('已保存', 'ok');
        await loadCats();
        viewCategories();
      } catch (e) { toast(e.message, 'err'); }
    });

    $('#adminContent').addEventListener('click', async function (e) {
      const b = e.target.closest('[data-delcat]');
      if (!b) return;
      const slug = b.getAttribute('data-delcat');
      if (!confirm('确定删除分类「' + slug + '」？')) return;
      try {
        await api.del('/api/admin/categories/' + encodeURIComponent(slug));
        toast('已删除', 'ok');
        await loadCats();
        viewCategories();
      } catch (ex) { toast(ex.message, 'err'); }
    });
  }

  // ============ 设置 ============
  async function viewSettings() {
    let res;
    try { res = await api.get('/api/admin/settings'); }
    catch (e) { $('#adminContent').innerHTML = '<div class="empty">' + esc(e.message) + '</div>'; return; }
    const s = res.settings;
    SETTINGS = s;

    $('#adminContent').innerHTML =
      '<div style="display:flex;flex-direction:column;gap:14px;max-width:860px">' +

      '<div class="panel"><div class="panel-head"><h3>站点信息</h3></div><div class="panel-body">' +
      '<div class="form-grid">' +
      field('站点名称', input('siteName', s.siteName)) +
      field('Logo 文字', input('logoText', s.logoText), '1~2 个字符最佳') +
      '<div class="full">' + field('首页标语', input('siteTagline', s.siteTagline)) + '</div>' +
      '<div class="full">' + field('站点简介', ta('siteDesc', s.siteDesc, '', 3)) + '</div>' +
      field('页脚文案', input('footerText', s.footerText)) +
      field('备案号', input('icp', s.icp, '可选')) +
      '<div class="full">' + field('公告栏文案', input('announcement', s.announcement)) + '</div>' +
      '</div>' +
      sw('showAnnouncement', s.showAnnouncement, '显示顶部公告栏') +
      sw('showStats', s.showStats, '在首页展示累计调用量') +
      '</div></div>' +

      '<div class="panel"><div class="panel-head"><h3>访问控制</h3></div><div class="panel-body">' +
      '<div style="display:flex;flex-direction:column;gap:2px">' +
      sw('allowGuest', s.allowGuest, '允许未登录用户浏览接口目录') +
      sw('allowPlayground', s.allowPlayground, '开放接口详情页的在线调试') +
      sw('requireLoginAll', s.requireLoginAll, '全站接口都要求登录（覆盖单个接口设置）') +
      sw('registerOpen', s.registerOpen, '开放自助注册') +
      sw('maintenance', s.maintenance, '维护模式（仅管理员可访问后台）') +
      '</div>' +
      field('维护提示文案', input('maintenanceMsg', s.maintenanceMsg)) +
      '</div></div>' +

      '<div class="panel"><div class="panel-head"><h3>上游接入</h3><span class="sp"></span>' +
      '<button class="btn btn-ghost btn-sm" id="btnProbe">测试连通性</button></div><div class="panel-body">' +
      '<div class="form-grid">' +
      '<div class="full">' + field('上游地址', input('upstreamBase', s.upstreamBase, 'https://upstream.example.com'),
        '只填域名根，例如 https://upstream.example.com；路径由下面的前缀与接口配置决定') + '</div>' +
      '<div class="full">' + field('上游 API Key', input('upstreamKey', s.upstreamKey, 'sk-...'),
        '★ 核心密钥：所有转发请求都会带上它，只保存在服务端，任何前台接口都不会返回') + '</div>' +
      field('鉴权头名称', input('upstreamAuthHeader', s.upstreamAuthHeader, 'x-api-key')) +
      field('鉴权值前缀', input('upstreamAuthPrefix', s.upstreamAuthPrefix, '留空即直接传 Key')) +
      '<div class="full">' + field('上游路径前缀', input('upstreamPrefix', s.upstreamPrefix, '/api'),
        '默认把 /v1/chat/completions 转发到 {上游地址}/api/chat/completions；单个接口可在接口管理里覆盖') + '</div>' +
      '<div class="full">' + field('本站对外地址', input('publicBase', s.publicBase, 'https://api.你的域名.com'),
        '用于生成文档里的 base_url 与示例代码，留空自动取当前域名') + '</div>' +
      '</div>' +
      sw('gatewayEnabled', s.gatewayEnabled, '启用 /v1 网关转发（关闭后所有调用返回 503）') +
      '<div id="probeOut" style="margin-top:12px;font-size:12.5px;color:var(--text-3)"></div>' +
      '</div></div>' +

      '<div class="panel"><div class="panel-head"><h3>安全与限流</h3></div><div class="panel-body">' +
      '<div class="form-grid">' +
      field('每 IP 每分钟请求数', num('rateLimitPerMin', s.rateLimitPerMin)) +
      field('突发容量', num('rateLimitBurst', s.rateLimitBurst)) +
      field('请求体上限 (MB)', num('maxBodyMb', s.maxBodyMb)) +
      field('登录失败封禁 (分钟)', num('banMinutes', s.banMinutes)) +
      '<div class="full">' + field('后台 IP 白名单',
        ta('adminIpAllowRaw', (s.adminIpAllow || []).join('\n'), '每行一个 IP 或 CIDR，留空表示不限制', 3),
        '配置后仅这些地址可访问管理端 API') + '</div>' +
      '</div></div></div>' +

      '<div style="display:flex;gap:10px;align-items:center;position:sticky;bottom:0;padding:14px 0;background:linear-gradient(transparent,var(--bg) 40%)">' +
      '<button class="btn btn-primary" id="setSave">保存设置</button>' +
      '<span style="font-size:12.5px;color:var(--text-3)" id="setStatus"></span></div>' +
      '</div>';

    const probeBtn = $('#btnProbe');
    if (probeBtn) {
      probeBtn.addEventListener('click', async function () {
        const out = $('#probeOut');
        probeBtn.disabled = true;
        out.innerHTML = '<span class="spinner"></span> 正在请求上游…';
        try {
          const r = await api.get('/api/admin/upstream/probe');
          const ok = r.ok;
          out.innerHTML = '<span class="tag ' + (ok ? 'tag-free' : 'tag-off') + '">' +
            (r.status || '失败') + '</span> <span style="margin-left:6px">' + (r.ms || 0) + 'ms</span>' +
            ' <span class="mono" style="margin-left:8px;color:var(--text-3)">' + esc(r.path || '') + '</span>' +
            '<pre style="margin-top:8px;max-height:160px;overflow:auto;background:var(--bg-3);padding:10px;border-radius:8px;font-size:11.5px">' +
            esc((r.body || r.error || '').slice(0, 600)) + '</pre>';
        } catch (e) {
          out.innerHTML = '<span class="tag tag-off">失败</span> <span style="margin-left:6px">' + esc(e.message) + '</span>';
        }
        probeBtn.disabled = false;
      });
    }

    $('#setSave').addEventListener('click', async function () {
      const v = collect($('#adminContent'));
      v.adminIpAllow = (v.adminIpAllowRaw || '').split('\n').map(function (x) { return x.trim(); }).filter(Boolean);
      delete v.adminIpAllowRaw;
      v.tts = s.tts;
      const btn = $('#setSave');
      btn.disabled = true;
      btn.innerHTML = '<span class="spinner"></span> 保存中';
      try {
        const r = await api.put('/api/admin/settings', v);
        SETTINGS = r.settings;
        $('#setStatus').textContent = '已保存 · ' + new Date().toLocaleTimeString('zh-CN');
        toast('设置已保存', 'ok');
      } catch (e) { toast(e.message, 'err'); }
      btn.disabled = false;
      btn.textContent = '保存设置';
    });
  }

  // ============ 广告位 ============
  async function viewAds() {
    let res;
    try { res = await api.get('/api/admin/ads'); }
    catch (e) { $('#adminContent').innerHTML = '<div class="empty">' + esc(e.message) + '</div>'; return; }

    $('#adminContent').innerHTML =
      '<div style="max-width:900px">' +
      '<p style="font-size:13.5px;color:var(--text-2);margin-bottom:16px;line-height:1.7">' +
      '每个广告位支持任意 HTML（含 &lt;script&gt; 与 iframe）。填写后打开开关即会出现在对应位置；' +
      '脚本类广告请自行确认来源可信。</p>' +
      '<div style="display:flex;flex-direction:column;gap:12px">' +
      (res.items || []).map(function (a) {
        return '<div class="panel" data-slot="' + esc(a.slot) + '">' +
          '<div class="panel-head"><h3 style="font-size:13.5px">' + esc(a.name || a.slot) + '</h3>' +
          '<span class="tag mono" style="font-size:10px">' + esc(a.slot) + '</span>' +
          '<span class="sp"></span>' +
          '<label class="switch"><input type="checkbox" data-adon' + (a.enabled ? ' checked' : '') + '><i></i></label>' +
          '</div><div class="panel-body">' +
          '<textarea class="textarea" data-adhtml style="font-family:var(--mono);font-size:12px;min-height:74px" ' +
          'placeholder="<a href=&#39;https://...&#39;><img src=&#39;...&#39;></a>">' + esc(a.html || '') + '</textarea>' +
          '<div style="display:flex;gap:8px;margin-top:10px;align-items:center">' +
          '<button class="btn btn-ghost btn-sm" data-adsave>保存</button>' +
          '<span style="font-size:11.5px;color:var(--text-3)" data-admsg></span></div>' +
          '</div></div>';
      }).join('') + '</div></div>';

    $('#adminContent').addEventListener('click', async function (e) {
      const b = e.target.closest('[data-adsave]');
      if (!b) return;
      const panel = b.closest('[data-slot]');
      const slot = panel.getAttribute('data-slot');
      const html = panel.querySelector('[data-adhtml]').value;
      const enabled = panel.querySelector('[data-adon]').checked;
      const name = panel.querySelector('h3').textContent;
      b.disabled = true;
      try {
        await api.post('/api/admin/ads', { slot: slot, name: name, html: html, enabled: enabled });
        panel.querySelector('[data-admsg]').textContent = '已保存';
        toast('广告位已更新', 'ok');
      } catch (ex) { toast(ex.message, 'err'); }
      b.disabled = false;
    });
  }

  // ============ 音色工坊（后台） ============
  async function viewTTS() {
    let cfgRes, voiceRes;
    try {
      cfgRes = await api.get('/api/admin/settings');
      voiceRes = await api.get('/api/admin/voices');
    } catch (e) {
      $('#adminContent').innerHTML = '<div class="empty">' + esc(e.message) + '</div>';
      return;
    }
    const t = cfgRes.settings.tts;

    $('#adminContent').innerHTML =
      '<div style="display:grid;grid-template-columns:minmax(0,1fr) minmax(0,1fr);gap:14px;align-items:start" class="tts-adm-grid">' +
      '<div class="panel"><div class="panel-head"><h3>服务配置</h3></div><div class="panel-body">' +
      sw('enabled', t.enabled, '启用音色工坊') +
      field('接口地址', input('baseUrl', t.baseUrl)) +
      field('API Key', input('apiKey', t.apiKey, 'sk-...'),
        '仅保存在服务端数据库中，任何前台接口都不会返回该值') +
      '<div class="form-grid">' +
      field('克隆模型', input('model', t.model)) +
      field('预置音色模型', input('presetModel', t.presetModel)) +
      '<div class="full">' + field('音色设计模型', input('designModel', t.designModel)) + '</div>' +
      field('默认预置音色', input('defaultVoice', t.defaultVoice)) +
      field('样本大小上限 (MB)', num('maxUploadMb', t.maxUploadMb)) +
      field('文本字数上限', num('maxChars', t.maxChars)) +
      field('请求超时 (秒)', num('timeoutSec', t.timeoutSec)) +
      field('记录保留 (天)', num('keepRenderDays', t.keepRenderDays)) +
      '</div>' +
      sw('guestAllowed', t.guestAllowed, '允许游客（未登录）使用') +
      '<button class="btn btn-primary btn-block" id="ttsSave">保存配置</button>' +
      '<div style="margin-top:12px;padding:11px 13px;border:1px solid var(--line);border-radius:8px;font-size:11.5px;color:var(--text-3);line-height:1.7">' +
      'Key 只用于服务端调用上游，前端仅能获知「是否已配置」。' +
      '</div>' +
      '</div></div>' +

      '<div class="panel"><div class="panel-head"><h3>全部音色样本</h3><span class="sp"></span>' +
      '<span class="hint">' + voiceRes.total + ' 个 · ' + bytes(voiceRes.voiceBytes) + '</span></div>' +
      '<div style="max-height:520px;overflow:auto">' +
      (voiceRes.voices.length
        ? '<table class="tbl"><thead><tr><th>名称</th><th style="width:80px">时长</th>' +
          '<th style="width:80px">大小</th><th style="width:70px">来源</th><th style="width:50px"></th></tr></thead><tbody>' +
          voiceRes.voices.map(function (v) {
            return '<tr data-vid="' + v.id + '"><td style="color:var(--text)">' + esc(v.name) + '</td>' +
              '<td class="mono" style="font-size:11.5px">' + (v.durationMs ? window.UI.dur(v.durationMs) : '—') + '</td>' +
              '<td class="mono" style="font-size:11.5px;color:var(--text-3)">' + bytes(v.size) + '</td>' +
              '<td style="font-size:11.5px">' + (v.source === 'record' ? '录音' : '上传') + '</td>' +
              '<td><a class="iconbtn" href="/api/tts/voices/' + v.id + '/audio" target="_blank">' + icon('play', 11) + '</a></td></tr>';
          }).join('') + '</tbody></table>'
        : '<div class="empty" style="border:0">暂无样本</div>') +
      '</div></div></div>';

    $('#ttsSave').addEventListener('click', async function () {
      const v = collect($('#adminContent'));
      const s = cfgRes.settings;
      s.tts = Object.assign({}, t, v);
      const btn = $('#ttsSave');
      btn.disabled = true;
      btn.innerHTML = '<span class="spinner"></span> 保存中';
      try {
        const r = await api.put('/api/admin/settings', s);
        SETTINGS = r.settings;
        toast('音色工坊配置已保存', 'ok');
      } catch (e) { toast(e.message, 'err'); }
      btn.disabled = false;
      btn.textContent = '保存配置';
    });
  }

  // ============ 账号 ============
  async function viewUsers() {
    let res;
    try { res = await api.get('/api/admin/users'); }
    catch (e) { $('#adminContent').innerHTML = '<div class="empty">' + esc(e.message) + '</div>'; return; }

    $('#adminContent').innerHTML =
      '<div class="toolbar"><span style="font-size:13px;color:var(--text-2)">共 ' +
      (res.items || []).length + ' 个账号</span><span style="flex:1"></span>' +
      '<button class="btn btn-primary btn-sm" id="uAdd">' + icon('plus', 14) + ' 新增账号</button></div>' +
      '<div class="table-wrap"><table class="tbl"><thead><tr>' +
      '<th>用户名</th><th style="width:90px">角色</th><th style="width:90px">状态</th>' +
      '<th style="width:150px">最近登录</th><th style="width:140px">创建时间</th>' +
      '<th style="width:60px"></th></tr></thead><tbody>' +
      (res.items || []).map(function (u) {
        return '<tr data-uid="' + u.id + '"><td style="color:var(--text)">' + esc(u.username) +
          (u.note ? '<span style="color:var(--text-3);font-size:11.5px;margin-left:8px">' + esc(u.note) + '</span>' : '') +
          '</td><td><span class="tag ' + (u.role === 'admin' ? 'tag-accent' : '') + '">' +
          (u.role === 'admin' ? '管理员' : '普通用户') + '</span></td>' +
          '<td><span class="tag ' + (u.status === 'active' ? 'tag-free' : 'tag-off') + '">' +
          (u.status === 'active' ? '正常' : '停用') + '</span></td>' +
          '<td style="font-size:12px;color:var(--text-3)">' + (u.lastLogin ? timeAgo(u.lastLogin) : '从未') + '</td>' +
          '<td style="font-size:12px;color:var(--text-3)">' + new Date(u.createdAt).toLocaleDateString('zh-CN') + '</td>' +
          '<td><div class="tbl-actions">' +
          '<button class="iconbtn" data-uact="edit" data-id="' + u.id + '">' + icon('edit', 11) + '</button>' +
          '<button class="iconbtn danger" data-uact="del" data-id="' + u.id + '">' + icon('trash', 11) + '</button>' +
          '</div></td></tr>';
      }).join('') + '</tbody></table></div>';

    $('#uAdd').addEventListener('click', function () {
      modal('新增账号',
        field('用户名', '<input class="input" id="nuName" placeholder="至少 3 位">') +
        field('密码', '<input class="input" id="nuPass" type="password" placeholder="至少 8 位">') +
        field('角色', '<select class="select" id="nuRole"><option value="user">普通用户</option>' +
          '<option value="admin">管理员</option></select>') +
        field('备注', '<input class="input" id="nuNote" placeholder="可选">'),
        '<button class="btn btn-ghost" onclick="__closeModal()">取消</button>' +
        '<button class="btn btn-primary" id="nuSave">创建</button>');
      $('#nuSave').addEventListener('click', async function () {
        try {
          await api.post('/api/admin/users', {
            username: $('#nuName').value.trim(),
            password: $('#nuPass').value,
            role: $('#nuRole').value,
            note: $('#nuNote').value
          });
          closeModal();
          toast('账号已创建', 'ok');
          viewUsers();
        } catch (e) { toast(e.message, 'err'); }
      });
    });

    $('#adminContent').addEventListener('click', async function (e) {
      const b = e.target.closest('[data-uact]');
      if (!b) return;
      const id = Number(b.getAttribute('data-id'));
      const u = (res.items || []).find(function (x) { return x.id === id; });
      if (b.getAttribute('data-uact') === 'del') {
        if (!confirm('确定删除账号「' + u.username + '」？')) return;
        try {
          await api.del('/api/admin/users/' + id);
          toast('已删除', 'ok');
          viewUsers();
        } catch (ex) { toast(ex.message, 'err'); }
        return;
      }
      modal('编辑账号 · ' + u.username,
        field('新密码', '<input class="input" id="euPass" type="password" placeholder="留空则不修改">') +
        field('角色', '<select class="select" id="euRole">' +
          '<option value="user"' + (u.role === 'user' ? ' selected' : '') + '>普通用户</option>' +
          '<option value="admin"' + (u.role === 'admin' ? ' selected' : '') + '>管理员</option></select>') +
        field('状态', '<select class="select" id="euStatus">' +
          '<option value="active"' + (u.status === 'active' ? ' selected' : '') + '>正常</option>' +
          '<option value="disabled"' + (u.status !== 'active' ? ' selected' : '') + '>停用</option></select>') +
        field('备注', '<input class="input" id="euNote" value="' + esc(u.note || '') + '">'),
        '<button class="btn btn-ghost" onclick="__closeModal()">取消</button>' +
        '<button class="btn btn-primary" id="euSave">保存</button>');
      $('#euSave').addEventListener('click', async function () {
        try {
          await api.put('/api/admin/users/' + id, {
            password: $('#euPass').value,
            role: $('#euRole').value,
            status: $('#euStatus').value,
            note: $('#euNote').value
          });
          closeModal();
          toast('已保存', 'ok');
          viewUsers();
        } catch (ex) { toast(ex.message, 'err'); }
      });
    });
  }

  // ============ 日志 ============
  let LOG_OFFSET = 0;
  async function viewLogs() {
    let res;
    try { res = await api.get('/api/admin/logs?limit=100&offset=' + LOG_OFFSET); }
    catch (e) { $('#adminContent').innerHTML = '<div class="empty">' + esc(e.message) + '</div>'; return; }

    $('#adminContent').innerHTML =
      '<div class="toolbar"><span style="font-size:13px;color:var(--text-2)">共 ' + res.total + ' 条</span>' +
      '<span style="flex:1"></span>' +
      '<button class="btn btn-ghost btn-sm" id="lgPrev"' + (LOG_OFFSET <= 0 ? ' disabled' : '') + '>上一页</button>' +
      '<button class="btn btn-ghost btn-sm" id="lgNext"' +
      (LOG_OFFSET + 100 >= res.total ? ' disabled' : '') + '>下一页</button>' +
      '<button class="btn btn-quiet btn-sm" id="lgRefresh">刷新</button></div>' +
      '<div class="table-wrap"><table class="tbl"><thead><tr>' +
      '<th style="width:150px">时间</th><th style="width:110px">IP</th><th style="width:70px">方法</th>' +
      '<th>路径</th><th style="width:70px">状态</th><th style="width:80px">耗时</th>' +
      '<th style="width:80px">类型</th></tr></thead><tbody>' +
      (res.items.length ? res.items.map(function (l) {
        return '<tr><td class="mono" style="font-size:11px;color:var(--text-3)">' +
          new Date(l.ts).toLocaleString('zh-CN') + '</td>' +
          '<td class="mono" style="font-size:11.5px">' + esc(l.ip) + '</td>' +
          '<td class="mono" style="font-size:11px;color:var(--text-2)">' + esc(l.method) + '</td>' +
          '<td class="mono" style="font-size:11.5px;color:var(--text-2);word-break:break-all">' + esc(l.path) + '</td>' +
          '<td><span class="mono" style="font-size:11.5px;color:' +
          (l.status < 400 ? 'var(--green)' : 'var(--red)') + '">' + l.status + '</span></td>' +
          '<td class="mono" style="font-size:11px;color:var(--text-3)">' + l.ms + 'ms</td>' +
          '<td><span class="tag" style="font-size:10px">' + esc(l.kind || 'page') + '</span></td></tr>';
      }).join('') : '<tr><td colspan="7"><div class="empty" style="border:0">暂无日志</div></td></tr>') +
      '</tbody></table></div>';

    $('#lgPrev').addEventListener('click', function () { LOG_OFFSET = Math.max(0, LOG_OFFSET - 100); viewLogs(); });
    $('#lgNext').addEventListener('click', function () { LOG_OFFSET += 100; viewLogs(); });
    $('#lgRefresh').addEventListener('click', viewLogs);
  }

  // ============ 导入导出 ============
  function viewData() {
    $('#adminContent').innerHTML =
      '<div style="display:grid;grid-template-columns:1fr 1fr;gap:14px;align-items:start;max-width:1000px" class="data-grid">' +
      '<div class="panel"><div class="panel-head"><h3>导出</h3></div><div class="panel-body">' +
      '<p style="font-size:13px;color:var(--text-2);line-height:1.7;margin-bottom:14px">' +
      '导出全部接口与分类为 JSON，可用于备份或迁移到其他环境。</p>' +
      '<a class="btn btn-ghost btn-block" href="/api/admin/export" download>' + icon('download', 15) + ' 下载 JSON 备份</a>' +
      '</div></div>' +
      '<div class="panel"><div class="panel-head"><h3>导入</h3></div><div class="panel-body">' +
      '<p style="font-size:13px;color:var(--text-2);line-height:1.7;margin-bottom:12px">' +
      '粘贴导出格式的 JSON。同 slug 的接口会被覆盖。</p>' +
      '<textarea class="textarea" id="impJSON" style="font-family:var(--mono);font-size:11.5px;min-height:180px" ' +
      'placeholder="{ &quot;interfaces&quot;: [ ... ], &quot;categories&quot;: [ ... ] }"></textarea>' +
      '<label class="switchline" style="margin:12px 0"><input type="checkbox" id="impWipe"> ' +
      '导入前清空现有接口（危险）</label>' +
      '<button class="btn btn-primary btn-block" id="impBtn">开始导入</button>' +
      '<div id="impMsg" style="font-size:12.5px;color:var(--text-3);margin-top:10px"></div>' +
      '</div></div></div>';

    $('#impBtn').addEventListener('click', async function () {
      const raw = $('#impJSON').value.trim();
      if (!raw) { toast('请先粘贴 JSON', 'err'); return; }
      let data;
      try { data = JSON.parse(raw); }
      catch (e) { toast('JSON 解析失败：' + e.message, 'err'); return; }
      const wipe = $('#impWipe').checked;
      if (wipe && !confirm('将清空现有全部接口后导入，确定继续？')) return;
      const btn = $('#impBtn');
      btn.disabled = true;
      btn.innerHTML = '<span class="spinner"></span> 导入中';
      try {
        const r = await api.post('/api/admin/import', {
          wipe: wipe,
          interfaces: data.interfaces || data,
          categories: data.categories || []
        });
        $('#impMsg').textContent = '成功导入 ' + r.imported + ' 个接口、' + r.categories + ' 个分类';
        toast('导入完成', 'ok');
        await loadCats();
      } catch (e) { toast(e.message, 'err'); }
      btn.disabled = false;
      btn.textContent = '开始导入';
    });
  }

  const VIEWS = {
    overview: viewOverview,
    interfaces: viewInterfaces,
    categories: viewCategories,
    settings: viewSettings,
    ads: viewAds,
    tts: viewTTS,
    users: viewUsers,
    logs: viewLogs,
    data: viewData
  };

  async function loadCats() {
    try {
      const r = await api.get('/api/admin/categories');
      CATS = r.items || [];
    } catch (e) { /* ignore */ }
  }

  // ============ 启动 ============
  async function boot() {
    $('#loginForm').addEventListener('submit', doLogin);
    $('#btnLogout').addEventListener('click', async function (e) {
      e.preventDefault();
      await api.post('/api/auth/logout', {}).catch(function () {});
      ME = null;
      location.href = '/';          // 退出后回前台，登录统一走 /console
    });
    $('#btnQuickAdd').addEventListener('click', function () { openIfaceEditor(null); });
    window.addEventListener('hashchange', function () { if (ME) route(); });

    // 登录统一走前台 /console，这里只做身份校验
    let user = null;
    try {
      const r = await api.get('/api/auth/me');
      user = r.user || null;
    } catch (e) { user = null; }

    if (!user) { location.replace('/console'); return; }
    if (user.role !== 'admin') {
      $('#loginView').classList.add('hidden');
      $('#adminView').classList.add('hidden');
      document.body.innerHTML =
        '<div style="min-height:100vh;display:grid;place-items:center;text-align:center">' +
        '<div><div style="font-size:15px;margin-bottom:10px">该账号没有后台权限</div>' +
        '<div style="font-size:13px;color:var(--text-3)">请使用管理员账号登录后再访问。</div>' +
        '<div style="margin-top:16px"><a class="btn btn-ghost btn-sm" href="/">返回首页</a>' +
        '<a class="btn btn-ghost btn-sm" href="/console" style="margin-left:8px">切换账号</a></div></div></div>';
      return;
    }
    ME = user;
    if (user.mustChangePass) showForcePass();
    await loadCats();
    try { SETTINGS = (await api.get('/api/admin/settings')).settings; } catch (e) { /* ignore */ }
    showAdmin();
  }

  document.addEventListener('DOMContentLoaded', boot);
})();
