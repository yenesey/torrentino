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

  function focusAfterSwap(event) {
    // Polling replaces the screen every five seconds; moving focus there would
    // steal it from the user. Only focus after an explicit command/search.
    var path = event && event.detail && event.detail.requestConfig && event.detail.requestConfig.path;
    if (path && path.indexOf('poll=1') !== -1) { return; }
    var alert = document.querySelector('#screen [role="alert"]');
    if (alert) {
      alert.setAttribute('tabindex', '-1');
      alert.focus();
      return;
    }
    var heading = document.querySelector('#screen h1, #screen h2');
    if (heading) {
      heading.setAttribute('tabindex', '-1');
      heading.focus();
    }
  }

  document.addEventListener('htmx:afterSwap', focusAfterSwap);

  // A stale selection is a normal answer (the list changed); render its
  // fragment instead of leaving the old screen in place.
  document.addEventListener('htmx:beforeSwap', function (event) {
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
      window.htmx.ajax('GET', current, { target: '#screen', swap: 'outerHTML' });
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
