/* Shared runtime: api client, icons, shell, ads, helpers */
(function () {
  'use strict';

  const $ = (sel, root) => (root || document).querySelector(sel);
  const $$ = (sel, root) => Array.from((root || document).querySelectorAll(sel));

  const ICONS = {
    search: '<circle cx="11" cy="11" r="7"/><path d="m20 20-3.2-3.2"/>',
    chat: '<path d="M21 11.5a8.4 8.4 0 0 1-9 8.4 9.7 9.7 0 0 1-3.8-.8L3 21l1.9-4.6A8.4 8.4 0 0 1 12 3a8.4 8.4 0 0 1 9 8.5Z"/>',
    image: '<rect x="3" y="4" width="18" height="16" rx="2"/><circle cx="8.5" cy="9.5" r="1.5"/><path d="m21 15-4.5-4.5L7 20"/>',
    video: '<rect x="2.5" y="6" width="13" height="12" rx="2"/><path d="m15.5 10.5 6-3.5v10l-6-3.5"/>',
    audio: '<path d="M12 3v18"/><path d="M8 8v8"/><path d="M16 6v12"/><path d="M4 11v2"/><path d="M20 10v4"/>',
    eye: '<path d="M2.5 12S6 5.5 12 5.5 21.5 12 21.5 12 18 18.5 12 18.5 2.5 12 2.5 12Z"/><circle cx="12" cy="12" r="3"/>',
    tool: '<path d="M14.5 6.5a3.5 3.5 0 0 0 4.7 4.7L21 13l-8 8-2-2 1.8-1.8a3.5 3.5 0 0 0-4.7-4.7L6 14.3 3 11.3l8-8 3.5 3.2Z"/>',
    vector: '<circle cx="6" cy="6" r="2.5"/><circle cx="18" cy="18" r="2.5"/><path d="M8.5 6H15a3 3 0 0 1 3 3v6.5"/>',
    copy: '<rect x="9" y="9" width="12" height="12" rx="2"/><path d="M5 15V5a2 2 0 0 1 2-2h8"/>',
    arrow: '<path d="M5 12h14"/><path d="m13 6 6 6-6 6"/>',
    check: '<path d="m5 13 4.5 4.5L19 7"/>',
    x: '<path d="M6 6l12 12"/><path d="M18 6 6 18"/>',
    play: '<path d="M8 5.5v13l11-6.5Z"/>',
    trash: '<path d="M4 7h16"/><path d="M9 7V5h6v2"/><path d="M6 7l1 13h10l1-13"/>',
    download: '<path d="M12 3v12"/><path d="m7 11 5 5 5-5"/><path d="M4 20h16"/>',
    upload: '<path d="M12 17V5"/><path d="m7 9 5-5 5 5"/><path d="M4 20h16"/>',
    mic: '<rect x="9" y="3" width="6" height="11" rx="3"/><path d="M5 11a7 7 0 0 0 14 0"/><path d="M12 18v3"/>',
    edit: '<path d="M4 20h4l10-10-4-4L4 16v4Z"/><path d="m13.5 6.5 4 4"/>',
    plus: '<path d="M12 5v14"/><path d="M5 12h14"/>',
    grid: '<rect x="3" y="3" width="7" height="7" rx="1.5"/><rect x="14" y="3" width="7" height="7" rx="1.5"/><rect x="3" y="14" width="7" height="7" rx="1.5"/><rect x="14" y="14" width="7" height="7" rx="1.5"/>',
    list: '<path d="M8 6h13"/><path d="M8 12h13"/><path d="M8 18h13"/><path d="M3.5 6h.01"/><path d="M3.5 12h.01"/><path d="M3.5 18h.01"/>',
    settings: '<circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.7 1.7 0 0 0 .3 1.9l.1.1a2 2 0 1 1-2.8 2.8l-.1-.1a1.7 1.7 0 0 0-2.9 1.2 2 2 0 1 1-4 0 1.7 1.7 0 0 0-2.9-1.2l-.1.1a2 2 0 1 1-2.8-2.8l.1-.1A1.7 1.7 0 0 0 3 15a2 2 0 1 1 0-4 1.7 1.7 0 0 0 1.5-2.9l-.1-.1a2 2 0 1 1 2.8-2.8l.1.1A1.7 1.7 0 0 0 10 4a2 2 0 1 1 4 0 1.7 1.7 0 0 0 2.9 1.5l.1-.1a2 2 0 1 1 2.8 2.8l-.1.1A1.7 1.7 0 0 0 21 11a2 2 0 1 1 0 4Z"/>',
    megaphone: '<path d="M3 11v2a1 1 0 0 0 1 1h2l4 4V6L6 10H4a1 1 0 0 0-1 1Z"/><path d="M14 8a4 4 0 0 1 0 8"/><path d="M17 5a8 8 0 0 1 0 14"/>',
    users: '<circle cx="9" cy="8" r="3.5"/><path d="M3 20c0-3.3 2.7-5.5 6-5.5s6 2.2 6 5.5"/><path d="M16 5.2a3.5 3.5 0 0 1 0 6.6"/><path d="M18 14.5c2.3.6 3.8 2.4 3.8 4.8"/>',
    log: '<path d="M4 5h16"/><path d="M4 10h16"/><path d="M4 15h10"/><path d="M4 20h7"/>',
    db: '<ellipse cx="12" cy="6" rx="8" ry="3"/><path d="M4 6v12c0 1.7 3.6 3 8 3s8-1.3 8-3V6"/><path d="M4 12c0 1.7 3.6 3 8 3s8-1.3 8-3"/>',
    logout: '<path d="M15 17l5-5-5-5"/><path d="M20 12H9"/><path d="M12 4H5a1 1 0 0 0-1 1v14a1 1 0 0 0 1 1h7"/>',
    shield: '<path d="M12 3 5 6v6c0 4.5 3 8 7 9 4-1 7-4.5 7-9V6l-7-3Z"/>',
    bolt: '<path d="M13 3 5 14h6l-1 7 8-11h-6l1-7Z"/>',
    gauge: '<path d="M12 14a2 2 0 1 0 0-4 2 2 0 0 0 0 4Z"/><path d="M13.4 10.6 17 7"/><path d="M20.5 17a9 9 0 1 0-17 0"/>',
    link: '<path d="M10 13a4 4 0 0 0 5.7 0l2.6-2.6A4 4 0 0 0 12.6 4.7L11 6.3"/><path d="M14 11a4 4 0 0 0-5.7 0L5.7 13.6a4 4 0 0 0 5.7 5.7l1.6-1.6"/>',
    info: '<circle cx="12" cy="12" r="9"/><path d="M12 11v6"/><path d="M12 7.5h.01"/>',
    alert: '<path d="M12 4 2.5 20h19L12 4Z"/><path d="M12 10v4"/><path d="M12 17.5h.01"/>',
    folder: '<path d="M3 7a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V7Z"/>',
    wave: '<path d="M4 12h2"/><path d="M8 8v8"/><path d="M12 5v14"/><path d="M16 9v6"/><path d="M20 11v2"/>'
  };

  function icon(name, size) {
    const p = ICONS[name] || ICONS.info;
    const s = size || 16;
    return '<svg width="' + s + '" height="' + s + '" viewBox="0 0 24 24" fill="none" ' +
      'stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round" ' +
      'aria-hidden="true">' + p + '</svg>';
  }

  function esc(s) {
    return String(s == null ? '' : s).replace(/[&<>"']/g, function (c) {
      return { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c];
    });
  }

  // ---------- toast ----------
  let toastWrap = null;
  function toast(msg, type, ms) {
    if (!toastWrap) {
      toastWrap = document.createElement('div');
      toastWrap.className = 'toast-wrap';
      document.body.appendChild(toastWrap);
    }
    const el = document.createElement('div');
    el.className = 'toast ' + (type === 'err' ? 'err' : type === 'ok' ? 'ok' : type === 'warn' ? 'warn' : '');
    const mark = type === 'err' ? '!' : type === 'ok' ? '✓' : type === 'warn' ? '⚠' : 'i';
    el.innerHTML = '<span class="i">' + mark + '</span><span>' + esc(msg) + '</span>';
    toastWrap.appendChild(el);
    setTimeout(function () {
      el.style.transition = 'opacity .2s, transform .2s';
      el.style.opacity = '0';
      el.style.transform = 'translateY(6px)';
      setTimeout(function () { el.remove(); }, 220);
    }, ms || (type === 'err' ? 4200 : 2600));
  }

  // ---------- api ----------
  async function request(method, url, body, opts) {
    const init = { method: method, headers: {}, credentials: 'same-origin' };
    if (body !== undefined && body !== null) {
      init.headers['Content-Type'] = 'application/json';
      init.body = JSON.stringify(body);
    }
    if (opts && opts.headers) Object.assign(init.headers, opts.headers);
    let res;
    try {
      res = await fetch(url, init);
    } catch (e) {
      throw new Error('网络连接失败，请检查服务是否运行');
    }
    const ct = res.headers.get('Content-Type') || '';
    let data = null;
    if (ct.indexOf('application/json') >= 0) {
      try { data = await res.json(); } catch (e) { data = null; }
    } else {
      data = await res.text();
    }
    if (!res.ok) {
      const msg = (data && (data.message || data.error)) || ('请求失败 (' + res.status + ')');
      const err = new Error(msg);
      err.status = res.status;
      err.data = data;
      throw err;
    }
    return data;
  }

  const api = {
    get: (u, o) => request('GET', u, null, o),
    post: (u, b, o) => request('POST', u, b === undefined ? {} : b, o),
    put: (u, b, o) => request('PUT', u, b, o),
    patch: (u, b, o) => request('PATCH', u, b, o),
    del: (u, o) => request('DELETE', u, null, o)
  };

  // ---------- format ----------
  function money(n) {
    if (!n) return '0';
    if (n < 0.001) return String(n.toFixed(5)).replace(/0+$/, '');
    if (n < 0.01) return n.toFixed(4);
    if (n < 1) return n.toFixed(3);
    return n.toFixed(2);
  }
  function priceText(it) {
    if (it.pricingMode === 'paid') return '¥' + money(it.price) + ' / ' + (it.priceUnit || '次');
    return '免费';
  }
  function bytes(n) {
    if (!n) return '0 B';
    if (n < 1024) return n + ' B';
    if (n < 1024 * 1024) return (n / 1024).toFixed(0) + ' KB';
    return (n / 1024 / 1024).toFixed(2) + ' MB';
  }
  function ms(n) {
    if (!n) return '—';
    if (n < 1000) return n + 'ms';
    return (n / 1000).toFixed(1) + 's';
  }
  function dur(msv) {
    if (!msv) return '—';
    const s = Math.round(msv / 1000);
    return Math.floor(s / 60) + ':' + String(s % 60).padStart(2, '0');
  }
  function timeAgo(iso) {
    if (!iso) return '—';
    const t = new Date(iso).getTime();
    const d = Date.now() - t;
    if (d < 60000) return '刚刚';
    if (d < 3600000) return Math.floor(d / 60000) + ' 分钟前';
    if (d < 86400000) return Math.floor(d / 3600000) + ' 小时前';
    if (d < 86400000 * 30) return Math.floor(d / 86400000) + ' 天前';
    return new Date(iso).toLocaleDateString('zh-CN');
  }

  // ---------- copy ----------
  async function copy(text, okMsg) {
    try {
      if (navigator.clipboard && window.isSecureContext) {
        await navigator.clipboard.writeText(text);
      } else {
        const ta = document.createElement('textarea');
        ta.value = text;
        ta.style.position = 'fixed';
        ta.style.opacity = '0';
        document.body.appendChild(ta);
        ta.select();
        document.execCommand('copy');
        ta.remove();
      }
      toast(okMsg || '已复制到剪贴板', 'ok');
      return true;
    } catch (e) {
      toast('复制失败，请手动选择文本', 'err');
      return false;
    }
  }

  // ---------- code highlight ----------
  function highlightJSON(raw) {
    if (!raw) return '';
    let s = esc(raw);
    s = s.replace(/(&quot;(?:[^&\\]|\\.|&(?!quot;))*?&quot;)(\s*:)/g, '<span class="k">$1</span>$2');
    s = s.replace(/:\s*(&quot;(?:[^&\\]|\\.|&(?!quot;))*?&quot;)/g, ': <span class="s">$1</span>');
    s = s.replace(/\b(true|false|null)\b/g, '<span class="n">$1</span>');
    s = s.replace(/(^|[\s,\[:(])(-?\d+(?:\.\d+)?)(?=[\s,\]}])/g, '$1<span class="n">$2</span>');
    return s;
  }

  function highlightCode(raw, lang) {
    if (lang === 'json') return highlightJSON(raw);
    let s = esc(raw);
    s = s.replace(/(^|\n)(\s*)(#|\/\/)([^\n]*)/g, '$1$2<span class="c">$3$4</span>');
    s = s.replace(/(&#39;[^&\n]*?&#39;|&quot;[^&\n]*?&quot;)/g, '<span class="s">$1</span>');
    s = s.replace(/\b(curl|python|import|from|const|let|await|fetch|async|package|func|return|echo|new|def|print|with|as)\b/g,
      '<span class="k">$1</span>');
    s = s.replace(/(^|\s)(-{1,2}[a-zA-Z][\w-]*)/g, '$1<span class="b">$2</span>');
    return s;
  }

  // ---------- shell ----------
  function brandHTML(site, href) {
    return '<a class="brand" href="' + (href || '/') + '">' +
      '<span class="brand-mark">' + esc(site.logoText || '清') + '</span>' +
      '<span class="brand-name">' + esc(site.name || '清影') + '</span></a>';
  }

  function renderHeader(el, site, active, me) {
    if (!el) return;
    const nav = [['/#capabilities', '能力', 'caps']];
    // 「我的作者」只对登录用户展示：关注与作品快照按账号保存
    if (me) nav.push(['/authors', '我的作者', 'authors']);
    nav.push(['/tts', '音色工坊', 'tts'], ['/#about', '关于', 'about']);
    const navHTML = nav.map(function (n) {
      return '<a href="' + n[0] + '" class="' + (active === n[2] ? 'active' : '') + '">' + n[1] + '</a>';
    }).join('');

    // 右侧：同一个登录入口，登录后按角色渲染——普通用户只看到自己，
    // 管理员额外多一个「后台」按钮；未登录统一是「登录 / 注册」。
    // 文字包一层 .btn-label，小屏下可单独省略/截断，图标保留
    let right;
    if (!me) {
      right = '<a class="btn btn-ghost btn-sm js-login" href="/console">' +
        icon('shield', 13) + ' <span class="btn-label">登录 / 注册</span></a>' +
        '<a class="btn btn-ghost btn-sm hide-xs" href="/#top">' + icon('bolt', 13) +
        ' <span class="btn-label">开始解析</span></a>';
    } else if (me.role === 'admin') {
      right = '<a class="btn btn-primary btn-sm js-admin" href="/admin">' + icon('settings', 13) +
        ' <span class="btn-label">后台</span></a>' +
        '<a class="btn btn-ghost btn-sm js-user" href="/console">' + icon('shield', 13) +
        ' <span class="btn-label">' + esc(me.username || 'admin') + '</span></a>';
    } else {
      right = '<a class="btn btn-ghost btn-sm js-user" href="/console">' + icon('shield', 13) +
        ' <span class="btn-label">' + esc(me.username || '我的') + '</span></a>';
    }

    el.className = 'topbar';
    el.innerHTML =
      '<div class="topbar-inner">' +
      brandHTML(site) +
      '<nav class="nav">' + navHTML + '</nav>' +
      '<div class="topbar-right">' +
      '<span class="pill"><i class="dot dot-live"></i>服务正常</span>' +
      right +
      '</div></div>';
  }

  function renderFooter(el, site) {
    if (!el) return;
    el.className = 'footer';
    el.innerHTML =
      '<div class="footer-inner">' +
      '<div class="footer-brand">' + brandHTML(site) +
      '<p>为个人工作流定制的安静工具台 —— 解析、洞察、生成，皆在一处。</p>' +
      '<p class="footer-fine">部分能力来自第三方平台，仅供个人学习与研究，请勿用于商业用途。</p></div>' +
      '<div class="footer-cols">' +
      '<div class="footer-col"><h4>能力</h4>' +
      '<a href="/#capabilities">能力清单</a><a href="/tts">音色工坊</a><a href="/#top">开始解析</a></div>' +
      '<div class="footer-col"><h4>说明</h4>' +
      '<a href="/#about">使用须知</a><a href="/#about">免责声明</a>' +
      '<a href="/api/public/health" target="_blank" rel="noopener">服务状态</a></div>' +
      '<div class="footer-col"><h4>其他</h4>' +
      '<a href="/console">个人中心</a>' +
      '<a href="/robots.txt" target="_blank" rel="noopener">robots.txt</a></div>' +
      '</div></div>' +
      '<div class="footer-bottom"><span>' + esc(site.footerText || '') + '</span>' +
      '<span>' + esc(site.icp || '') + '</span></div>';
  }

  function renderAds(ads) {
    if (!ads) return;
    $$('[data-ad]').forEach(function (el) {
      const slot = el.getAttribute('data-ad');
      if (ads[slot]) {
        el.innerHTML = ads[slot];
        el.classList.remove('hidden');
      }
    });
  }

  // ---------- misc ----------
  function debounce(fn, wait) {
    let t;
    return function () {
      const args = arguments, ctx = this;
      clearTimeout(t);
      t = setTimeout(function () { fn.apply(ctx, args); }, wait || 200);
    };
  }

  function query(name) {
    return new URLSearchParams(location.search).get(name) || '';
  }

  window.UI = {
    $: $, $$: $$, icon: icon, esc: esc, toast: toast, api: api,
    money: money, priceText: priceText, bytes: bytes, ms: ms, dur: dur, timeAgo: timeAgo,
    copy: copy, highlightJSON: highlightJSON, highlightCode: highlightCode,
    renderHeader: renderHeader, renderFooter: renderFooter, renderAds: renderAds,
    brandHTML: brandHTML, debounce: debounce, query: query
  };
})();
