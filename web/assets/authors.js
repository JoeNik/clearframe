/* 我的作者：关注列表 + 作品快照管理。
   支持增量刷新（只拉新作品）与多选分批深查（播放量等）。 */
(function () {
  'use strict';
  const { $, icon, esc, api, toast, renderHeader, renderFooter, renderAds, timeAgo } = window.UI;
  const num = window.RENDER.num;
  const PAGE_SIZE = 50;

  let ME = null;
  let SITE = null;
  const state = {
    authors: [],
    current: null,          // 当前查看的作者
    videos: [],
    total: 0,
    page: 1,
    selected: new Set(),
    busy: false
  };

  // ---------- 骨架 ----------

  function shell() {
    return '<div style="display:flex;flex-direction:column;gap:18px;min-width:0">' +
      '<div id="viewHead"></div>' +
      '<div id="viewBody"></div>' +
      '</div>';
  }

  function boot() {
    $('#appMain').innerHTML = shell();
    $('#appMain').classList.add('tts-shell');
  }

  async function init() {
    let BOOT = null;
    try { BOOT = await api.get('/api/public/bootstrap'); } catch (e) { /* 忽略 */ }
    SITE = (BOOT && BOOT.site) || {};
    ME = BOOT && BOOT.me;
    renderHeader($('#header'), SITE, 'authors', ME);
    renderFooter($('#footer'), SITE);
    if (BOOT) renderAds(BOOT.ads);
    boot();

    if (!ME) {
      $('#viewHead').innerHTML = '';
      $('#viewBody').innerHTML =
        '<div class="panel"><div class="panel-body" style="text-align:center;padding:48px 24px">' +
        '<div style="font-size:34px">' + icon('shield', 34) + '</div>' +
        '<h3 style="margin:14px 0 8px">请先登录</h3>' +
        '<p style="color:var(--text-2);font-size:14px;line-height:1.8">关注作者与作品数据按账号保存，登录后即可使用。</p>' +
        '<p style="margin-top:16px"><a class="btn btn-primary" href="/console">去登录 / 注册</a></p>' +
        '</div></div>';
      return;
    }
    await loadAuthors();
  }

  // ---------- 作者列表 ----------

  async function loadAuthors() {
    let r;
    try { r = await api.get('/api/dy/authors'); }
    catch (e) {
      $('#viewBody').innerHTML = '<div class="cap-noparam">' + esc(e.message || '读取失败') + '</div>';
      return;
    }
    state.authors = r.items || [];
    renderList();
  }

  function renderList() {
    $('#viewHead').innerHTML =
      '<div>' +
        '<div class="eyebrow" style="margin-bottom:10px">My Authors</div>' +
        '<h1 style="font-size:clamp(26px,3.4vw,36px);letter-spacing:-.03em">我的作者</h1>' +
        '<p style="color:var(--text-2);font-size:14.5px;margin-top:8px;max-width:680px;line-height:1.7">' +
          '关注抖音作者后，作品链接与互动数据会保存为快照，不用每次重新爬取；刷新只拉新作品。' +
          '在「<a href="/cap/dyzz">抖音作者作品分析</a>」查询结果的底部可以一键关注。</p>' +
      '</div>' +
      '<div class="panel"><div class="panel-body">' +
        '<div style="display:flex;gap:10px;flex-wrap:wrap">' +
          '<input class="input" id="addInput" style="flex:1;min-width:240px" ' +
            'placeholder="粘贴作者主页链接或 sec_uid，服务端会自动拉取作品快照">' +
          '<button class="btn btn-primary" id="addBtn">' + icon('plus', 14) + ' 添加并同步</button>' +
        '</div>' +
        '<div id="addStatus" style="margin-top:10px;font-size:12.5px;color:var(--text-3)"></div>' +
      '</div></div>';

    $('#addBtn').addEventListener('click', addFromInput);
    $('#addInput').addEventListener('keydown', function (e) {
      if (e.key === 'Enter') addFromInput();
    });

    if (!state.authors.length) {
      $('#viewBody').innerHTML =
        '<div class="panel"><div class="panel-body" style="text-align:center;padding:44px 24px">' +
        '<div style="font-size:34px">' + icon('users', 34) + '</div>' +
        '<h3 style="margin:14px 0 8px">还没有关注的作者</h3>' +
        '<p style="color:var(--text-2);font-size:14px;line-height:1.8">去 <a href="/cap/dyzz">抖音作者作品分析</a> 查询一个作者，' +
        '在结果底部点「关注此作者」；或直接在上面粘贴主页链接添加。</p>' +
        '</div></div>';
      return;
    }

    $('#viewBody').innerHTML = '<div class="author-grid">' + state.authors.map(function (a) {
      return '<div class="panel author-card">' +
        '<div class="panel-body">' +
        '<div style="display:flex;gap:12px;align-items:center">' +
          (a.avatar ? '<img class="avatar" src="' + esc(a.avatar) + '" alt="" referrerpolicy="no-referrer">' : '<span class="avatar ph"></span>') +
          '<div style="min-width:0">' +
            '<div style="font-weight:600;overflow:hidden;text-overflow:ellipsis;white-space:nowrap">' + esc(a.nickname || '未知作者') + '</div>' +
            '<div class="mono" style="font-size:11px;color:var(--text-3)">' + esc(a.secUid.slice(0, 20)) + '…</div>' +
          '</div>' +
        '</div>' +
        '<div class="mini-stats" style="padding:12px 0 0">' +
          '<div class="mini-stat"><b>' + num(a.videoCount) + '</b><span>快照作品</span></div>' +
          (a.followerCount ? '<div class="mini-stat"><b>' + num(a.followerCount) + '</b><span>粉丝</span></div>' : '') +
          '<div class="mini-stat"><b>' + (a.lastSyncedAt ? timeAgo(a.lastSyncedAt * 1000) : '未') + '</b><span>上次同步</span></div>' +
          (a.lastNewCount ? '<div class="mini-stat"><b>+' + num(a.lastNewCount) + '</b><span>上次新增</span></div>' : '') +
        '</div>' +
        '<div style="display:flex;gap:8px;margin-top:14px;flex-wrap:wrap">' +
          '<button class="btn btn-primary btn-sm" data-open="' + a.id + '">' + icon('video', 13) + ' 作品管理</button>' +
          '<button class="btn btn-ghost btn-sm" data-refresh="' + a.id + '">' + icon('download', 13) + ' 增量刷新</button>' +
          '<button class="btn btn-ghost btn-sm" data-unfollow="' + a.id + '" style="color:var(--red)">取关</button>' +
        '</div>' +
        '</div></div>';
    }).join('') + '</div>';

    $$('#viewBody [data-open]').forEach(function (b) {
      b.addEventListener('click', function () { openAuthor(Number(b.getAttribute('data-open'))); });
    });
    $$('#viewBody [data-refresh]').forEach(function (b) {
      b.addEventListener('click', function () { refreshAuthor(Number(b.getAttribute('data-refresh')), b); });
    });
    $$('#viewBody [data-unfollow]').forEach(function (b) {
      b.addEventListener('click', async function () {
        const id = Number(b.getAttribute('data-unfollow'));
        if (!window.confirm('取消关注后不再出现在列表；已保存的作品快照会保留。确定取关？')) return;
        try {
          await api.del('/api/dy/authors/' + id);
          toast('已取关', 'ok');
          await loadAuthors();
        } catch (e) { toast(e.message || '操作失败', 'err'); }
      });
    });
  }

  async function addFromInput() {
    const input = $('#addInput');
    const v = input.value.trim();
    if (!v) { toast('请先粘贴作者主页链接或 sec_uid'); return; }
    const btn = $('#addBtn');
    const st = $('#addStatus');
    btn.disabled = true;
    btn.innerHTML = '<span class="spinner"></span> 拉取中';
    st.textContent = '正在解析并拉取作品快照，作者作品较多时需要十几秒…';
    try {
      const r = await api.post('/api/dy/authors', { url: v });
      st.textContent = '已保存「' + (r.author.nickname || r.author.secUid.slice(0, 12) + '…') + '」的 ' + r.saved + ' 条作品快照';
      toast('关注成功', 'ok');
      input.value = '';
      await loadAuthors();
    } catch (e) {
      st.textContent = '';
      toast(e.message || '添加失败', 'err');
    }
    btn.disabled = false;
    btn.innerHTML = icon('plus', 14) + ' 添加并同步';
  }

  async function refreshAuthor(id, btn) {
    if (state.busy) return;
    state.busy = true;
    const old = btn.innerHTML;
    btn.disabled = true;
    btn.innerHTML = '<span class="spinner"></span> 刷新中';
    try {
      const r = await api.post('/api/dy/authors/' + id + '/refresh');
      toast('刷新完成：新增 ' + r.new_count + ' 条，更新 ' + r.updated + ' 条数据', 'ok');
      if (state.current && state.current.id === id) {
        await openAuthor(id, state.page);
      } else {
        await loadAuthors();
      }
    } catch (e) {
      toast(e.message || '刷新失败', 'err');
    }
    btn.disabled = false;
    btn.innerHTML = old;
    state.busy = false;
  }

  // ---------- 作品管理（详情） ----------

  async function openAuthor(id, page) {
    state.current = state.authors.find(function (a) { return a.id === id; }) || null;
    state.page = page || 1;
    state.selected.clear();
    await loadVideos();
  }

  async function loadVideos() {
    const a = state.current;
    if (!a) return;
    $('#viewHead').innerHTML =
      '<div style="display:flex;align-items:center;gap:12px;flex-wrap:wrap">' +
        '<button class="btn btn-ghost btn-sm" id="backBtn">' + icon('arrow', 13) + ' 返回列表</button>' +
        '<span class="sp" style="flex:1"></span>' +
        '<button class="btn btn-ghost btn-sm" id="refreshBtn">' + icon('download', 13) + ' 增量刷新（只拉新作品）</button>' +
      '</div>';

    let r;
    try {
      r = await api.get('/api/dy/authors/' + a.id + '/videos?page=' + state.page + '&size=' + PAGE_SIZE);
    } catch (e) {
      $('#viewBody').innerHTML = '<div class="cap-noparam">' + esc(e.message || '读取失败') + '</div>';
      return;
    }
    const au = r.author || a;
    state.current = au;
    state.videos = r.items || [];
    state.total = r.total || 0;

    $('#viewHead').innerHTML +=
      '<div class="panel"><div class="panel-body" style="display:flex;gap:14px;align-items:center;flex-wrap:wrap">' +
        (au.avatar ? '<img class="avatar" src="' + esc(au.avatar) + '" alt="" referrerpolicy="no-referrer">' : '<span class="avatar ph"></span>') +
        '<div style="min-width:0;flex:1">' +
          '<div style="font-weight:600;font-size:17px">' + esc(au.nickname || '未知作者') + '</div>' +
          '<div style="font-size:12.5px;color:var(--text-3);margin-top:2px">快照 ' + num(state.total) +
            ' 条 · 上次全量同步 ' + (au.lastSyncedAt ? timeAgo(au.lastSyncedAt * 1000) : '未') +
            (au.lastNewCount ? ' · 上次新增 ' + au.lastNewCount : '') + '</div>' +
        '</div>' +
      '</div></div>';

    const pages = Math.max(1, Math.ceil(state.total / PAGE_SIZE));
    $('#viewBody').innerHTML =
      '<div class="panel"><div class="panel-body">' +
        '<div class="vt-wrap"><table class="video-table"><thead><tr>' +
          '<th class="vt-l vt-chk-head"><input type="checkbox" id="chkAll"></th>' +
          '<th class="vt-l">作品</th><th class="vt-l">发布时间</th><th class="vt-r">时长</th>' +
          '<th class="vt-r">播放</th><th class="vt-r">点赞</th><th class="vt-r">评论</th>' +
          '<th class="vt-r">分享</th><th class="vt-r">收藏</th><th class="vt-r">操作</th>' +
        '</tr></thead><tbody id="videoRows"></tbody></table></div>' +
        '<div style="display:flex;align-items:center;gap:12px;margin-top:12px;flex-wrap:wrap">' +
          '<button class="btn btn-ghost btn-sm" id="prevBtn"' + (state.page <= 1 ? ' disabled' : '') + '>上一页</button>' +
          '<span style="font-size:12.5px;color:var(--text-3)">第 ' + state.page + ' / ' + pages + ' 页</span>' +
          '<button class="btn btn-ghost btn-sm" id="nextBtn"' + (state.page >= pages ? ' disabled' : '') + '>下一页</button>' +
          '<span class="sp" style="flex:1"></span>' +
          '<span id="batchStatus" style="font-size:12.5px;color:var(--text-3)"></span>' +
          '<button class="btn btn-primary btn-sm" id="batchBtn" disabled>' + icon('bolt', 13) +
            ' 查询已选 (0)</button>' +
        '</div>' +
        '<div style="font-size:12px;color:var(--text-3);margin-top:8px">' +
          '「播放」来自单作品深度查询（点击「查询已选」后自动分批请求，每次 10 个、自动间隔限速）；' +
          '「趋势」在新页打开该作品的播放 / 互动逐日数据。</div>' +
      '</div></div>';

    $('#backBtn').addEventListener('click', renderList);
    $('#refreshBtn').addEventListener('click', function () { refreshAuthor(a.id, $('#refreshBtn')); });
    $('#prevBtn').addEventListener('click', function () { if (state.page > 1) openAuthor(a.id, state.page - 1); });
    $('#nextBtn').addEventListener('click', function () { openAuthor(a.id, state.page + 1); });
    $('#batchBtn').addEventListener('click', batchStats);
    $('#chkAll').addEventListener('change', function () {
      const on = $('#chkAll').checked;
      state.videos.forEach(function (v) {
        if (on) state.selected.add(v.awemeId); else state.selected.delete(v.awemeId);
      });
      renderRows();
    });
    renderRows();
  }

  function renderRows() {
    const body = $('#videoRows');
    if (!body) return;
    body.innerHTML = state.videos.map(function (v) {
      const tags = (v.isTop ? '<span class="tag tag-accent">置顶</span>' : '') +
        (v.kind === 'image' ? '<span class="tag">图集</span>' : '') +
        (v.isDeleted ? '<span class="tag tag-warn">已删除</span>' : '') +
        (v.isPrivate ? '<span class="tag tag-warn">私密</span>' : '');
      const playCell = v.playCount != null
        ? '<td class="vt-r"><b>' + num(v.playCount) + '</b><div style="font-size:10.5px;color:var(--text-3)">' +
            (v.statsUpdatedAt ? timeAgo(v.statsUpdatedAt * 1000) : '') + '</div></td>'
        : '<td class="vt-r" style="color:var(--text-3)">—</td>';
      const rateCell = v.interactRate != null
        ? '<div style="font-size:10.5px;color:var(--text-3)">互动率 ' + (v.interactRate * 100).toFixed(2) + '%</div>'
        : '';
      return '<tr>' +
        '<td class="vt-l vt-chk"><input type="checkbox" data-chk="' + esc(v.awemeId) + '"' +
          (state.selected.has(v.awemeId) ? ' checked' : '') + '></td>' +
        '<td class="vt-l vt-title">' +
          (v.cover ? '<img class="vt-cover" src="' + esc(v.cover) + '" alt="" loading="lazy" referrerpolicy="no-referrer">' : '') +
          '<span class="vt-t" title="' + esc(v.desc || v.title || '') + '">' + esc(v.title || '未命名作品') + '</span>' + tags + '</td>' +
        '<td class="vt-l vt-date">' + dyDate(v.createTime) + '</td>' +
        '<td class="vt-r">' + (v.duration ? fmtDur(v.duration) : '—') + '</td>' +
        playCell +
        '<td class="vt-r">' + num(v.diggCount) + rateCell + '</td>' +
        '<td class="vt-r">' + num(v.commentCount) + '</td>' +
        '<td class="vt-r">' + num(v.shareCount) + '</td>' +
        '<td class="vt-r">' + num(v.collectCount) + '</td>' +
        '<td class="vt-r vt-act">' +
          '<a class="btn btn-ghost btn-sm" href="/cap/dytj?id=' + encodeURIComponent(v.awemeId) + '" target="_blank" rel="noopener">趋势</a>' +
          '<a class="btn btn-ghost btn-sm" href="' + esc(v.url) + '" target="_blank" rel="noopener">原链</a>' +
        '</td></tr>';
    }).join('');

    $$('#videoRows [data-chk]').forEach(function (cb) {
      cb.addEventListener('change', function () {
        if (cb.checked) state.selected.add(cb.getAttribute('data-chk'));
        else state.selected.delete(cb.getAttribute('data-chk'));
        updateBatchBar();
      });
    });
    updateBatchBar();
  }

  function updateBatchBar() {
    const btn = $('#batchBtn');
    if (!btn) return;
    btn.innerHTML = icon('bolt', 13) + ' 查询已选 (' + state.selected.size + ')';
    btn.disabled = state.selected.size === 0 || state.busy;
    const st = $('#batchStatus');
    if (st && !state.busy) st.textContent = state.selected.size ? '' : '';
  }

  // 多选分批深查：每批 10 个，前端逐批请求，进度实时可见，失败的单独提示
  async function batchStats() {
    if (state.busy || !state.current) return;
    const ids = Array.from(state.selected);
    if (!ids.length) return;
    state.busy = true;
    const btn = $('#batchBtn');
    const st = $('#batchStatus');
    btn.disabled = true;
    const failList = [];
    let done = 0;
    for (let i = 0; i < ids.length; i += 10) {
      const chunk = ids.slice(i, i + 10);
      st.innerHTML = '<span class="spinner"></span> 查询中 ' + Math.min(done + chunk.length, ids.length) +
        ' / ' + ids.length + '（每批间隔约 0.4 秒/条）';
      try {
        const r = await api.post('/api/dy/authors/' + state.current.id + '/stats', { aweme_ids: chunk });
        const got = {};
        (r.items || []).forEach(function (v) { got[v.awemeId] = v; });
        state.videos = state.videos.map(function (v) { return got[v.awemeId] || v; });
        (r.errors || []).forEach(function (e) { failList.push(e); });
      } catch (e) {
        failList.push({ aweme_id: chunk.join(','), error: e.message || '请求失败' });
      }
      done += chunk.length;
      renderRows();
    }
    state.busy = false;
    state.selected.clear();
    renderRows();
    if (failList.length) {
      st.innerHTML = '<span style="color:var(--red)">' + failList.length + ' 个失败</span>　' +
        '<span style="color:var(--text-3)">' + esc(String(failList[0].error)).slice(0, 60) + '</span>';
      toast('部分作品查询失败，多为上游临时限流，稍后可再选这些重试', 'err');
    } else {
      st.textContent = '全部查询完成';
      toast('查询完成，播放量已保存到快照', 'ok');
    }
    btn.disabled = state.selected.size === 0;
  }

  // ---------- 工具 ----------

  function dyDate(ts) {
    const n = Number(ts);
    if (!n) return '—';
    const d = new Date(n * 1000);
    return d.getFullYear() + '-' + String(d.getMonth() + 1).padStart(2, '0') + '-' + String(d.getDate()).padStart(2, '0');
  }

  function fmtDur(sec) {
    const s = Math.round(sec);
    return Math.floor(s / 60) + ':' + String(s % 60).padStart(2, '0');
  }

  document.addEventListener('DOMContentLoaded', init);
})();
