// Torrentino web UI helpers: CSRF header for htmx, visibility-aware polling
// for /downloads, focus after swaps and double-submit protection.
(function () {
  'use strict';

  if (window.htmx) {
    window.htmx.config.allowEval = false;
    document.addEventListener('htmx:configRequest', function (event) {
      var meta = document.querySelector('meta[name="csrf-token"]');
      if (meta) {
        event.detail.headers['X-CSRF-Token'] = meta.getAttribute('content');
      }
    });
  }

  var scrollBeforeSwap = null;

  function focusNoScroll(element, setTabIndex) {
    if (!element) { return; }
    if (setTabIndex) { element.setAttribute('tabindex', '-1'); }
    try {
      element.focus({ preventScroll: true });
    } catch (err) {
      element.focus();
    }
  }

  function focusAfterSwap(event) {
    var path = event && event.detail && event.detail.requestConfig && event.detail.requestConfig.path;
    var savedScroll = scrollBeforeSwap;
    scrollBeforeSwap = null;

    // Search replaces the list with a new query, so starting at the top is
    // expected. For commands and polling keep the user's scroll position.
    if (path && path.indexOf('/find/search') !== -1) {
      window.scrollTo(0, 0);
    } else if (savedScroll !== null) {
      window.scrollTo(0, savedScroll);
      window.requestAnimationFrame(function () { window.scrollTo(0, savedScroll); });
    }

    // Polling replaces the screen every five seconds; moving focus there would
    // steal it from the user. Only focus after an explicit command/search.
    if (path && path.indexOf('poll=1') !== -1) { return; }

    var alert = document.querySelector('#screen [role="alert"]');
    if (alert) {
      focusNoScroll(alert, true);
      return;
    }
    // After selecting a row keep keyboard focus on that row instead of jumping
    // to the heading (which used to scroll the page to the top).
    var selected = document.querySelector('#screen .row.selected .select');
    if (selected) {
      focusNoScroll(selected, false);
      return;
    }
    var heading = document.querySelector('#screen h1, #screen h2');
    if (heading) {
      focusNoScroll(heading, true);
    }
  }

  document.addEventListener('htmx:afterSwap', focusAfterSwap);

  // A stale selection is a normal answer (the list changed); render its
  // fragment instead of leaving the old screen in place.
  document.addEventListener('htmx:beforeSwap', function (event) {
    scrollBeforeSwap = window.scrollY;
    if (event.detail.xhr.status === 409 || event.detail.xhr.status === 429) {
      event.detail.shouldSwap = true;
      event.detail.isError = false;
    }
  });

  document.addEventListener('submit', function (event) {
    var form = event.target;
    if (!form || form.tagName !== 'FORM') { return; }
    var submit = form.querySelector('button[type="submit"], button:not([type])');
    if (submit) {
      window.setTimeout(function () { submit.disabled = true; }, 0);
    }
  }, true);

  var timer = null;
  function pollTarget() {
    var screen = document.getElementById('screen');
    if (!screen) { return null; }
    var url = screen.getAttribute('data-poll-url');
    return url || null;
  }
  function startPolling() {
    if (timer) { return; }
    var url = pollTarget();
    if (!url) { stopPolling(); return; }
    timer = window.setInterval(function () {
      if (document.visibilityState !== 'visible') { return; }
      var current = pollTarget();
      if (!current) { stopPolling(); return; }
      if (!window.htmx) { return; }
      window.htmx.ajax('GET', current, { target: '#screen', swap: 'outerHTML show:none' });
    }, 5000);
  }
  function stopPolling() {
    if (timer) {
      window.clearInterval(timer);
      timer = null;
    }
  }
  document.addEventListener('visibilitychange', function () {
    if (document.visibilityState === 'visible') { startPolling(); } else { stopPolling(); }
  });
  document.addEventListener('htmx:afterSwap', startPolling);
  startPolling();
})();
