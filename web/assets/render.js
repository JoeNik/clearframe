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

  // ---------- 趋势图（Tab 切换 + 悬浮逐日明细） ----------
  const TW = 680, TH = 210, TPL = 12, TPR = 12, TPT = 18, TPB = 30;
  const TIW = TW - TPL - TPR, TIH = TH - TPT - TPB;
  const TRENDS = [
    { k: 'play_count', label: '播放量', f: 'p' },
    { k: 'digg_count',  label: '点赞',   f: 'g' },
    { k: 'hot_score',   label: '热度分', f: 'h' }
  ];

  function trendChart(daily) {
    const keys = TRENDS.filter(function (t) {
      return daily.some(function (d) { return d[t.k] != null; });
    });
    if (!keys.length || daily.length < 2) return '';
    const n = daily.length;
    const x = function (i) { return TPL + TIW * (n === 1 ? 0.5 : i / (n - 1)); };

    const series = keys.map(function (t) {
      const vals = daily.map(function (d) { return Number(d[t.k]) || 0; });
      const max = Math.max.apply(null, vals) || 1;
      return { k: t.k, label: t.label,
        pts: vals.map(function (v, i) { return [x(i), TPT + TIH * (1 - v / max)]; }) };
    });
    const linePath = function (pts) {
      return pts.map(function (p, i) { return (i ? 'L' : 'M') + p[0].toFixed(1) + ' ' + p[1].toFixed(1); }).join(' ');
    };

    // X 轴日期刻度（最多 6 个，首尾必带）
    const xIdx = [];
    for (let i = 0; i < n; i += Math.max(1, Math.ceil(n / 6))) xIdx.push(i);
    if (xIdx[xIdx.length - 1] !== n - 1) xIdx.push(n - 1);

    let grid = '';
    [0, 0.5, 1].forEach(function (f) {
      const y = (TPT + TIH * f).toFixed(1);
      grid += '<line class="t2-grid" x1="' + TPL + '" y1="' + y + '" x2="' + (TW - TPR) + '" y2="' + y + '"/>';
    });
    let xlab = '';
    xIdx.forEach(function (i) {
      const d = daily[i] || {};
      xlab += '<text class="t2-xlab" text-anchor="middle" x="' + x(i).toFixed(1) + '" y="' + (TH - 8) + '">' +
        esc(String(d.date || '').slice(5)) + '</text>';
    });

    // 逐日明细挂到容器，供悬浮提示读取
    const daysLite = daily.map(function (d) {
      return { d: d.date, p: d.play_count, vp: d.valid_play_count, g: d.digg_count,
        c: d.comment_count, f: d.collect_count, s: d.share_count, h: d.hot_score };
    });

    const gid = 'tg' + Math.random().toString(36).slice(2, 8);
    return '<div class="trend2" data-days="' + esc(JSON.stringify(daysLite)) + '">' +
      '<div class="trend-head"><span class="trend-title">逐日趋势 <i class="t2-hint">悬停看每日明细</i></span>' +
      '<span class="trend-meta t2-peak"></span></div>' +
      (keys.length > 1
        ? '<div class="t2-tabs">' + keys.map(function (t, i) {
            return '<button type="button" class="t2-tab' + (i ? '' : ' on') + '" data-k="' + t.k + '">' +
              esc(t.label) + '</button>';
          }).join('') + '</div>'
        : '') +
      '<div class="t2-wrap"><svg class="t2-svg" viewBox="0 0 ' + TW + ' ' + TH + '" preserveAspectRatio="none">' +
        '<defs><linearGradient id="' + gid + '" x1="0" y1="0" x2="0" y2="1">' +
          '<stop offset="0%" stop-color="var(--accent)" stop-opacity=".25"/>' +
          '<stop offset="100%" stop-color="var(--accent)" stop-opacity="0"/></linearGradient></defs>' +
        grid +
        series.map(function (s, si) {
          return '<g class="t2-ser' + (si ? '' : ' on') + '" data-k="' + s.k + '">' +
            '<path class="t2-area" fill="url(#' + gid + ')" d="' + linePath(s.pts) +
              ' L' + x(n - 1).toFixed(1) + ' ' + (TPT + TIH) + ' L' + x(0).toFixed(1) + ' ' + (TPT + TIH) + ' Z"/>' +
            '<path class="t2-line" d="' + linePath(s.pts) + '"/></g>';
        }).join('') +
        series.map(function (s, si) {
          return s.pts.map(function (p) {
            return '<circle class="t2-dot' + (si ? '' : ' on') + '" data-k="' + s.k +
              '" cx="' + p[0].toFixed(1) + '" cy="' + p[1].toFixed(1) + '" r="2.6"/>';
          }).join('');
        }).join('') +
        '<line class="t2-cursor" x1="-10" y1="' + TPT + '" x2="-10" y2="' + (TPT + TIH) + '"/>' +
        xlab +
      '</svg><div class="t2-tip"></div></div>' +
    '</div>';
  }

  const TIP_ROWS = [
    ['播放', 'p'], ['有效播放', 'vp'], ['点赞', 'g'], ['评论', 'c'],
    ['收藏', 'f'], ['分享', 's'], ['热度分', 'h']
  ];

  function bindTrend(t2) {
    let days;
    try { days = JSON.parse(t2.getAttribute('data-days') || '[]'); } catch (e) { days = []; }
    if (!days.length) return;
    const n = days.length;
    const svg = t2.querySelector('.t2-svg');
    const tip = t2.querySelector('.t2-tip');
    const cur = t2.querySelector('.t2-cursor');
    const peakEl = t2.querySelector('.t2-peak');
    const labels = { play_count: '播放量', digg_count: '点赞', hot_score: '热度分' };
    const valOf = function (d, k) {
      const t = TRENDS.filter(function (x) { return x.k === k; })[0];
      return t ? d[t.f] : undefined;
    };

    function setActive(k) {
      t2.querySelectorAll('.t2-ser,.t2-dot,.t2-tab').forEach(function (e) {
        e.classList.toggle('on', e.getAttribute('data-k') === k);
      });
      const vals = days.map(function (d) { return Number(valOf(d, k)) || 0; });
      const max = Math.max.apply(null, vals) || 0;
      const maxI = vals.indexOf(max);
      if (peakEl && max > 0) {
        peakEl.innerHTML = '峰值 <b>' + esc(String(days[maxI].d || '').slice(5)) + ' · ' + num(max) +
          '</b> · 最新 <b>' + num(vals[n - 1]) + '</b>';
      }
    }
    t2.querySelectorAll('.t2-tab').forEach(function (btn) {
      btn.addEventListener('click', function () { setActive(btn.getAttribute('data-k')); });
    });
    const onTab = t2.querySelector('.t2-tab.on');
    setActive(onTab ? onTab.getAttribute('data-k') : 'play_count');

    if (!svg || !tip) return;
    svg.addEventListener('mousemove', function (ev) {
      const r = svg.getBoundingClientRect();
      const vx = (ev.clientX - r.left) / r.width * TW;
      let i = Math.round((vx - TPL) / TIW * (n - 1));
      i = Math.max(0, Math.min(n - 1, i));
      const cx = TPL + TIW * (n === 1 ? 0.5 : i / (n - 1));
      if (cur) { cur.setAttribute('x1', cx); cur.setAttribute('x2', cx); }
      const d = days[i] || {};
      const rows = TIP_ROWS.filter(function (p) { return d[p[1]] != null; });
      tip.innerHTML = '<b>' + esc(d.d || '') + '</b>' + rows.map(function (p) {
        return '<span>' + p[0] + '</span><em>' + num(d[p[1]]) + '</em>';
      }).join('');
      tip.classList.add('on');
      const px = (cx / TW) * r.width;
      tip.style.left = Math.min(Math.max(px, 78), r.width - 78) + 'px';
    });
    svg.addEventListener('mouseleave', function () {
      tip.classList.remove('on');
      if (cur) { cur.setAttribute('x1', -10); cur.setAttribute('x2', -10); }
    });
  }

  // ---------- 数据分析 ----------
  function statsHTML(data) {
    const extra = data.extra || data;
    const st = extra.statistics || {};
    const trend = extra.statistics_trend || {};
    const daily = Array.isArray(trend.daily) ? trend.daily : [];
    const tsum = trend.summary || {};
    const follower = (extra.author_extra || {}).follower_count;
    const play = Number(st.play_count) || 0;
    const rateOf = function (v) { return play > 0 ? Number(v) / play : null; };

    let html = '<div class="result-card stats-card">' +
      '<div class="result-head"><div class="result-title">作品数据洞察</div>' +
      '<div class="result-tags"><span class="tag tag-accent">数据分析</span>' +
      (trend.days ? '<span class="tag">观测期 ' + esc(trend.start_date || '') + ' ~ ' + esc(trend.end_date || '') + '</span>' : '') +
      (extra.aweme_id ? '<span class="tag mono">ID ' + esc(extra.aweme_id) + '</span>' : '') +
      '</div></div>';

    // ---- 主指标：播放量（含有效播放）----
    const totals = tsum.totals || {};
    const validPlay = totals.valid_play_count != null ? Number(totals.valid_play_count) : null;
    const lastDay = daily.length ? daily[daily.length - 1] : null;
    const finish = tsum.weighted_rates && tsum.weighted_rates.finish_rate != null
      ? Number(tsum.weighted_rates.finish_rate)
      : (st.finish_rate != null ? Number(st.finish_rate)
        : (lastDay && lastDay.finish_rate != null ? Number(lastDay.finish_rate) : null));

    const rings = [];
    if (st.interact_rate != null) rings.push({ label: '互动率', v: Number(st.interact_rate) });
    if (finish != null) rings.push({ label: '完播率', v: finish });
    if (follower != null) rings.push({ label: '作者粉丝', v: null, text: num(follower) });

    html += '<div class="stat-hero">' +
      '<div class="sh-main">' +
        '<div class="sh-label">累计播放</div>' +
        '<div class="sh-val">' + num(play) + '</div>' +
        (validPlay != null
          ? '<div class="sh-sub">其中有效播放 <b>' + num(validPlay) + '</b> · 占 ' + pct(rateOf(validPlay), 1) + '</div>'
          : '') +
      '</div>' +
      (rings.length ? '<div class="sh-side">' + rings.map(function (r) {
        if (r.v == null) {
          return '<div class="ring"><div class="ring-val">' + r.text + '</div>' +
            '<div class="ring-label">' + r.label + '</div></div>';
        }
        const p = Math.max(0, Math.min(1, r.v));
        return '<div class="ring"><div class="ring-dial" style="--deg:' + (p * 360).toFixed(1) + 'deg">' +
          '<span>' + pct(r.v, p < 0.1 ? 2 : 1) + '</span></div>' +
          '<div class="ring-label">' + r.label + '</div></div>';
      }).join('') + '</div>' : '') +
    '</div>';

    // ---- 次级指标：带「每百次播放」转化副行 ----
    const cards = [
      { k: 'digg_count', label: '点赞', ico: 'bolt' },
      { k: 'comment_count', label: '评论', ico: 'chat' },
      { k: 'collect_count', label: '收藏', ico: 'check' },
      { k: 'share_count', label: '分享', ico: 'link' }
    ].filter(function (c) { return st[c.k] != null; });
    if (cards.length) {
      html += '<div class="metric-grid">' + cards.map(function (c) {
        const r = rateOf(st[c.k]);
        return '<div class="metric"><span class="metric-ico">' + icon(c.ico, 14) + '</span>' +
          '<div class="metric-val">' + num(st[c.k]) + '</div>' +
          '<div class="metric-label">' + c.label + '</div>' +
          (r != null ? '<div class="metric-sub">每 100 次播放约 ' + (r * 100).toFixed(1) + ' 次</div>' : '') +
          '</div>';
      }).join('') + '</div>';
    }

    // ---- 自动洞察（全部由真实数据推导，不编造）----
    const insights = [];
    if (daily.length) {
      const plays = daily.map(function (d) { return Number(d.play_count) || 0; });
      const peakV = Math.max.apply(null, plays);
      const peakI = plays.indexOf(peakV);
      const psum = plays.reduce(function (a, b) { return a + b; }, 0);
      if (peakV > 0) {
        let t = '峰值出现在 ' + esc(daily[peakI].date) + '，单日播放 ' + num(peakV);
        if (psum > 0 && peakV / psum > 0.5) t += '，占观测期总量的 ' + pct(peakV / psum, 0);
        insights.push(t);
      }
      const tz = ((tsum.trailing_zero_play || {}).days) || 0;
      const lastV = plays[plays.length - 1];
      if (tz > 0) insights.push('最近 ' + tz + ' 天无新增播放，热度已回落');
      else if (peakV > 0 && lastV === 0) insights.push('最新一日无新增播放，热度趋于平缓');
      else if (peakV > 0 && lastV <= peakV * 0.001) insights.push('播放已基本归零，热度趋于平缓');
      else if (peakV > 0 && lastV <= peakV * 0.05) insights.push('播放已回落至峰值的 ' + pct(lastV / peakV, 1) + '，趋于平缓');
      else if (peakV > 0 && lastV >= peakV * 0.7) insights.push('近期播放仍处高位');
    }
    if (st.interact_rate != null && play > 0) {
      insights.push('每 100 次播放带来约 ' + Math.round(Number(st.interact_rate) * 100) + ' 次互动' +
        (st.digg_count != null ? '，其中点赞 ' + num(st.digg_count) + ' 次贡献最大' : ''));
    }
    if (insights.length) {
      html += '<div class="insights">' + insights.map(function (t) {
        return '<div class="ins-row"><span class="ins-dot"></span><span>' + t + '</span></div>';
      }).join('') + '</div>';
    }

    // ---- 逐日趋势（可切换指标、悬浮看明细）或互动构成 ----
    if (daily.length >= 2) {
      html += trendChart(daily);
    } else {
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
              '<span class="bar-v">' + num(v) + ' · 占播放 ' + pct(rateOf(v), 1) + '</span></div>';
          }).join('') + '</div></div>';
      }
    }

    html += '<div class="gloss">互动率 =（点赞+评论+收藏+分享）÷ 播放量 · 完播率 = 看完视频的播放占比 · 热度分为平台综合热度指标，供横向比较 · 数据来自上游统计，观测期为上游返回窗口</div>';
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

    // 数据分析卡：绑定趋势图 Tab 切换与悬浮明细
    const t2 = el.querySelector('.trend2');
    if (t2) bindTrend(t2);

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
