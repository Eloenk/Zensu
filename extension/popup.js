document.addEventListener('DOMContentLoaded', () => {
  const domainText = document.getElementById('domain-text');
  const cookieOutput = document.getElementById('cookie-output');
  const btnCopy = document.getElementById('btn-copy');
  const btnRefresh = document.getElementById('btn-refresh');
  const statusMessage = document.getElementById('status-message');

  function showStatus(text, isError = false) {
    statusMessage.textContent = text;
    statusMessage.className = 'status-text ' + (isError ? 'status-error' : 'status-success');
  }

  function clearStatus() {
    statusMessage.textContent = '';
    statusMessage.className = 'status-text';
  }

  function getBaseDomain(hostname) {
    const parts = hostname.split('.');
    if (parts.length > 2) {
      const last2 = parts.slice(-2).join('.');
      const secondToLast = parts[parts.length - 2].toLowerCase();
      const doubleTldShorts = ['co', 'com', 'org', 'net', 'gov', 'edu', 'ac'];
      if (parts.length > 3 && doubleTldShorts.includes(secondToLast)) {
        return parts.slice(-3).join('.');
      }
      return last2;
    }
    return hostname;
  }

  async function loadCookies() {
    clearStatus();
    btnCopy.disabled = true;
    cookieOutput.value = '';

    const tabs = await chrome.tabs.query({ active: true, currentWindow: true });
    if (!tabs || tabs.length === 0) {
      domainText.textContent = 'No active tab found';
      showStatus('Error: Could not determine active tab.', true);
      return;
    }

    const activeTab = tabs[0];
    const urlString = activeTab.url;

    if (!urlString) {
      domainText.textContent = 'Restricted page';
      showStatus('Error: Cannot access URLs of this page type.', true);
      return;
    }

    try {
      const urlObj = new URL(urlString);

      if (urlObj.protocol !== 'http:' && urlObj.protocol !== 'https:') {
        domainText.textContent = urlObj.protocol + '// page';
        showStatus('Error: Cannot retrieve cookies for non-web pages.', true);
        return;
      }

      const hostname = urlObj.hostname;
      domainText.textContent = hostname;

      const baseDomain = getBaseDomain(hostname);

      const queryParams = {};
      if (activeTab.storeId) {
        queryParams.storeId = activeTab.storeId;
      }

      const unpartitionedQuery = new Promise((resolve) => {
        chrome.cookies.getAll(queryParams, (res) => resolve(res || []));
      });

      const partitionedQuery = new Promise((resolve) => {
        try {
          chrome.cookies.getAll({ ...queryParams, partitionKey: {} }, (res) => resolve(res || []));
        } catch (err) {
          // Fallback if partitionKey is not supported in older browser versions
          resolve([]);
        }
      });

      Promise.all([unpartitionedQuery, partitionedQuery]).then(([unpart, part]) => {
        const allCookies = [...unpart, ...part];

        if (allCookies.length === 0) {
          cookieOutput.value = '';
          showStatus('No cookies found in the store.', true);
          return;
        }

        //Deduplicate cookies
        const uniqueAllCookies = [];
        const seenAllKeys = new Set();
        for (const cookie of allCookies) {
          const key = `${cookie.name}|${cookie.domain}|${cookie.path}`;
          if (!seenAllKeys.has(key)) {
            seenAllKeys.add(key);
            uniqueAllCookies.push(cookie);
          }
        }

        //the fix was to filter cookies in js instead of chrome.cookies.getAll
        const matchedCookies = uniqueAllCookies.filter(cookie => {
          let cookieDomain = cookie.domain;
          if (cookieDomain.startsWith('.')) {
            cookieDomain = cookieDomain.substring(1);
          }

          return hostname === cookieDomain ||
            hostname.endsWith('.' + cookieDomain) ||
            cookieDomain.endsWith('.' + hostname);
        });

        if (matchedCookies.length === 0) {
          cookieOutput.value = '';
          showStatus(`No cookies match the domain ${hostname}.`, true);
          return;
        }

        const cookieString = matchedCookies.map(c => `${c.name}=${c.value}`).join('; ');
        cookieOutput.value = cookieString;
        btnCopy.disabled = false;
      });

    } catch (err) {
      domainText.textContent = 'Invalid URL';
      showStatus(`Error: ${err.message}`, true);
    }
  }

  btnCopy.addEventListener('click', async () => {
    const textToCopy = cookieOutput.value;
    if (!textToCopy) return;

    try {
      await navigator.clipboard.writeText(textToCopy);
      const originalText = btnCopy.innerHTML;
      btnCopy.className = 'btn-primary btn-success';
      btnCopy.innerHTML = `
        <svg width="14" height="14" viewBox="0 0 16 16" fill="currentColor">
          <path d="M13.78 4.22a.75.75 0 0 1 0 1.06l-7.25 7.25a.75.75 0 0 1-1.06 0L2.22 9.28a.751.751 0 0 1 .018-1.042.751.751 0 0 1 1.042-.018L6 10.94l6.72-6.72a.75.75 0 0 1 1.06 0Z"></path>
        </svg>
        Copied!
      `;

      setTimeout(() => {
        btnCopy.className = 'btn-primary';
        btnCopy.innerHTML = originalText;
      }, 2000);

    } catch (err) {
      showStatus('Failed to copy to clipboard.', true);
    }
  });

  btnRefresh.addEventListener('click', loadCookies);

  loadCookies();
});
