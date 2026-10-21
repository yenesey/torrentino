// Set the colour theme before the stylesheet is parsed to avoid a flash of the
// wrong theme. app.js owns the toggle and persists the explicit choice.
(function () {
  'use strict';

  var ua = navigator.userAgent || '';
  if (/Android/i.test(ua)) {
    document.documentElement.setAttribute('data-os', 'android');
  } else if (/Windows/i.test(ua)) {
    document.documentElement.setAttribute('data-os', 'windows');
  }

  try {
    var saved = localStorage.getItem('torrentino-theme');
    var theme = (saved === 'light' || saved === 'dark')
      ? saved
      : (window.matchMedia && window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light');
    document.documentElement.setAttribute('data-theme', theme);
  } catch (err) {
    // Storage can be unavailable in private mode; the CSS media query remains.
  }
})();
