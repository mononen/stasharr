// ==UserScript==
// @name         Stasharr
// @namespace    github.com/mononen/stasharr
// @version      0.1.0
// @description  Send StashDB content to Stasharr for automated acquisition
// @author       mononen
// @match        https://stashdb.org/*
// @grant        GM_xmlhttpRequest
// @grant        GM_getValue
// @grant        GM_setValue
// @grant        GM_registerMenuCommand
// @connect      localhost
// @connect      *
// @run-at       document-idle
// ==/UserScript==

(function () {
  'use strict';

  // --- Configuration ---
  const CONFIG_KEYS = {
    URL: 'stasharr_url',
    API_KEY: 'stasharr_api_key',
  };

  const DEFAULTS = {
    [CONFIG_KEYS.URL]: 'http://localhost:8080',
    [CONFIG_KEYS.API_KEY]: '',
  };

  function getConfig() {
    return {
      url: GM_getValue(CONFIG_KEYS.URL, DEFAULTS[CONFIG_KEYS.URL]),
      apiKey: GM_getValue(CONFIG_KEYS.API_KEY, DEFAULTS[CONFIG_KEYS.API_KEY]),
    };
  }

  // Register settings menu command
  GM_registerMenuCommand('Stasharr Settings', () => {
    // TODO: implement settings dialog
  });

  // --- URL Detection ---
  const URL_PATTERNS = [
    { pattern: /\/scenes\/([a-f0-9-]+)/i, type: 'scene' },
    { pattern: /\/performers\/([a-f0-9-]+)/i, type: 'performer' },
    { pattern: /\/studios\/([a-f0-9-]+)/i, type: 'studio' },
  ];

  function detectPage() {
    const path = window.location.pathname;
    for (const { pattern, type } of URL_PATTERNS) {
      const match = path.match(pattern);
      if (match) {
        return { type, id: match[1] };
      }
    }
    return null;
  }

  // --- Panel Injection ---
  // TODO: implement panel DOM creation and injection
  // TODO: implement MutationObserver for SPA navigation
  // TODO: implement submission logic via GM_xmlhttpRequest
})();
