/* 首页：解析工具 + 能力矩阵 */
(function () {
  'use strict';
  const { $, $$, icon, esc, api, renderHeader, renderFooter, renderAds, toast } = window.UI;

  let BOOT = null;
  let IFACES = [];
  let CATS = [];
  let BUSY = false;

  // 平台识别规则：命中即用对应能力，否则走聚合通道
  const RULES = [
    { re: /(douyin|iesdouyin)\.com|v\.douyin/i, slug: 'dyjx', label: '抖音' },
    { re: /kuaishou\.com|v\.kuaishou|gifshow/i, slug: 'ksjx', label: '快手' },
    { re: /(bilibili|b23\.tv|bili2233|acg\.tv)/i, slug: 'bilibili', label: 'B站' },
    { re: /(weixin|channels|finder|wxapp)/i, slug: 'wxsph', label: '视频号' },
    { re: /pipix/i, slug: 'pipigx', label: '皮皮搞笑' },
    { re: /doubao/i, slug: 'doubao', label: '豆包' },
    { re: /jimeng/i, slug: 'jimeng', label: '即梦' },
    { re: /dola/i, slug: 'dola', label: 'Dola' },
    { re: /(xiaohongshu|xhslink)/i, slug: 'svparse', label: '小红书' }
  ];

  const CHIPS = [
    { label: '抖音', slug: 'dyjx', demo: 'https://v.douyin.com/XBwlKFr1ya0/' },
    { label: '作品数据', slug: 'dytj', demo: 'https://www.douyin.com/video/7699804945230384411' },
    { label: '快手', slug: 'ksjx', demo: 'https://v.kuaishou.com/KAjGG5zb' },
    { label: 'B站', slug: 'bilibili', demo: 'https://www.bilibili.com/video/BV1GJ411x7h7' },
    { label: '视频号', slug: 'wxsph', demo: '' },
    { label: '小红书', slug: 'svparse', demo: '' },
    { label: '即梦', slug: 'jimeng', demo: 'https://jimeng.jianying.com/s/5I-I8pYkPFU/?t=8011' },
    { label: '豆包', slug: 'doubao', demo: '' },
    { label: '聚合解析', slug: 'svparse', demo: '' }
  ];

  const ICON_OF = {
    dyjx: 'video', svparse: 'grid', wxsph: 'video', zjyh: 'video',
    jimeng: 'video', doubao: 'video', ksjx: 'video', bilibili: 'video',
    dydj: 'play', dytj: 'gauge', bbhj: 'users', dola: 'video',
    pipigx: 'video', domailcx: 'link'
  };

  function pick(url) {
    for (const r of RULES) if (r.re.test(url)) return { slug: r.slug, label: r.label };
    return { slug: 'svparse', label: '聚合解析' };
  }

  function extractUrl(text) {
    const m = String(text || '').match(/https?:\/\/[^\s，,、"'）)】\]]+/);
    return m ? m[0] : String(text || '').trim();
  }

  // ---------- 调用 ----------
  async function call(slug, url) {
    const r = await fetch('/v1/' + slug + '?url=' + encodeURIComponent(url), {
      method: 'GET',
      headers: { 'Accept': 'application/json' }
    });
    const text = await r.text();
    let data = null;
    try { data = JSON.parse(text); } catch (e) { /* 非 JSON */ }

    const remaining = r.headers.get('X-Guest-Remaining');
    const out = { ok: false, media: null, msg: '', code: r.status, remaining };

    const pickMsg = function () {
      if (!data) return '';
      if (typeof data.error === 'string' && data.error) return data.error;
      if (data.error && typeof data.error === 'object') return data.error.message || data.error.code || '';
      return data.message || data.msg || '';
    };

    if (!r.ok) {
      out.msg = pickMsg() || ('接口返回 HTTP ' + r.status);
      out.code = (data && typeof data.code !== 'undefined') ? data.code : r.status;
      return out;
    }
    if (data && typeof data.code !== 'undefined' && Number(data.code) !== 200) {
      out.msg = pickMsg() || '解析失败';
      out.code = data.code;
      return out;
    }
    const media = data && data.data;
    if (!media || (Array.isArray(media) && media.length === 0)) {
      out.msg = '没有解析到内容，可能是链接失效、作品已删除或平台暂不支持';
      out.code = 200;
      return out;
    }
    out.ok = true;
    out.code = 200;
    out.media = Array.isArray(media) ? media[0] : media;
    return out;
  }

  // ---------- 解析 ----------
  async function parse() {
    if (BUSY) return;
    const raw = $('#urlInput').value.trim();
    if (!raw) { $('#urlInput').focus(); toast('先粘贴一个分享链接'); return; }
    const url = extractUrl(raw);
    if (!/^https?:\/\//i.test(url)) { toast('没有识别到有效链接，请粘贴完整分享文案'); return; }

    const target = pick(url);
    const btn = $('#btnParse');
    const st = $('#parseStatus');
    const wrap = $('#resultWrap');
    BUSY = true;
    btn.disabled = true;
    $('#btnParseText').textContent = '解析中';
    wrap.innerHTML = '';
    st.innerHTML = '<div class="parse-loading"><span class="scan-line"></span>' +
      '<span class="spinner"></span><span>正在通过「' + esc(target.label) + '」通道解析…</span></div>';

    try {
      let res = await call(target.slug, url);
      if (!res.ok && target.slug !== 'svparse') {
        st.innerHTML = '<div class="parse-loading"><span class="spinner"></span>' +
          '<span>「' + esc(target.label) + '」通道暂不可用，正在切换聚合通道重试…</span></div>';
        const alt = await call('svparse', url);
        if (alt.ok) { alt.note = '已自动切换聚合通道'; res = alt; }
      }
      if (!res.ok) { showError(res.msg, res.code, target.label); return; }

      st.innerHTML = '';
      window.RENDER.show(wrap, res.media, { sourceLabel: target.label + ' · 已去除水印', note: res.note });
      wrap.scrollIntoView({ behavior: 'smooth', block: 'start' });
      if (res.remaining !== null) renderQuota(res.remaining);
    } catch (e) {
      showError('网络异常：' + e.message, 0, target.label);
    } finally {
      BUSY = false;
      btn.disabled = false;
      $('#btnParseText').textContent = '立即解析';
    }
  }

  function showError(msg, code, label) {
    $('#parseStatus').innerHTML = '';
    const rate = Number(code) === 429 || /频繁|限流|稍后/.test(String(msg));
    $('#resultWrap').innerHTML =
      '<div class="result-card err">' +
        '<div class="err-ico">' + icon('alert', 22) + '</div>' +
        '<div><div class="err-title">' + (rate ? '上游临时限流' : '解析失败') + '</div>' +
          '<div class="err-msg">' + esc(msg) + '</div>' +
          '<div class="err-hint">' + (rate
            ? '等 1 分钟左右再点一次「立即解析」即可，不用换链接；刚解析过的内容 60 秒内会直接命中缓存。'
            : '通道：' + esc(label) + (code ? ' · 状态码 ' + esc(String(code)) : '') +
              '　可以换一个平台或稍后重试。') + '</div></div>' +
      '</div>';
  }

  function renderQuota(rem) {
    const el = $('#parseQuota');
    if (!el) return;
    const n = Number(rem);
    if (isNaN(n)) { el.textContent = ''; return; }
    const me = BOOT && BOOT.me;
    if (me) {
      el.innerHTML = n > 0 ? '今日还可解析 <b>' + n + '</b> 次' : '今日额度已用完，明天再来';
      return;
    }
    el.innerHTML = n > 0
      ? '游客今日还可解析 <b>' + n + '</b> 次 · <a href="/console" style="color:var(--accent)">登录后额度更高</a>'
      : '今日免费次数已用完 · <a href="/console" style="color:var(--accent)">登录 / 注册</a> 继续解析';
  }

  // ---------- 能力矩阵 ----------
  function capCard(it, wide, demo) {
    const ico = ICON_OF[it.slug] || (it.category === 'data' ? 'gauge' : it.category === 'tool' ? 'tool' : 'video');
    const badge = it.category === 'data'
      ? '<span class="cap-badge data">数据分析</span>'
      : it.category === 'tool'
        ? '<span class="cap-badge tool">实用工具</span>'
        : '';
    // 计费徽标：跟随上游同步状态
    const priceBadge = it.pricingMode === 'paid'
      ? '<span class="cap-badge paid">¥' + (Number(it.price) || 0) + '/' + esc(it.priceUnit || '次') + '</span>'
      : '<span class="cap-badge free">免费</span>';
    return '<a class="cap-card' + (wide ? ' cap-wide' : '') + '" href="/cap/' + esc(it.slug) + '" data-spot>' +
      '<div class="cap-glow"></div>' +
      '<div class="cap-top"><span class="cap-ico">' + icon(ico, 18) + '</span>' + badge + priceBadge +
        '<span class="cap-arrow">' + icon('arrow', 15) + '</span></div>' +
      '<div class="cap-name">' + esc(it.name) + '</div>' +
      '<div class="cap-desc">' + esc(it.summary || '') + '</div>' +
      (demo ? '<div class="cap-demo">' + miniSpark() + '</div>' : '') +
      '<div class="cap-foot"><span class="cap-go">立即使用</span>' +
        (it.popularity ? '<span class="cap-heat">' + (it.popularity / 10).toFixed(0) + '% 热度</span>' : '') +
      '</div></a>';
  }

  // 装饰用迷你波形，仅出现在宽卡上
  function miniSpark() {
    const pts = [12, 18, 14, 26, 22, 34, 30, 44, 38, 52, 46, 62];
    const W = 260, H = 56, max = 62;
    const d = pts.map(function (v, i) {
      return (i ? 'L' : 'M') + (i * (W / (pts.length - 1))).toFixed(1) + ' ' + (H - (v / max) * H).toFixed(1);
    }).join(' ');
    return '<svg viewBox="0 0 ' + W + ' ' + H + '" preserveAspectRatio="none" aria-hidden="true">' +
      '<path d="' + d + ' L' + W + ' ' + H + ' L0 ' + H + ' Z" fill="rgba(198,242,78,.08)"/>' +
      '<path d="' + d + '" fill="none" stroke="var(--accent)" stroke-width="1.6" stroke-linecap="round"/></svg>';
  }

  const GROUP_META = [
    { slug: 'parse', title: '视频解析', desc: '粘贴链接即可拿到无水印原画视频、图集与实况照片' },
    { slug: 'data', title: '数据分析', desc: '把作品背后的播放、互动、趋势数据摊开来看' },
    { slug: 'tool', title: '实用工具', desc: '顺手用的小工具，解决偶发需求' }
  ];

  function renderCapabilities() {
    const box = $('#capGrid');
    if (!box) return;
    let html = '';
    GROUP_META.forEach(function (g) {
      const list = IFACES.filter(function (i) { return i.category === g.slug; });
      if (!list.length) return;
      html += '<div class="cap-group" data-reveal>' +
        '<div class="cap-group-head">' +
          '<h3>' + esc(g.title) + '<span class="cap-count">' + list.length + '</span></h3>' +
          '<p>' + esc(g.desc) + '</p>' +
        '</div>' +
        '<div class="cap-grid">' + list.map(function (it) {
          const wide = (it.slug === 'svparse') && list.length >= 3;
          const demo = it.category === 'data' || it.slug === 'svparse';
          return capCard(it, wide, demo);
        }).join('') + '</div></div>';
    });
    box.innerHTML = html || '<div class="empty">暂无可用能力</div>';
  }

  function renderPlatformChips() {
    const box = $('#platformChips');
    if (!box) return;
    box.innerHTML = CHIPS.map(function (c, i) {
      return '<button class="chip chip-sm' + (i === 0 ? ' on' : '') + '" data-idx="' + i + '">' + esc(c.label) + '</button>';
    }).join('');
    box.addEventListener('click', function (e) {
      const b = e.target.closest('.chip');
      if (!b) return;
      $$('#platformChips .chip').forEach(function (x) { x.classList.remove('on'); });
      b.classList.add('on');
      const c = CHIPS[Number(b.getAttribute('data-idx'))];
      if (!c) return;
      if (!c.demo) { toast('把「' + c.label + '」的分享链接粘进来即可'); $('#urlInput').focus(); return; }
      $('#urlInput').value = c.demo;
      $('#urlInput').focus();
    });
  }

  // ---------- 启动 ----------
  async function boot() {
    try {
      BOOT = await api.get('/api/public/bootstrap');
    } catch (e) {
      document.body.innerHTML = '<div style="padding:80px;text-align:center;color:#8b929c">服务暂不可用，请稍后刷新</div>';
      return;
    }
    const site = BOOT.site || {};
    renderHeader($('#header'), site, 'home', BOOT.me);
    renderFooter($('#footer'), site);
    renderAds(BOOT.ads);

    if (site.showAnnounce && site.announcement) {
      const a = $('#announce');
      a.className = 'announce';
      a.innerHTML = '<div class="announce-inner"><i class="dot dot-live"></i><span>' +
        esc(site.announcement) + '</span></div>';
    }
    if (site.desc) $('#heroSub').textContent = site.desc;

    CATS = BOOT.categories || [];
    try {
      const res = await api.get('/api/public/interfaces?limit=300');
      IFACES = res.items || [];
    } catch (e) { IFACES = []; }

    renderPlatformChips();
    renderCapabilities();

    const cnt = $('#capCount');
    if (cnt) cnt.textContent = String(IFACES.length);

    $('#btnParse').addEventListener('click', parse);
    $('#urlInput').addEventListener('keydown', function (e) { if (e.key === 'Enter') parse(); });
    $('#urlInput').addEventListener('paste', function () {
      setTimeout(function () {
        const v = $('#urlInput').value;
        if (v && /^https?:\/\//i.test(extractUrl(v))) parse();
      }, 60);
    });

    const q = new URLSearchParams(location.search).get('url');
    if (q) { $('#urlInput').value = q; parse(); }

    if (window.FX) window.FX.init();

    // 能力区是异步渲染的，锚点要等渲染完再定位（用瞬时跳转，避免平滑滚动被布局变化打断）
    if (location.hash) {
      const target = document.querySelector(location.hash);
      if (target) setTimeout(function () {
        target.scrollIntoView({ block: 'start' });
      }, 60);
    }
  }

  document.addEventListener('DOMContentLoaded', boot);
})();
