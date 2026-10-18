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


  var themeButton = document.getElementById('theme-toggle');
  function currentTheme() {
    return document.documentElement.getAttribute('data-theme') === 'dark' ? 'dark' : 'light';
  }
  function syncThemeButton(theme) {
    if (!themeButton) { return; }
    themeButton.hidden = false;
    themeButton.textContent = theme === 'dark' ? '☀' : '☾';
    var label = theme === 'dark' ? 'Switch to light theme' : 'Switch to dark theme';
    themeButton.setAttribute('aria-label', label);
    themeButton.setAttribute('title', label);
    themeButton.setAttribute('aria-pressed', theme === 'dark' ? 'true' : 'false');
  }
  if (themeButton) {
    syncThemeButton(currentTheme());
    themeButton.addEventListener('click', function () {
      var next = currentTheme() === 'dark' ? 'light' : 'dark';
      document.documentElement.setAttribute('data-theme', next);
      try { localStorage.setItem('torrentino-theme', next); } catch (err) {}
      syncThemeButton(next);
    });
    if (window.matchMedia) {
      var themeMedia = window.matchMedia('(prefers-color-scheme: dark)');
      var onSystemThemeChange = function () {
        var saved = null;
        try { saved = localStorage.getItem('torrentino-theme'); } catch (err) {}
        if (saved === 'light' || saved === 'dark') { return; }
        var next = themeMedia.matches ? 'dark' : 'light';
        document.documentElement.setAttribute('data-theme', next);
        syncThemeButton(next);
      };
      if (themeMedia.addEventListener) { themeMedia.addEventListener('change', onSystemThemeChange); }
    }
  }

  var scrollBeforeSwap = null;
  var pendingScrollRestore = null;

  var busyOverlay = document.getElementById('busy');
  var busyScreen = document.getElementById('screen');
  function setBusy(on) {
    if (busyOverlay) { busyOverlay.hidden = !on; }
    if (busyScreen) {
      if (on) {
        busyScreen.setAttribute('inert', '');
        busyScreen.setAttribute('aria-busy', 'true');
      } else {
        busyScreen.removeAttribute('inert');
        busyScreen.setAttribute('aria-busy', 'false');
      }
    }
    document.body.classList.toggle('is-busy', on);
  }
  function requestPath(event) {
    return event && event.detail && event.detail.requestConfig && event.detail.requestConfig.path;
  }
  function isPoll(event) {
    var path = requestPath(event);
    return !!path && path.indexOf('poll=1') !== -1;
  }
  document.addEventListener('htmx:beforeRequest', function (event) {
    if (!isPoll(event)) { setBusy(true); }
  });
  document.addEventListener('htmx:afterSettle', function (event) {
    if (!isPoll(event)) { setBusy(false); }
  });
  document.addEventListener('htmx:afterRequest', function (event) {
    if (!isPoll(event) && event.detail && !event.detail.successful) { setBusy(false); }
  });
  document.addEventListener('htmx:sendError', function (event) {
    if (!isPoll(event)) { setBusy(false); }
  });

  function restoreScroll(y) {
    window.scrollTo(0, y);
    window.requestAnimationFrame(function () { window.scrollTo(0, y); });
    window.setTimeout(function () { window.scrollTo(0, y); }, 50);
  }

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
      pendingScrollRestore = null;
      window.scrollTo(0, 0);
    } else if (savedScroll !== null) {
      pendingScrollRestore = savedScroll;
      restoreScroll(savedScroll);
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
  // Restore once more after htmx/focus has settled; this also covers Find's
  // search field and Torrserver rows, which used to scroll the page to the top.
  document.addEventListener('htmx:afterSettle', function () {
    if (pendingScrollRestore !== null) {
      restoreScroll(pendingScrollRestore);
      pendingScrollRestore = null;
    }
  });

  // Keep the search field convenient on initial load, but never via autofocus:
  // autofocus fires again on every htmx screen swap and scrolls to the top.
  var initialSearch = document.querySelector('#screen[data-kind="find"] .search-form input[type="search"]');
  if (initialSearch) {
    focusNoScroll(initialSearch, false);
  }

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
