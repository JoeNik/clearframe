/* 能力页：单一能力的参数表单与结果展示 */
(function () {
  'use strict';
  const { $, $$, icon, esc, api, renderHeader, renderFooter, renderAds, toast } = window.UI;

  let IT = null;      // 接口详情
  let BUSY = false;
  const slug = (location.pathname.split('/').filter(Boolean).pop() || '').replace(/[?#].*$/, '');

  const ICON_OF = {
    dyjx: 'video', svparse: 'grid', wxsph: 'video', zjyh: 'video',
    jimeng: 'video', doubao: 'video', ksjx: 'video', bilibili: 'video',
    dydj: 'play', dytj: 'gauge', bbhj: 'users', dola: 'video',
    pipigx: 'video', domailcx: 'link', dyzz: 'users'
  };

  // 上游访问密钥由服务端注入，前台不展示
  function visibleParams() {
    return (IT.params || []).filter(function (p) {
      return !(p.name === 'key' && /密钥|access key/i.test(p.desc || ''));
    });
  }

  function renderForm() {
    const form = $('#capForm');
    const ps = visibleParams();
    if (!ps.length) {
      form.innerHTML = '<div class="cap-noparam">' + icon('check', 16) +
        ' 这项能力不需要任何参数，直接点「开始执行」即可。</div>';
      return;
    }
    form.innerHTML = ps.map(function (p, i) {
      const id = 'p_' + p.name;
      return '<div class="field">' +
        '<label for="' + id + '">' + esc(p.name) +
          (p.required ? '<span class="req">*</span>' : '<span class="opt">选填</span>') +
        '</label>' +
        '<input class="input" id="' + id + '" data-param="' + esc(p.name) + '" ' +
          'data-required="' + (p.required ? '1' : '0') + '" ' +
          'placeholder="' + esc(p.default || (p.type === 'int' ? '数字' : '请输入')) + '" ' +
          (p.type === 'int' ? 'inputmode="numeric" ' : '') +
          'value="' + (i === 0 && p.default ? esc(p.default) : '') + '">' +
        (p.desc ? '<div class="pdef">' + esc(p.desc) + '</div>' : '') +
      '</div>';
    }).join('');
  }

  function collect() {
    const out = {};
    const ps = visibleParams();
    for (const p of ps) {
      const el = document.querySelector('[data-param="' + p.name + '"]');
      const v = el ? el.value.trim() : '';
      if (!v) {
        if (p.required) { el.focus(); toast('请填写「' + p.name + '」'); return null; }
        continue;
      }
      out[p.name] = v;
    }
    if (!Object.keys(out).length) {
      const optional = ps.filter(function (p) { return p.default; })[0];
      if (optional) {
        const el = document.querySelector('[data-param="' + optional.name + '"]');
        if (el) el.value = optional.default;
        out[optional.name] = optional.default;
      } else {
        toast('至少填写一个参数');
        return null;
      }
    }
    return out;
  }

  async function run() {
    if (BUSY || !IT) return;
    const vals = collect();
    if (!vals) return;

    const btn = $('#capRun');
    const st = $('#capStatus');
    const wrap = $('#capResult');
    BUSY = true;
    btn.disabled = true;
    btn.textContent = '执行中…';
    wrap.innerHTML = '';
    st.innerHTML = '<div class="parse-loading"><span class="spinner"></span><span>正在执行…</span></div>';

    const isPost = (IT.method || 'GET').toUpperCase() === 'POST';
    const url = isPost ? '/v1/' + IT.slug : '/v1/' + IT.slug + '?' + new URLSearchParams(vals).toString();

    try {
      const r = await fetch(url, {
        method: isPost ? 'POST' : 'GET',
        headers: isPost
          ? { 'Content-Type': 'application/json', 'Accept': 'application/json' }
          : { 'Accept': 'application/json' },
        body: isPost ? JSON.stringify(vals) : undefined
      });
      const text = await r.text();
      let data = null;
      try { data = JSON.parse(text); } catch (e) { /* 非 JSON */ }

      const rem = r.headers.get('X-Guest-Remaining');
      if (rem !== null) renderQuota(rem);

      const pickMsg = function () {
        if (!data) return '';
        if (typeof data.error === 'string' && data.error) return data.error;
        if (data.error && typeof data.error === 'object') return data.error.message || data.error.code || '';
        return data.message || data.msg || '';
      };

      if (!r.ok) { fail(pickMsg() || ('接口返回 HTTP ' + r.status), r.status); return; }
      if (data && typeof data.code !== 'undefined' && Number(data.code) !== 200) {
        fail(pickMsg() || '执行失败', data.code); return;
      }
      const payload = data && typeof data.data !== 'undefined' ? data.data : data;
      if (payload == null || (Array.isArray(payload) && !payload.length) || payload === '') {
        fail('没有返回内容，可能是参数不匹配或数据不存在', 200); return;
      }
      st.innerHTML = '';
      window.RENDER.show(wrap, Array.isArray(payload) ? payload[0] : payload, {
        sourceLabel: IT.name + ' · 执行完成'
      });
      wrap.scrollIntoView({ behavior: 'smooth', block: 'start' });
    } catch (e) {
      fail('网络异常：' + e.message, 0);
    } finally {
      BUSY = false;
      btn.disabled = false;
      btn.textContent = '开始执行';
    }
  }

  function fail(msg, code) {
    $('#capStatus').innerHTML = '';
    const rate = Number(code) === 429 || /频繁|限流|稍后/.test(String(msg));
    $('#capResult').innerHTML =
      '<div class="result-card err"><div class="err-ico">' + icon('alert', 22) + '</div>' +
      '<div><div class="err-title">' + (rate ? '上游临时限流' : '执行失败') + '</div>' +
      '<div class="err-msg">' + esc(msg) + '</div>' +
      '<div class="err-hint">' + (rate
        ? '等 1 分钟左右再点一次「开始执行」即可，不用改参数；刚查过的内容 60 秒内会直接命中缓存。'
        : (code ? '状态码 ' + esc(String(code)) + '　' : '') + '换个参数试试，或稍后重试。') +
      '</div></div></div>';
  }

  function renderQuota(rem) {
    const el = $('#capQuota');
    if (!el) return;
    const n = Number(rem);
    if (isNaN(n)) return;
    el.innerHTML = n > 0 ? '今日剩余 <b>' + n + '</b> 次' : '今日次数已用完';
  }

  function renderDesc() {
    const box = $('#capDesc');
    const raw = (IT.description || '').trim();
    if (!raw) { box.innerHTML = '<p class="muted">暂无详细说明。</p>'; return; }
    box.innerHTML = raw.split(/\n{2,}/).map(function (p) {
      return '<p>' + esc(p) + '</p>';
    }).join('');
  }

  async function boot() {
    let BOOT = null;
    try { BOOT = await api.get('/api/public/bootstrap'); } catch (e) { /* 忽略 */ }
    const site = (BOOT && BOOT.site) || {};
    renderHeader($('#header'), site, 'caps', BOOT && BOOT.me);
    renderFooter($('#footer'), site);
    if (BOOT) renderAds(BOOT.ads);

    try {
      const d = await api.get('/api/public/interfaces/' + encodeURIComponent(slug));
      IT = d.interface;
    } catch (e) {
      $('#capTitle').textContent = '能力不存在';
      $('#capSummary').textContent = '这项能力可能已下线，或者链接输错了。';
      $('#capForm').innerHTML = '<div class="cap-noparam">' + icon('alert', 16) +
        ' <a href="/#capabilities">返回能力清单</a></div>';
      $('#capRun').disabled = true;
      return;
    }

    document.title = IT.name + ' · ' + (site.name || '清影 ClearFrame');
    $('#crumbName').textContent = IT.name;
    $('#capTitle').textContent = IT.name;
    $('#capSummary').textContent = IT.summary || '';
    $('#capIco').innerHTML = icon(ICON_OF[IT.slug] || 'bolt', 22);
    $('#capKind').textContent = IT.category === 'data' ? '数据分析' : IT.category === 'tool' ? '实用工具' : '视频解析';

    const tags = [];
    // 计费标签：跟随上游同步状态，不硬编码
    tags.push(IT.pricingMode === 'paid'
      ? '<span class="tag tag-warn">¥' + (Number(IT.price) || 0) + ' / ' + esc(IT.priceUnit || '次') + '</span>'
      : '<span class="tag tag-accent">免费</span>');
    tags.push('<span class="tag">' + (IT.category === 'data' ? '数据分析' : IT.category === 'tool' ? '实用工具' : '视频解析') + '</span>');
    if (IT.requiresLogin) tags.push('<span class="tag">需登录</span>');
    if (IT.popularity) tags.push('<span class="tag">' + (IT.popularity / 10).toFixed(0) + '% 热度</span>');
    $('#capTags').innerHTML = tags.join('');

    renderForm();
    renderDesc();

    // 支持深链接：/cap/dyjx?url=xxx 自动填参并执行
    const sp = new URLSearchParams(location.search);
    let filled = false;
    visibleParams().forEach(function (p) {
      const v = sp.get(p.name);
      if (!v) return;
      const el = document.querySelector('[data-param="' + p.name + '"]');
      if (el) { el.value = v; filled = true; }
    });
    if (filled) setTimeout(run, 150);

    $('#capRun').addEventListener('click', run);
    $('#capReset').addEventListener('click', function () {
      $$('#capForm input').forEach(function (el) { el.value = ''; });
      $('#capResult').innerHTML = '';
      $('#capStatus').innerHTML = '';
    });
    $('#capForm').addEventListener('submit', function (e) { e.preventDefault(); run(); });
    $('#capForm').addEventListener('keydown', function (e) {
      if (e.key === 'Enter' && e.target.tagName === 'INPUT') { e.preventDefault(); run(); }
    });

    if (window.FX) window.FX.init();
  }

  document.addEventListener('DOMContentLoaded', boot);
})();
