// ==UserScript==
// @name         Stasharr
// @namespace    github.com/mononen/stasharr
// @version      0.1.1
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
  // If you want to hardcode your settings, edit the values in DEFAULTS below.
  // Otherwise, use the "Stasharr Settings" menu in Tampermonkey.
  const DEFAULTS = {
    url: 'http://localhost:3000', // Your Stasharr UI URL
    apiKey: '',                  // Your STASHARR_SECRET_KEY
    collapsed: false,
  };

  // Internal storage keys (do not change these)
  const STORAGE_KEYS = {
    URL: 'stasharr_url',
    API_KEY: 'stasharr_api_key',
    COLLAPSED: 'stasharr_collapsed',
  };

  const URL_PATTERNS = [
    { pattern: /\/scenes\/([a-f0-9-]+)/i, type: 'scene' },
    { pattern: /\/performers\/([a-f0-9-]+)/i, type: 'performer' },
    { pattern: /\/studios\/([a-f0-9-]+)/i, type: 'studio' },
  ];

  const STYLES = `
    #stasharr-panel {
      position: fixed;
      bottom: 20px;
      right: 20px;
      width: 280px;
      background: #1a1b1e;
      color: #c1c2c5;
      border: 1px solid #373a40;
      border-radius: 8px;
      box-shadow: 0 10px 15px -3px rgba(0, 0, 0, 0.5);
      z-index: 9999;
      font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif;
      font-size: 14px;
      overflow: hidden;
      transition: all 0.2s ease-in-out;
    }
    #stasharr-panel.collapsed {
      width: 140px;
    }
    #stasharr-panel .header {
      display: flex;
      justify-content: space-between;
      align-items: center;
      padding: 8px 12px;
      background: #25262b;
      border-bottom: 1px solid #373a40;
      font-weight: 600;
      user-select: none;
    }
    #stasharr-panel .header .title {
      display: flex;
      align-items: center;
      gap: 6px;
    }
    #stasharr-panel .header .controls {
      cursor: pointer;
      color: #909296;
      font-size: 18px;
    }
    #stasharr-panel .content {
      padding: 12px;
      display: flex;
      flex-direction: column;
      gap: 12px;
    }
    #stasharr-panel.collapsed .content {
      display: none;
    }
    #stasharr-panel .entity-name {
      font-weight: 500;
      color: #fff;
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
    }
    #stasharr-panel .status-text {
      font-size: 12px;
      color: #909296;
    }
    #stasharr-panel button {
      background: #228be6;
      color: #fff;
      border: none;
      padding: 8px 12px;
      border-radius: 4px;
      cursor: pointer;
      font-weight: 500;
      transition: background 0.2s;
    }
    #stasharr-panel button:hover {
      background: #1c7ed6;
    }
    #stasharr-panel button:disabled {
      background: #373a40;
      color: #5c5f66;
      cursor: not-allowed;
    }
    #stasharr-panel .alert {
      padding: 8px;
      border-radius: 4px;
      font-size: 12px;
    }
    #stasharr-panel .alert-warning {
      background: rgba(240, 140, 0, 0.1);
      color: #ff922b;
      border: 1px solid rgba(240, 140, 0, 0.2);
    }
    #stasharr-panel .alert-error {
      background: rgba(250, 82, 82, 0.1);
      color: #fa5252;
      border: 1px solid rgba(250, 82, 82, 0.2);
    }
    #stasharr-panel .alert-success {
      background: rgba(64, 192, 87, 0.1);
      color: #40c057;
      border: 1px solid rgba(64, 192, 87, 0.2);
    }
    #stasharr-panel a {
      color: #339af0;
      text-decoration: none;
    }
    #stasharr-panel a:hover {
      text-decoration: underline;
    }
    #stasharr-panel .tag-input-label {
      font-size: 11px;
      color: #909296;
      margin-bottom: 3px;
    }
    #stasharr-panel input[type="text"] {
      width: 100%;
      background: #2c2d32;
      color: #c1c2c5;
      border: 1px solid #373a40;
      border-radius: 4px;
      padding: 5px 8px;
      font-size: 12px;
      box-sizing: border-box;
      outline: none;
    }
    #stasharr-panel input[type="text"]:focus {
      border-color: #228be6;
    }
  `;

  // --- State ---
  let currentState = {
    page: null,
    submitting: false,
    result: null,
    startingAll: false,
    collapsed: GM_getValue(STORAGE_KEYS.COLLAPSED, DEFAULTS.collapsed),
    tagInput: '',
  };

  // --- Configuration ---
  function getConfig() {
    return {
      url: GM_getValue(STORAGE_KEYS.URL, DEFAULTS.url),
      apiKey: GM_getValue(STORAGE_KEYS.API_KEY, DEFAULTS.apiKey),
    };
  }

  function showSettings() {
    const config = getConfig();
    const url = prompt('Stasharr Base URL:', config.url);
    if (url !== null) {
      GM_setValue(STORAGE_KEYS.URL, url.replace(/\/$/, ''));
    }
    const apiKey = prompt('Stasharr API Key (STASHARR_SECRET_KEY):', config.apiKey);
    if (apiKey !== null) {
      GM_setValue(STORAGE_KEYS.API_KEY, apiKey);
    }
    updateUI();
  }

  GM_registerMenuCommand('Stasharr Settings', showSettings);

  // --- Helpers ---
  function detectPage() {
    const path = window.location.pathname;
    for (const { pattern, type } of URL_PATTERNS) {
      const match = path.match(pattern);
      if (match) {
        return { type, id: match[1], url: window.location.href };
      }
    }
    return null;
  }

  function getEntityName() {
    // Try to find the title in common StashDB locations
    const selectors = [
      'h1',
      '.scene-info-header h1',
      '.performer-info-header h1',
      '.studio-info-header h1',
      'title'
    ];

    for (const selector of selectors) {
      const el = document.querySelector(selector);
      if (el) {
        let text = el.innerText || el.textContent;
        text = text.replace(/ - StashDB$/, '').trim();
        if (text && text !== 'StashDB' && text !== 'Loading...') {
          return text;
        }
      }
    }

    // Fallback to URL ID if name not found yet
    const page = detectPage();
    return page ? `ID: ${page.id.substring(0, 8)}...` : 'Unknown Entity';
  }

  // --- UI Logic ---
  let panelElement = null;

  function injectPanel() {
    if (panelElement) return;

    const style = document.createElement('style');
    style.textContent = STYLES;
    document.head.appendChild(style);

    panelElement = document.createElement('div');
    panelElement.id = 'stasharr-panel';
    document.body.appendChild(panelElement);

    updateUI();
  }

  function toggleCollapse() {
    currentState.collapsed = !currentState.collapsed;
    GM_setValue(STORAGE_KEYS.COLLAPSED, currentState.collapsed);
    updateUI();
  }

  function updateUI() {
    if (!panelElement) return;

    const config = getConfig();
    const isConfigured = config.url && config.apiKey;
    const page = detectPage();
    currentState.page = page;

    panelElement.className = currentState.collapsed ? 'collapsed' : '';

    let contentHtml = '';
    const headerHtml = `
      <div class="header">
        <div class="title">🎬 Stasharr</div>
        <div class="controls" id="stasharr-toggle">${currentState.collapsed ? '[+]' : '[−]'}</div>
      </div>
    `;

    if (!isConfigured) {
      contentHtml = `
        <div class="content">
          <div class="alert alert-warning">
            ⚠ Not configured.<br/>
            Open Tampermonkey menu to set URL and API Key.
          </div>
        </div>
      `;
    } else if (!page) {
      contentHtml = `
        <div class="content">
          <div class="status-text">Browse to a scene, performer, or studio to begin.</div>
        </div>
      `;
    } else if (currentState.submitting) {
      contentHtml = `
        <div class="content">
          <div class="entity-name">${getEntityName()}</div>
          <div class="status-text">⏳ Submitting...</div>
        </div>
      `;
    } else if (currentState.result) {
      const r = currentState.result;
      const isBatch = page && (page.type === 'performer' || page.type === 'studio');
      contentHtml = `
        <div class="content">
          <div class="alert alert-${r.success ? 'success' : 'error'}">
            ${r.success ? '✓ Queued!' : '✗ Failed: ' + r.message}
          </div>
          ${r.link ? `<a href="${r.link}" target="_blank">View in Stasharr →</a>` : ''}
          ${r.success && isBatch && r.batchId ? `
            <button id="stasharr-autostart" ${currentState.startingAll ? 'disabled' : ''}>
              ${currentState.startingAll ? 'Starting…' : 'Start all 20 now'}
            </button>
          ` : ''}
          <button id="stasharr-reset">Back</button>
        </div>
      `;
    } else {
      const typeLabel = page.type.charAt(0).toUpperCase() + page.type.slice(1);
      const isBatchType = page.type === 'performer' || page.type === 'studio';
      contentHtml = `
        <div class="content">
          <div class="status-text">${typeLabel}:</div>
          <div class="entity-name" title="${getEntityName()}">${getEntityName()}</div>
          ${isBatchType ? `
            <div>
              <div class="tag-input-label">Tag IDs (optional, comma-separated)</div>
              <input type="text" id="stasharr-tag-input" placeholder="e.g. abc123, def456" value="${currentState.tagInput}">
            </div>
          ` : ''}
          <button id="stasharr-submit">Send to Stasharr</button>
        </div>
      `;
    }

    panelElement.innerHTML = headerHtml + contentHtml;

    // Re-attach listeners
    document.getElementById('stasharr-toggle').onclick = toggleCollapse;
    const tagInput = document.getElementById('stasharr-tag-input');
    if (tagInput) {
      tagInput.oninput = (e) => { currentState.tagInput = e.target.value; };
    }
    const submitBtn = document.getElementById('stasharr-submit');
    if (submitBtn) submitBtn.onclick = submitToStasharr;
    const resetBtn = document.getElementById('stasharr-reset');
    if (resetBtn) resetBtn.onclick = () => { currentState.result = null; currentState.startingAll = false; currentState.tagInput = ''; updateUI(); };
    const autoStartBtn = document.getElementById('stasharr-autostart');
    if (autoStartBtn) autoStartBtn.onclick = autoStartBatch;
  }

  // --- API Logic ---
  function submitToStasharr() {
    const config = getConfig();
    const page = currentState.page;
    if (!page) return;

    currentState.submitting = true;
    currentState.result = null;
    updateUI();

    GM_xmlhttpRequest({
      method: 'POST',
      url: `${config.url}/api/v1/jobs`,
      headers: {
        'Content-Type': 'application/json',
        'X-Api-Key': config.apiKey
      },
      data: JSON.stringify((() => {
        const req = { url: page.url, type: page.type };
        if ((page.type === 'performer' || page.type === 'studio') && currentState.tagInput.trim()) {
          req.tag_ids = currentState.tagInput.split(',').map(s => s.trim()).filter(Boolean);
        }
        return req;
      })()),
      onload: (response) => {
        currentState.submitting = false;
        try {
          const body = JSON.parse(response.responseText);
          if (response.status === 202) {
            let link = '';
            let batchId = null;
            if (page.type === 'scene') {
              link = `${config.url}/queue?job=${body.job_id}`;
            } else {
              batchId = body.batch_job_id || null;
              link = `${config.url}/batches${batchId ? '/' + batchId : ''}`;
            }
            currentState.result = { success: true, link, batchId };
          } else {
            currentState.result = {
              success: false,
              message: body.error ? body.error.message : `Server returned ${response.status}`
            };
          }
        } catch (e) {
          currentState.result = { success: false, message: 'Invalid server response' };
        }
        updateUI();
      },
      onerror: () => {
        currentState.submitting = false;
        currentState.result = { success: false, message: 'Could not reach Stasharr. Is it running?' };
        updateUI();
      }
    });
  }

  function autoStartBatch() {
    const config = getConfig();
    const batchId = currentState.result && currentState.result.batchId;
    if (!batchId) return;

    currentState.startingAll = true;
    updateUI();

    GM_xmlhttpRequest({
      method: 'POST',
      url: `${config.url}/api/v1/batches/${batchId}/auto-start`,
      headers: {
        'Content-Type': 'application/json',
        'X-Api-Key': config.apiKey
      },
      data: '',
      onload: (response) => {
        currentState.startingAll = false;
        try {
          const body = JSON.parse(response.responseText);
          if (response.status === 200) {
            currentState.result = {
              ...currentState.result,
              success: true,
              batchId: null, // hide the button after starting
              startedCount: body.started,
            };
          } else {
            currentState.result = {
              ...currentState.result,
              success: false,
              message: body.error ? body.error.message : `Server returned ${response.status}`
            };
          }
        } catch (e) {
          currentState.result = { ...currentState.result, success: false, message: 'Invalid server response' };
        }
        updateUI();
      },
      onerror: () => {
        currentState.startingAll = false;
        currentState.result = { ...currentState.result, success: false, message: 'Could not reach Stasharr.' };
        updateUI();
      }
    });
  }

  // --- Initialization & Observation ---
  injectPanel();

  // Watch for SPA navigation
  let lastUrl = window.location.href;
  const observer = new MutationObserver(() => {
    if (window.location.href !== lastUrl) {
      lastUrl = window.location.href;
      currentState.result = null;
      updateUI();
    }
  });
  observer.observe(document.body, { childList: true, subtree: true });

})();
