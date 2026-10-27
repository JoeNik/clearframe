/* 轻量页面动效：滚动入场 / 数字滚动 / 卡片聚光 / Hero 视差
   全部走 CSS 变量 + IntersectionObserver，不引入任何依赖。 */
(function () {
  'use strict';

  const REDUCED = window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches;

  // ---------- 滚动入场 ----------
  function reveal() {
    const els = Array.prototype.slice.call(document.querySelectorAll('[data-reveal], .cap-group, .section-head'));
    if (!els.length) return;
    // 开启隐藏态：只有 JS 正常执行到这里，元素才会先隐藏再入场
    document.documentElement.classList.add('fx-on');

    if (REDUCED || !('IntersectionObserver' in window)) {
      els.forEach(function (el) { el.classList.add('in'); });
      return;
    }

    els.forEach(function (el, i) {
      el.style.setProperty('--d', Math.min(i, 6) * 55 + 'ms');
    });

    // 首屏内已经可见的元素立即入场，避免等待观察器回调
    const vh = window.innerHeight || 800;
    const rest = [];
    els.forEach(function (el) {
      const r = el.getBoundingClientRect();
      if (r.top < vh * 0.94) el.classList.add('in');
      else rest.push(el);
    });

    const io = new IntersectionObserver(function (entries) {
      entries.forEach(function (en) {
        if (!en.isIntersecting) return;
        en.target.classList.add('in');
        io.unobserve(en.target);
      });
    }, { rootMargin: '0px 0px -6% 0px', threshold: 0.06 });
    rest.forEach(function (el) { io.observe(el); });

    // 兜底：2 秒后仍未入场的元素直接显示，任何情况下都不留白
    setTimeout(function () {
      els.forEach(function (el) { el.classList.add('in'); });
    }, 2000);
  }

  // ---------- 数字滚动 ----------
  function countUp(el) {
    const target = Number(el.getAttribute('data-count')) || 0;
    el.dataset.done = '1';
    if (REDUCED) { el.textContent = String(target); return; }
    const dur = 900;
    const t0 = performance.now();
    function step(t) {
      const p = Math.min(1, (t - t0) / dur);
      const eased = 1 - Math.pow(1 - p, 3);
      el.textContent = String(Math.round(target * eased));
      if (p < 1) requestAnimationFrame(step);
    }
    requestAnimationFrame(step);
  }

  function counters() {
    const els = Array.prototype.slice.call(document.querySelectorAll('[data-count]'));
    if (!els.length) return;
    if (!('IntersectionObserver' in window)) { els.forEach(countUp); return; }
    const io = new IntersectionObserver(function (entries) {
      entries.forEach(function (en) {
        if (!en.isIntersecting) return;
        countUp(en.target);
        io.unobserve(en.target);
      });
    }, { threshold: 0.4 });
    els.forEach(function (el) { io.observe(el); });
    // 兜底：2 秒后直接把数字落到终值
    setTimeout(function () {
      els.forEach(function (el) {
        if (el.dataset.done) return;
        el.dataset.done = '1';
        el.textContent = String(Number(el.getAttribute('data-count')) || 0);
      });
    }, 2000);
  }

  // ---------- 卡片聚光 ----------
  function spotlight() {
    if (REDUCED) return;
    document.addEventListener('pointermove', function (e) {
      const card = e.target.closest && e.target.closest('[data-spot]');
      if (!card) return;
      const r = card.getBoundingClientRect();
      card.style.setProperty('--mx', (e.clientX - r.left) + 'px');
      card.style.setProperty('--my', (e.clientY - r.top) + 'px');
    }, { passive: true });
  }

  // ---------- Hero 视差 ----------
  function heroParallax() {
    if (REDUCED) return;
    const hero = document.querySelector('.tool-hero');
    if (!hero) return;
    const orbs = hero.querySelector('.fx-orbs');
    if (!orbs) return;
    hero.addEventListener('pointermove', function (e) {
      const r = hero.getBoundingClientRect();
      const x = (e.clientX - r.left) / r.width - 0.5;
      const y = (e.clientY - r.top) / r.height - 0.5;
      orbs.style.setProperty('--px', (x * 26).toFixed(1) + 'px');
      orbs.style.setProperty('--py', (y * 20).toFixed(1) + 'px');
    }, { passive: true });
  }

  // ---------- 解析框聚焦辉光 ----------
  function parseFocus() {
    const box = document.querySelector('.parse-box');
    if (!box) return;
    const input = box.querySelector('input');
    if (!input) return;
    input.addEventListener('focus', function () { box.classList.add('focus'); });
    input.addEventListener('blur', function () { box.classList.remove('focus'); });
  }

  function init() {
    reveal();
    counters();
    spotlight();
    heroParallax();
    parseFocus();
  }

  window.FX = { init: init };
})();
