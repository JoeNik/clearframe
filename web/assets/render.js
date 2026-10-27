/* 结果渲染器：媒体 / 数据分析 / 通用 JSON
   供首页解析与能力页共用。window.RENDER.show(el, data, opts) 一把梭。 */
(function () {
  'use strict';
  const { esc, icon, copy, toast, bytes } = window.UI;

  // ---------- 数值 ----------
  function num(n) {
    const v = Number(n);
    if (!isFinite(v)) return '—';
    const a = Math.abs(v);
    if (a >= 1e8) return (v / 1e8).toFixed(a >= 1e9 ? 0 : 1).replace(/\.0$/, '') + '亿';
    if (a >= 1e4) return (v / 1e4).toFixed(a >= 1e5 ? 0 : 1).replace(/\.0$/, '') + '万';
    return String(Math.round(v * 100) / 100);
  }
  function pct(v, digits) {
    const n = Number(v);
    if (!isFinite(n)) return '—';
    return (n * 100).toFixed(digits == null ? 2 : digits) + '%';
  }
  function fmtDur(sec) {
    if (!sec) return '';
    const s = Math.round(sec);
    return Math.floor(s / 60) + ':' + String(s % 60).padStart(2, '0');
  }

  // ---------- 媒体 ----------
  function mediaHTML(m, opts) {
    opts = opts || {};
    const type = m.type || (m.images && m.images.length ? 'image' : 'video');
    const author = m.author || {};
    const title = m.title || m.desc || '未命名作品';
    const tags = [];
    if (m.quality) tags.push('<span class="tag tag-accent">' + esc(m.quality) + '</span>');
    if (m.duration) tags.push('<span class="tag">' + fmtDur(m.duration) + '</span>');
    if (m.size_label || m.size) tags.push('<span class="tag">' + esc(m.size_label || bytes(m.size)) + '</span>');
    tags.push('<span class="tag">' + (type === 'image' ? '图集' : type === 'live' ? '实况' : '视频') + '</span>');
    if (opts.note) tags.push('<span class="tag tag-accent">' + esc(opts.note) + '</span>');

    let body = '';
    if (type === 'image' && Array.isArray(m.images) && m.images.length) {
      body = '<div class="media-grid">' + m.images.map(function (u, i) {
        const src = typeof u === 'string' ? u : (u.url || '');
        return '<a class="media-item" href="' + esc(src) + '" target="_blank" rel="noopener">' +
          '<img src="' + esc(src) + '" loading="lazy" alt="图 ' + (i + 1) + '">' +
          '<span class="media-idx">' + (i + 1) + '</span></a>';
      }).join('') + '</div>';
    } else if (m.url) {
      body = '<div class="media-player">' +
        (m.cover ? '<img class="media-cover" src="' + esc(m.cover) + '" alt="">' : '') +
        '<video class="media-video" src="' + esc(m.url) + '" controls playsinline preload="metadata"' +
        (m.cover ? ' poster="' + esc(m.cover) + '"' : '') + '></video></div>';
    } else if (m.cover) {
      body = '<div class="media-player"><img class="media-cover solo" src="' + esc(m.cover) + '" alt=""></div>';
    }

    const backups = (m.video_backup || []).filter(function (b) { return b && b.url; });
    const actions = [];
    if (m.url) {
      actions.push('<a class="btn btn-primary" href="' + esc(m.url) + '" target="_blank" rel="noopener" download>' +
        icon('download', 15) + ' 下载无水印视频</a>');
    }
    if (Array.isArray(m.images) && m.images.length) {
      actions.push('<button class="btn btn-primary" data-act="open-all">' + icon('download', 15) +
        ' 打开全部图片（' + m.images.length + '）</button>');
    }
    if (m.url || (m.images && m.images.length)) {
      actions.push('<button class="btn btn-ghost" data-act="copy">' + icon('copy', 15) + ' 复制直链</button>');
    }

    let backupHTML = '';
    if (backups.length) {
      backupHTML = '<div class="backup-row"><span class="backup-label">备用画质</span>' +
        backups.map(function (b) {
          return '<a class="btn btn-ghost btn-sm" href="' + esc(b.url) + '" target="_blank" rel="noopener">' +
            esc(b.label || b.quality || '备用') + (b.size_label ? ' · ' + esc(b.size_label) : '') + '</a>';
        }).join('') + '</div>';
    }
    const music = m.music && (m.music.url || m.music.play_url);
    const musicHTML = music
      ? '<div class="backup-row"><span class="backup-label">背景音乐</span>' +
        '<a class="btn btn-ghost btn-sm" href="' + esc(m.music.url || m.music.play_url) + '" target="_blank" rel="noopener">' +
        icon('audio', 13) + ' ' + esc(m.music.title || m.music.name || '下载音频') + '</a></div>'
      : '';

    // 作品数据：媒体结果里若带上统计，就顺手展示出来
    const st = (m.extra && m.extra.statistics) || null;
    const statHTML = st ? '<div class="mini-stats">' + [
      ['play_count', '播放'], ['digg_count', '点赞'], ['comment_count', '评论'],
      ['share_count', '分享'], ['collect_count', '收藏']
    ].filter(function (p) { return st[p[0]] != null; }).map(function (p) {
      return '<div class="mini-stat"><b>' + num(st[p[0]]) + '</b><span>' + p[1] + '</span></div>';
    }).join('') + (st.interact_rate != null
      ? '<div class="mini-stat"><b>' + pct(st.interact_rate, 1) + '</b><span>互动率</span></div>' : '') + '</div>' : '';

    return '<div class="result-card">' +
      '<div class="result-head">' +
        '<div class="result-title">' + esc(title) + '</div>' +
        '<div class="result-tags">' + tags.join('') + '</div>' +
      '</div>' + body + statHTML +
      '<div class="result-foot">' +
        '<div class="result-author">' +
          (author.avatar ? '<img class="avatar" src="' + esc(author.avatar) + '" alt="">' : '<span class="avatar ph"></span>') +
          '<div><div class="an">' + esc(author.name || '未知作者') + '</div>' +
          '<div class="ai">' + esc(opts.sourceLabel || '解析完成') + '</div></div>' +
        '</div>' +
        '<div class="result-actions">' + actions.join('') + '</div>' +
      '</div>' + backupHTML + musicHTML + '</div>';
  }

  // ---------- 折线图 ----------
  function trendSVG(daily, key, label) {
    const pts = (daily || []).map(function (d) { return Number(d[key]) || 0; });
    if (pts.length < 2) return '';
    const W = 640, H = 150, PADL = 8, PADR = 8, PADT = 14, PADB = 22;
    const max = Math.max.apply(null, pts) || 1;
    const min = Math.min.apply(null, pts);
    const span = max - min || 1;
    const x = function (i) { return PADL + (W - PADL - PADR) * (i / (pts.length - 1)); };
    const y = function (v) { return PADT + (H - PADT - PADB) * (1 - (v - min) / span); };
    const line = pts.map(function (v, i) { return (i ? 'L' : 'M') + x(i).toFixed(1) + ' ' + y(v).toFixed(1); }).join(' ');
    const area = line + ' L' + x(pts.length - 1).toFixed(1) + ' ' + (H - PADB) + ' L' + x(0).toFixed(1) + ' ' + (H - PADB) + ' Z';
    const peak = pts.indexOf(Math.max.apply(null, pts));
    const last = pts.length - 1;
    const gid = 'g' + Math.random().toString(36).slice(2, 8);

    return '<div class="trend">' +
      '<div class="trend-head"><span class="trend-title">' + esc(label || '趋势') + '</span>' +
      '<span class="trend-meta">峰值 <b>' + num(max) + '</b> · 最新 <b>' + num(pts[last]) + '</b></span></div>' +
      '<svg class="trend-svg" viewBox="0 0 ' + W + ' ' + H + '" preserveAspectRatio="none" role="img">' +
        '<defs><linearGradient id="' + gid + '" x1="0" y1="0" x2="0" y2="1">' +
          '<stop offset="0%" stop-color="var(--accent)" stop-opacity=".28"/>' +
          '<stop offset="100%" stop-color="var(--accent)" stop-opacity="0"/>' +
        '</linearGradient></defs>' +
        '<path d="' + area + '" fill="url(#' + gid + ')"/>' +
        '<path class="trend-line" d="' + line + '" fill="none" stroke="var(--accent)" stroke-width="2" ' +
          'stroke-linejoin="round" stroke-linecap="round" vector-effect="non-scaling-stroke"/>' +
        '<circle cx="' + x(peak).toFixed(1) + '" cy="' + y(pts[peak]).toFixed(1) + '" r="3.5" fill="var(--accent)"/>' +
        '<circle cx="' + x(last).toFixed(1) + '" cy="' + y(pts[last]).toFixed(1) + '" r="3.5" fill="var(--accent)" ' +
          'stroke="var(--bg)" stroke-width="2"/>' +
      '</svg>' +
      '<div class="trend-axis"><span>' + esc((daily[0] || {}).date || '') + '</span>' +
      '<span>' + esc((daily[last] || {}).date || '') + '</span></div>' +
    '</div>';
  }

  // ---------- 数据分析 ----------
  function statsHTML(data) {
    const extra = data.extra || data;
    const st = extra.statistics || {};
    const trend = extra.statistics_trend || {};
    const daily = trend.daily || [];
    const follower = (extra.author_extra || {}).follower_count;

    const cards = [
      { k: 'play_count', label: '播放量', ico: 'play' },
      { k: 'digg_count', label: '点赞', ico: 'bolt' },
      { k: 'comment_count', label: '评论', ico: 'chat' },
      { k: 'share_count', label: '分享', ico: 'link' },
      { k: 'collect_count', label: '收藏', ico: 'check' }
    ].filter(function (c) { return st[c.k] != null; });

    let html = '<div class="result-card stats-card">' +
      '<div class="result-head"><div class="result-title">作品数据洞察</div>' +
      '<div class="result-tags"><span class="tag tag-accent">数据分析</span>' +
      (trend.days ? '<span class="tag">近 ' + trend.days + ' 天</span>' : '') +
      (extra.aweme_id ? '<span class="tag mono">ID ' + esc(extra.aweme_id) + '</span>' : '') +
      '</div></div>';

    html += '<div class="metric-grid">' + cards.map(function (c) {
      return '<div class="metric"><span class="metric-ico">' + icon(c.ico, 14) + '</span>' +
        '<div class="metric-val">' + num(st[c.k]) + '</div>' +
        '<div class="metric-label">' + c.label + '</div></div>';
    }).join('') + '</div>';

    const rings = [];
    if (st.interact_rate != null) rings.push({ label: '互动率', v: Number(st.interact_rate) });
    if (follower != null) rings.push({ label: '粉丝', v: null, text: num(follower) });
    const lastDay = daily.length ? daily[daily.length - 1] : null;
    if (lastDay && lastDay.finish_rate != null) rings.push({ label: '完播率', v: Number(lastDay.finish_rate) });
    if (lastDay && lastDay.hot_score != null) rings.push({ label: '热度', v: null, text: num(lastDay.hot_score) });

    if (rings.length) {
      html += '<div class="ring-row">' + rings.map(function (r) {
        if (r.v == null) {
          return '<div class="ring"><div class="ring-val">' + r.text + '</div>' +
            '<div class="ring-label">' + r.label + '</div></div>';
        }
        const p = Math.max(0, Math.min(1, r.v));
        const deg = (p * 360).toFixed(1);
        return '<div class="ring"><div class="ring-dial" style="--deg:' + deg + 'deg">' +
          '<span>' + pct(r.v, p < 0.1 ? 2 : 1) + '</span></div>' +
          '<div class="ring-label">' + r.label + '</div></div>';
      }).join('') + '</div>';
    }

    if (daily.length > 1) {
      html += trendSVG(daily, 'play_count', '每日播放量');
      html += trendSVG(daily, 'digg_count', '每日点赞');
    } else {
      // 上游未返回逐日趋势时，用真实计数画一张互动构成图，不编造数据
      const parts = [
        { k: 'digg_count', label: '点赞' },
        { k: 'comment_count', label: '评论' },
        { k: 'share_count', label: '分享' },
        { k: 'collect_count', label: '收藏' }
      ].filter(function (p) { return Number(st[p.k]) > 0; });
      if (parts.length) {
        const sum = parts.reduce(function (a, p) { return a + Number(st[p.k]); }, 0);
        const max = Math.max.apply(null, parts.map(function (p) { return Number(st[p.k]); }));
        html += '<div class="trend"><div class="trend-head">' +
          '<span class="trend-title">互动构成</span>' +
          '<span class="trend-meta">合计 <b>' + num(sum) + '</b></span></div>' +
          '<div class="bar-list">' + parts.map(function (p) {
            const v = Number(st[p.k]);
            return '<div class="bar-row"><span class="bar-k">' + p.label + '</span>' +
              '<span class="bar-track"><i style="width:' + ((v / max) * 100).toFixed(1) + '%"></i></span>' +
              '<span class="bar-v">' + num(v) + ' · ' + pct(v / sum, 1) + '</span></div>';
          }).join('') + '</div></div>';
      }
    }
    html += '</div>';
    return html;
  }

  // ---------- 通用 ----------
  function genericHTML(data) {
    if (data && typeof data === 'object' && !Array.isArray(data)) {
      const keys = Object.keys(data);
      const flat = keys.filter(function (k) {
        const v = data[k];
        return v == null || typeof v !== 'object';
      });
      if (flat.length >= 3 && flat.length >= keys.length - 2) {
        return '<div class="result-card stats-card">' +
          '<div class="result-head"><div class="result-title">查询结果</div>' +
          '<div class="result-tags"><span class="tag tag-accent">结构化数据</span></div></div>' +
          '<div class="kv-grid">' + flat.map(function (k) {
            const v = data[k];
            return '<div class="kv"><span class="kv-k">' + esc(k) + '</span>' +
              '<span class="kv-v">' + esc(v === '' || v == null ? '—' : v) + '</span></div>';
          }).join('') + '</div></div>';
      }
    }
    const pretty = JSON.stringify(data, null, 2);
    return '<div class="result-card json-card">' +
      '<div class="result-head"><div class="result-title">原始返回</div>' +
      '<div class="result-tags"><span class="tag">JSON</span></div></div>' +
      '<pre class="json-pre">' + window.UI.highlightJSON(pretty) + '</pre></div>';
  }

  function detect(data) {
    if (!data || typeof data !== 'object') return 'generic';
    // 先判媒体：媒体对象里也可能带 extra.statistics，不能被误判成统计页
    if (data.url || data.cover || (data.images && data.images.length) || data.type) return 'media';
    if (data.extra && (data.extra.statistics || data.extra.statistics_trend)) return 'stats';
    return 'generic';
  }

  // ---------- 入口 ----------
  function show(el, data, opts) {
    if (!el) return;
    opts = opts || {};
    const kind = opts.kind || detect(data);
    let html;
    if (kind === 'stats') html = statsHTML(data);
    else if (kind === 'media') html = mediaHTML(data, opts);
    else html = genericHTML(data);

    el.innerHTML = html;
    el.classList.add('reveal-in');

    const copyBtn = el.querySelector('[data-act="copy"]');
    if (copyBtn) copyBtn.addEventListener('click', function () {
      copy((data && (data.url || (data.images && data.images[0]))) || '');
    });
    const openAll = el.querySelector('[data-act="open-all"]');
    if (openAll) openAll.addEventListener('click', function () {
      const list = (data.images || []).map(function (u) { return typeof u === 'string' ? u : u.url; });
      copy(list.join('\n'));
      toast('已复制 ' + list.length + ' 条图片直链');
      list.slice(0, 6).forEach(function (u, i) {
        setTimeout(function () { window.open(u, '_blank', 'noopener'); }, i * 260);
      });
    });
  }

  window.RENDER = { show: show, num: num, pct: pct, detect: detect };
})();
