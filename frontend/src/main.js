import './style.css';
import { 
    SearchAnime, 
    GetEpisodes, 
    SelectDirectory, 
    GetConfig, 
    SaveConfig, 
    StartDownload, 
    GetProgress, 
    ClearProgress,
    GetPosterBase64,
    IsOnline,
    RetryFailed,
    CancelAnimeDownloads,
    FetchCredentialsFromChrome
} from '../wailsjs/go/main/App';

function applyTheme(theme) {
    if (theme === 'solid') {
        document.body.classList.remove('theme-glow');
        document.body.classList.add('theme-solid');
    } else {
        document.body.classList.remove('theme-solid');
        document.body.classList.add('theme-glow');
    }
}

// Load and apply initial theme
const initialTheme = localStorage.getItem('theme') || 'glow';
applyTheme(initialTheme);

// State variables
let currentAnimeTitle = '';
let currentAnimeSlug = '';
let episodeList = [];
const cancelledGroups = new Set();

// DOM References
const tabs = {
    search: document.getElementById('tab-search'),
    downloads: document.getElementById('tab-downloads'),
    settings: document.getElementById('tab-settings')
};

const panels = {
    search: document.getElementById('panel-search'),
    downloads: document.getElementById('panel-downloads'),
    settings: document.getElementById('panel-settings')
};

const searchInput = document.getElementById('search-input');
const searchBtn = document.getElementById('search-btn');
const searchStatus = document.getElementById('search-status');
const searchResults = document.getElementById('search-results');

const downloadsList = document.getElementById('downloads-list');
const downloadBadge = document.getElementById('download-badge');

const settingsForm = document.getElementById('settings-form');
const settingsDomain = document.getElementById('setting-domain');
const settingsUa = document.getElementById('setting-ua');
const settingsCf = document.getElementById('setting-cf');
const settingsDir = document.getElementById('setting-dir');
const settingsQuality = document.getElementById('setting-quality');
const settingsAudio = document.getElementById('setting-audio');
const settingsParallel = document.getElementById('setting-parallel');
const settingsTheme = document.getElementById('setting-theme');
const saveSettingsBtn = document.getElementById('save-settings-btn');
const btnBrowseDir = document.getElementById('btn-browse-dir');
const btnFetchCf = document.getElementById('btn-fetch-cf');
const saveStatus = document.getElementById('save-status');

const episodeModal = document.getElementById('episode-modal');
const modalAnimeTitle = document.getElementById('modal-anime-title');
const modalStatusText = document.getElementById('modal-status-text');
const modalPosterWrapper = document.getElementById('modal-poster-wrapper');
const modalEpisodesList = document.getElementById('modal-episodes-list');
const modalCloseBtn = document.getElementById('modal-close-btn');
const modalCancelBtn = document.getElementById('modal-cancel-btn');
const modalDownloadBtn = document.getElementById('modal-download-btn');
const modalSelectAll = document.getElementById('modal-select-all');
const modalSelectNone = document.getElementById('modal-select-none');

// ----------------------------------------------------
// Tab Switching
// ----------------------------------------------------
function switchTab(tabName) {
    Object.keys(tabs).forEach(name => {
        if (name === tabName) {
            tabs[name].classList.add('active');
            panels[name].classList.add('active');
        } else {
            tabs[name].classList.remove('active');
            panels[name].classList.remove('active');
        }
    });
}

tabs.search.addEventListener('click', () => switchTab('search'));
tabs.downloads.addEventListener('click', () => switchTab('downloads'));
tabs.settings.addEventListener('click', () => switchTab('settings'));

// ----------------------------------------------------
// Settings Handling
// ----------------------------------------------------
async function loadSettings() {
    try {
        const cfg = await GetConfig();
        settingsDomain.value = cfg.domain || 'https://animepahe.pw';
        settingsUa.value = cfg.ua || '';
        settingsCf.value = cfg.cf || '';
        settingsDir.value = cfg.downloadDir || '';
        settingsQuality.value = cfg.quality || '1080';
        settingsAudio.value = cfg.audio || 'jpn';
        settingsParallel.value = String(cfg.maxParallel || 3);
        settingsTheme.value = localStorage.getItem('theme') || 'glow';
    } catch (err) {
        console.error('Failed to load settings:', err);
    }
}

btnBrowseDir.addEventListener('click', async () => {
    try {
        const path = await SelectDirectory();
        if (path) {
            settingsDir.value = path;
        }
    } catch (err) {
        console.error('Directory selection failed:', err);
    }
});

btnFetchCf.addEventListener('click', async () => {
    btnFetchCf.disabled = true;
    const originalText = btnFetchCf.textContent;
    btnFetchCf.textContent = 'Opening Chrome...';
    try {
        const result = await FetchCredentialsFromChrome();
        if (result && result.ua && result.cf) {
            settingsUa.value = result.ua;
            settingsCf.value = result.cf;
            btnFetchCf.textContent = 'Success!';
            btnFetchCf.style.border = '1px solid #10b981';
            btnFetchCf.style.boxShadow = '0 0 10px rgba(16, 185, 129, 0.2)';
            setTimeout(() => {
                btnFetchCf.disabled = false;
                btnFetchCf.textContent = originalText;
                btnFetchCf.style.border = '';
                btnFetchCf.style.boxShadow = '';
            }, 3000);
        } else {
            throw new Error('Retrieved credentials were empty. Make sure you solved the Cloudflare challenge if prompted.');
        }
    } catch (err) {
        alert(`Failed to fetch credentials: ${err}`);
        btnFetchCf.disabled = false;
        btnFetchCf.textContent = originalText;
        btnFetchCf.style.border = '';
        btnFetchCf.style.boxShadow = '';
    }
});

settingsForm.addEventListener('submit', async (e) => {
    e.preventDefault();
    saveStatus.className = 'save-status-msg';
    saveStatus.textContent = 'Saving...';
    try {
        await SaveConfig(
            settingsUa.value.trim(),
            settingsCf.value.trim(),
            settingsDir.value.trim(),
            settingsQuality.value,
            settingsAudio.value,
            settingsDomain.value.trim(),
            parseInt(settingsParallel.value, 10)
        );
        saveStatus.classList.add('success');
        saveStatus.textContent = 'Settings saved successfully!';
        
        // Save and apply the theme changes
        localStorage.setItem('theme', settingsTheme.value);
        applyTheme(settingsTheme.value);

        setTimeout(() => { saveStatus.textContent = ''; }, 3000);
    } catch (err) {
        saveStatus.classList.add('error');
        saveStatus.textContent = `Error: ${err}`;
    }
});

// ----------------------------------------------------
// Search Handling
// ----------------------------------------------------
async function performSearch() {
    const q = searchInput.value.trim();
    if (!q) return;

    searchStatus.textContent = 'Searching...';
    searchResults.innerHTML = '';
    
    try {
        const results = await SearchAnime(q);
        searchStatus.textContent = `Found ${results.length} result(s)`;
        
        if (results.length === 0) {
            searchResults.innerHTML = '<div class="no-results">No anime found matching your query.</div>';
            return;
        }

        results.forEach(async anime => {
            const card = document.createElement('div');
            card.className = 'anime-card';
            card.innerHTML = `
                <div class="anime-poster-wrapper">
                    <div class="anime-poster-placeholder"></div>
                </div>
                <div class="anime-info">
                    <h3>${anime.title}</h3>
                </div>
            `;
            
            card.addEventListener('click', () => {
                openEpisodeModal(anime.title, anime.session, anime.poster);
            });
            
            searchResults.appendChild(card);

            // Fetch and display poster asynchronously in base64
            if (anime.poster) {
                try {
                    const base64Data = await GetPosterBase64(anime.poster);
                    if (base64Data) {
                        const wrapper = card.querySelector('.anime-poster-wrapper');
                        if (wrapper) {
                            wrapper.innerHTML = `<img src="data:image/webp;base64,${base64Data}" class="anime-poster" alt="poster" />`;
                        }
                    }
                } catch (err) {
                    console.error('Failed to load poster base64:', err);
                }
            }
        });
    } catch (err) {
        searchStatus.textContent = `Search failed: ${err}`;
    }
}

searchBtn.addEventListener('click', performSearch);
searchInput.addEventListener('keydown', (e) => {
    if (e.key === 'Enter') performSearch();
});

// ----------------------------------------------------
// Episode Selection Modal
// ----------------------------------------------------
async function openEpisodeModal(title, slug, posterURL) {
    currentAnimeTitle = title;
    currentAnimeSlug = slug;
    modalDownloadBtn.disabled = true;
    
    modalAnimeTitle.textContent = title;
    modalStatusText.textContent = 'Loading episodes list...';
    modalStatusText.style.color = 'var(--text-secondary)';
    modalEpisodesList.innerHTML = '<div style="grid-column: span 4; text-align: center; color: var(--text-secondary);">Loading episodes list...</div>';
    
    // Handle modal header poster dynamically
    modalPosterWrapper.innerHTML = '';
    if (posterURL) {
        modalPosterWrapper.style.display = 'flex';
        modalPosterWrapper.innerHTML = '<div class="anime-poster-placeholder"></div>';
        
        // Fetch and display poster inside the modal header asynchronously
        GetPosterBase64(posterURL).then(base64Data => {
            if (base64Data && currentAnimeSlug === slug) {
                modalPosterWrapper.innerHTML = `<img src="data:image/webp;base64,${base64Data}" class="modal-header-poster" alt="poster" />`;
            }
        }).catch(err => {
            console.error('Failed to load modal poster base64:', err);
            if (currentAnimeSlug === slug) {
                modalPosterWrapper.style.display = 'none';
            }
        });
    } else {
        modalPosterWrapper.style.display = 'none';
    }

    episodeModal.classList.add('active');

    try {
        const [eps, progressList] = await Promise.all([
            GetEpisodes(title, slug),
            GetProgress()
        ]);
        
        episodeList = eps;
        modalEpisodesList.innerHTML = '';

        if (eps.length === 0) {
            modalEpisodesList.innerHTML = '<div style="grid-column: span 4; text-align: center; color: var(--text-secondary);">No episodes found.</div>';
            modalStatusText.textContent = 'No episodes found.';
            return;
        }

        // Keep track of which episodes are currently active in downloads list (queued, downloading, or done)
        const activeOrCompletedEps = new Set();
        progressList.forEach(p => {
            if (p.anime === title) {
                if (p.status === 'queued' || p.status === 'downloading' || p.status === 'done') {
                    activeOrCompletedEps.add(p.epNum);
                }
            }
        });

        const allDownloaded = eps.every(ep => ep.exists || activeOrCompletedEps.has(ep.episode));
        if (allDownloaded) {
            modalStatusText.textContent = 'All episodes are already active or downloaded! ✓';
            modalStatusText.style.color = '#10b981';
        } else {
            modalStatusText.textContent = 'Select episodes to queue for download';
            modalStatusText.style.color = 'var(--text-secondary)';
        }

        eps.forEach(ep => {
            const card = document.createElement('div');
            card.className = 'ep-checkbox-card';
            const isAlreadyActive = activeOrCompletedEps.has(ep.episode);
            const isDisabled = ep.exists || isAlreadyActive;
            
            if (ep.exists) {
                card.classList.add('downloaded');
            } else if (isAlreadyActive) {
                card.classList.add('downloading-active');
            }

            card.innerHTML = `
                <input type="checkbox" id="ep-${ep.episode}" value="${ep.episode}" ${isDisabled ? 'disabled' : ''}>
                <label for="ep-${ep.episode}" class="ep-card-label">
                    <span>E${ep.episode}</span>
                    ${ep.exists ? '<span class="status-badge">✓ Saved</span>' : (isAlreadyActive ? '<span class="status-badge">✓ Active</span>' : '')}
                </label>
            `;
            modalEpisodesList.appendChild(card);
        });

        // Initialize state of the queue button based on initial selection (which is 0 initially)
        updateModalDownloadBtnState();

    } catch (err) {
        modalEpisodesList.innerHTML = `<div style="grid-column: span 4; text-align: center; color: #ef4444;">Failed to fetch episodes: ${err}</div>`;
        modalStatusText.textContent = 'Failed to fetch episodes.';
        modalStatusText.style.color = '#ef4444';
    }
}

function closeEpisodeModal() {
    episodeModal.classList.remove('active');
}

function updateModalDownloadBtnState() {
    const checkedCount = modalEpisodesList.querySelectorAll('input[type="checkbox"]:checked').length;
    modalDownloadBtn.disabled = (checkedCount === 0);
}

modalCloseBtn.addEventListener('click', closeEpisodeModal);
modalCancelBtn.addEventListener('click', closeEpisodeModal);

modalSelectAll.addEventListener('click', () => {
    const checkboxes = modalEpisodesList.querySelectorAll('input[type="checkbox"]:not(:disabled)');
    checkboxes.forEach(cb => cb.checked = true);
    updateModalDownloadBtnState();
});

modalSelectNone.addEventListener('click', () => {
    const checkboxes = modalEpisodesList.querySelectorAll('input[type="checkbox"]');
    checkboxes.forEach(cb => cb.checked = false);
    updateModalDownloadBtnState();
});

modalEpisodesList.addEventListener('change', (e) => {
    if (e.target && e.target.type === 'checkbox') {
        updateModalDownloadBtnState();
    }
});

modalDownloadBtn.addEventListener('click', async () => {
    const checkboxes = modalEpisodesList.querySelectorAll('input[type="checkbox"]:checked');
    if (checkboxes.length === 0) return;

    const epNums = Array.from(checkboxes).map(cb => parseFloat(cb.value));
    
    try {
        await StartDownload(currentAnimeTitle, currentAnimeSlug, epNums);
        closeEpisodeModal();
        switchTab('downloads');
    } catch (err) {
        alert(`Failed to start download: ${err}`);
    }
});

// ----------------------------------------------------
// Downloads Progress Updates
// ----------------------------------------------------
let activeUpdates = true;
const expandedGroups = new Set();

function parseETASeconds(etaStr) {
    if (!etaStr || etaStr === '--') return 0;
    let secs = 0;
    const minSecMatch = etaStr.match(/(\d+)m\s*(\d+)s/);
    if (minSecMatch) {
        secs = parseInt(minSecMatch[1], 10) * 60 + parseInt(minSecMatch[2], 10);
        return secs;
    }
    const secMatch = etaStr.match(/(\d+)s/);
    if (secMatch) {
        return parseInt(secMatch[1], 10);
    }
    const numMatch = etaStr.match(/^(\d+)$/);
    if (numMatch) {
        return parseInt(numMatch[1], 10);
    }
    return 0;
}

function formatSeconds(totalSecs) {
    if (totalSecs <= 0) return '--';
    if (totalSecs < 60) return `${totalSecs}s`;
    const mins = Math.floor(totalSecs / 60);
    const secs = totalSecs % 60;
    return `${mins}m ${secs}s`;
}

async function updateDownloadsProgress() {
    if (!activeUpdates) return;
    try {
        const progressList = await GetProgress();
        
        // Count active/queued downloads for sidebar badge
        let activeCount = 0;
        progressList.forEach(item => {
            if (item.status === 'downloading' || item.status === 'queued') {
                activeCount++;
            }
        });
        
        if (progressList.length === 0) {
            downloadsList.innerHTML = '<div style="color: var(--text-secondary); text-align: center; padding: 40px 0;">No active or past downloads.</div>';
            downloadBadge.style.display = 'none';
            return;
        }

        // Remove placeholder if present
        if (downloadsList.querySelector('div[style*="text-align: center"]')) {
            downloadsList.innerHTML = '';
        }

        // Group by anime name
        const groups = {};
        progressList.forEach(item => {
            const animeName = item.anime || 'Other';
            if (!groups[animeName]) {
                groups[animeName] = {
                    name: animeName,
                    items: [],
                };
            }
            groups[animeName].items.push(item);
        });

        // Clean up cancelledGroups set
        for (const name of cancelledGroups) {
            if (!groups[name]) {
                cancelledGroups.delete(name);
            }
        }

        // Build a set of current active group IDs
        const groupNames = Object.keys(groups).sort();
        const currentGroupDomIds = new Set(groupNames.map(name => `dl-group-${name.replace(/[^a-zA-Z0-9-_]/g, '_')}`));

        // Remove DOM elements that are no longer in the list (e.g. after clearing)
        Array.from(downloadsList.children).forEach(child => {
            if (child.id && child.id.startsWith('dl-group-') && !currentGroupDomIds.has(child.id)) {
                downloadsList.removeChild(child);
            }
        });

        groupNames.forEach((groupName, groupIndex) => {
            const group = groups[groupName];
            const domId = `dl-group-${groupName.replace(/[^a-zA-Z0-9-_]/g, '_')}`;
            let groupEl = document.getElementById(domId);

            // Calculate progress average
            let totalProg = 0;
            group.items.forEach(item => totalProg += item.progress);
            const avgProg = group.items.length > 0 ? totalProg / group.items.length : 0;

            // Calculate aggregate status
            const completedCount = group.items.filter(x => x.status === 'done').length;
            const failedCount = group.items.filter(x => x.status === 'failed').length;
            const downloadingCount = group.items.filter(x => x.status === 'downloading').length;
            const queuedCount = group.items.filter(x => x.status === 'queued').length;

            let statusText = '';
            let statusClass = '';
            if (downloadingCount > 0) {
                statusText = `Downloading (${completedCount}/${group.items.length} done)`;
                statusClass = 'status-downloading';
            } else if (queuedCount > 0) {
                statusText = `Queued (${completedCount}/${group.items.length} done)`;
                statusClass = 'status-queued';
            } else if (failedCount === group.items.length) {
                statusText = 'Failed';
                statusClass = 'status-failed';
            } else if (completedCount === group.items.length) {
                statusText = 'Done';
                statusClass = 'status-done';
            } else if (failedCount > 0) {
                statusText = `Done (${failedCount} failed)`;
                statusClass = 'status-done';
            } else {
                statusText = 'Done';
                statusClass = 'status-done';
            }

            // Calculate aggregate ETA
            let totalRemainingSecs = 0;
            let activeCountInGroup = 0;
            group.items.forEach(item => {
                if (item.status === 'downloading') {
                    totalRemainingSecs += parseETASeconds(item.eta);
                    activeCountInGroup++;
                } else if (item.status === 'queued') {
                    totalRemainingSecs += 90; // estimate 90s per queued episode
                }
            });

            let combinedETASecs = 0;
            if (activeCountInGroup > 0) {
                combinedETASecs = Math.ceil(totalRemainingSecs / activeCountInGroup);
            } else if (queuedCount > 0) {
                combinedETASecs = Math.ceil(totalRemainingSecs / 3);
            }

            let etaText = '--';
            if (combinedETASecs > 0) {
                etaText = formatSeconds(combinedETASecs);
            }

            const showCancel = (downloadingCount > 0 || queuedCount > 0) && !cancelledGroups.has(groupName);

            // Check if group is expanded
            const isExpanded = expandedGroups.has(groupName);

            if (!groupEl) {
                groupEl = document.createElement('div');
                groupEl.id = domId;
                groupEl.className = 'download-group';
                groupEl.innerHTML = `
                    <div class="group-header">
                        <div class="group-info">
                            <span class="group-title">${groupName}</span>
                            <span class="group-status ${statusClass}">${statusText}</span>
                        </div>
                        <div class="group-meta">
                            <button class="btn-cancel" title="Cancel/Remove Downloads" style="display: ${showCancel ? 'flex' : 'none'};">
                                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round" style="width:14px;height:14px;display:block;"><line x1="18" y1="6" x2="6" y2="18"></line><line x1="6" y1="6" x2="18" y2="18"></line></svg>
                            </button>
                            <button class="btn-retry" title="Retry Failed/Stuck Downloads">
                                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round" style="width:14px;height:14px;display:block;"><path d="M21.5 2v6h-6M21.34 15.57a10 10 0 1 1-.57-8.38l5.67-5.67"/></svg>
                            </button>
                            <span class="group-eta">${etaText}</span>
                            <span class="group-expand-icon">${isExpanded ? '▲' : '▼'}</span>
                        </div>
                    </div>
                    <div class="progress-track">
                        <div class="progress-bar" style="width: ${avgProg}%"></div>
                    </div>
                    <div class="group-episodes" style="display: ${isExpanded ? 'flex' : 'none'};"></div>
                `;
                downloadsList.appendChild(groupEl);

                // Add toggle listener
                const header = groupEl.querySelector('.group-header');
                header.addEventListener('click', () => {
                    const episodesDiv = groupEl.querySelector('.group-episodes');
                    const icon = groupEl.querySelector('.group-expand-icon');
                    if (expandedGroups.has(groupName)) {
                        expandedGroups.delete(groupName);
                        episodesDiv.style.display = 'none';
                        icon.textContent = '▼';
                    } else {
                        expandedGroups.add(groupName);
                        episodesDiv.style.display = 'flex';
                        icon.textContent = '▲';
                    }
                });

                // Add retry listener
                const retryBtn = groupEl.querySelector('.btn-retry');
                if (retryBtn) {
                    retryBtn.addEventListener('click', async (e) => {
                        e.stopPropagation(); // prevent toggling the group expansion
                        try {
                            await RetryFailed(groupName);
                            updateDownloadsProgress();
                        } catch (err) {
                            console.error('Failed to retry:', err);
                        }
                    });
                }

                // Add cancel listener
                const cancelBtn = groupEl.querySelector('.btn-cancel');
                if (cancelBtn) {
                    cancelBtn.addEventListener('click', async (e) => {
                        e.stopPropagation(); // prevent toggling the group expansion
                        cancelBtn.style.display = 'none';
                        cancelledGroups.add(groupName);
                        try {
                            await CancelAnimeDownloads(groupName);
                            updateDownloadsProgress();
                        } catch (err) {
                            console.error('Failed to cancel:', err);
                            cancelledGroups.delete(groupName);
                            cancelBtn.style.display = 'flex';
                        }
                    });
                }
            } else {
                // Update in place
                const statusEl = groupEl.querySelector('.group-status');
                if (statusEl) {
                    statusEl.className = `group-status ${statusClass}`;
                    statusEl.textContent = statusText;
                }

                const etaEl = groupEl.querySelector('.group-eta');
                if (etaEl) {
                    etaEl.textContent = etaText;
                }

                const progressBar = groupEl.querySelector('.progress-bar');
                if (progressBar) {
                    progressBar.style.width = `${avgProg}%`;
                }

                const icon = groupEl.querySelector('.group-expand-icon');
                if (icon) {
                    icon.textContent = isExpanded ? '▲' : '▼';
                }

                const episodesDiv = groupEl.querySelector('.group-episodes');
                if (episodesDiv) {
                    episodesDiv.style.display = isExpanded ? 'flex' : 'none';
                }

                const cancelBtn = groupEl.querySelector('.btn-cancel');
                if (cancelBtn) {
                    cancelBtn.style.display = showCancel ? 'flex' : 'none';
                }
            }

            // Maintain correct ordering in the downloadsList DOM
            if (downloadsList.children[groupIndex] !== groupEl) {
                downloadsList.insertBefore(groupEl, downloadsList.children[groupIndex]);
            }

            // Update sub-items
            const episodesDiv = groupEl.querySelector('.group-episodes');
            const epDomIds = new Set(group.items.map(item => `ep-row-${item.id.replace(/[^a-zA-Z0-9-_]/g, '_')}`));

            // Remove old episode rows
            Array.from(episodesDiv.children).forEach(child => {
                if (child.id && child.id.startsWith('ep-row-') && !epDomIds.has(child.id)) {
                    episodesDiv.removeChild(child);
                }
            });

            // Add or update episode rows in sorted order of episodes
            group.items.sort((a, b) => a.epNum - b.epNum);
            group.items.forEach((item, epIndex) => {
                const epDomId = `ep-row-${item.id.replace(/[^a-zA-Z0-9-_]/g, '_')}`;
                let epRow = document.getElementById(epDomId);
                const epLabel = `E${item.epNum}`;

                if (!epRow) {
                    epRow = document.createElement('div');
                    epRow.id = epDomId;
                    epRow.className = `tiny-ep-row${item.status === 'failed' ? ' failed-row' : ''}`;
                    epRow.innerHTML = `
                        <span class="tiny-ep-num">${epLabel}</span>
                        <div class="progress-track tiny-track">
                            <div class="progress-bar" style="width: ${item.progress}%"></div>
                        </div>
                        <span class="tiny-ep-pct">${Math.round(item.progress)}%</span>
                    `;
                    episodesDiv.appendChild(epRow);
                } else {
                    if (item.status === 'failed') {
                        epRow.classList.add('failed-row');
                    } else {
                        epRow.classList.remove('failed-row');
                    }
                    const progressBar = epRow.querySelector('.progress-bar');
                    if (progressBar) {
                        progressBar.style.width = `${item.progress}%`;
                    }
                    const pctEl = epRow.querySelector('.tiny-ep-pct');
                    if (pctEl) {
                        pctEl.textContent = `${Math.round(item.progress)}%`;
                    }
                }

                // Maintain correct ordering inside episodesDiv
                if (episodesDiv.children[epIndex] !== epRow) {
                    episodesDiv.insertBefore(epRow, episodesDiv.children[epIndex]);
                }
            });
        });

        if (activeCount > 0) {
            downloadBadge.textContent = String(activeCount);
            downloadBadge.style.display = 'inline-block';
        } else {
            downloadBadge.style.display = 'none';
        }

    } catch (err) {
        console.error('Error fetching download progress:', err);
    }
}



// Initialization
loadSettings();
updateDownloadsProgress();
setInterval(updateDownloadsProgress, 1000);

// Connectivity checks
let isOnline = true;
async function checkConnectivity() {
    try {
        const online = navigator.onLine && await IsOnline();
        updateOnlineStatus(online);
    } catch (err) {
        updateOnlineStatus(false);
    }
}

function updateOnlineStatus(online) {
    if (isOnline === online) return;
    isOnline = online;
    
    const offlineBanner = document.getElementById('offline-banner');
    if (!online) {
        if (!offlineBanner) {
            const banner = document.createElement('div');
            banner.id = 'offline-banner';
            banner.className = 'offline-banner';
            banner.innerHTML = `
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" style="width:16px;height:16px;flex-shrink:0;"><path d="M10.29 3.86L1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z"></path><line x1="12" y1="9" x2="12" y2="13"></line><line x1="12" y1="17" x2="12.01" y2="17"></line></svg>
                <span>Offline: No internet connection or mirror is unreachable. Search, settings edits, and directory browsing are disabled.</span>
            `;
            document.body.appendChild(banner);
        }
        searchInput.disabled = true;
        searchBtn.disabled = true;
        modalDownloadBtn.disabled = true;
        btnBrowseDir.disabled = true;
        saveSettingsBtn.disabled = true;
        document.body.classList.add('is-offline');
    } else {
        if (offlineBanner) {
            offlineBanner.remove();
        }
        searchInput.disabled = false;
        searchBtn.disabled = false;
        modalDownloadBtn.disabled = false;
        btnBrowseDir.disabled = false;
        saveSettingsBtn.disabled = false;
        document.body.classList.remove('is-offline');
    }
}

checkConnectivity();
setInterval(checkConnectivity, 3000);

if (window.runtime) {
    window.runtime.EventsOn("credentials_updated", (data) => {
        if (data && data.ua && data.cf) {
            settingsUa.value = data.ua;
            settingsCf.value = data.cf;
            
            saveStatus.className = 'save-status-msg success';
            saveStatus.textContent = 'Clearance credentials automatically resolved!';
            setTimeout(() => {
                saveStatus.textContent = '';
            }, 5000);
        }
    });
}
