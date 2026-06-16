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
    GetPosterBase64
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
const clearDownloadsBtn = document.getElementById('clear-downloads-btn');

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
        const eps = await GetEpisodes(title, slug);
        episodeList = eps;
        modalEpisodesList.innerHTML = '';

        if (eps.length === 0) {
            modalEpisodesList.innerHTML = '<div style="grid-column: span 4; text-align: center; color: var(--text-secondary);">No episodes found.</div>';
            modalStatusText.textContent = 'No episodes found.';
            return;
        }

        const allDownloaded = eps.every(ep => ep.exists);
        if (allDownloaded) {
            modalStatusText.textContent = 'All episodes are already downloaded! ✓';
            modalStatusText.style.color = '#10b981';
        } else {
            modalStatusText.textContent = 'Select episodes to queue for download';
            modalStatusText.style.color = 'var(--text-secondary)';
        }

        eps.forEach(ep => {
            const card = document.createElement('div');
            card.className = 'ep-checkbox-card';
            if (ep.exists) {
                card.classList.add('downloaded');
            }

            card.innerHTML = `
                <input type="checkbox" id="ep-${ep.episode}" value="${ep.episode}" ${ep.exists ? 'disabled' : ''}>
                <label for="ep-${ep.episode}" class="ep-card-label">
                    <span>E${ep.episode}</span>
                    ${ep.exists ? '<span class="status-badge">✓ Saved</span>' : ''}
                </label>
            `;
            modalEpisodesList.appendChild(card);
        });
    } catch (err) {
        modalEpisodesList.innerHTML = `<div style="grid-column: span 4; text-align: center; color: #ef4444;">Failed to fetch episodes: ${err}</div>`;
        modalStatusText.textContent = 'Failed to fetch episodes.';
        modalStatusText.style.color = '#ef4444';
    }
}

function closeEpisodeModal() {
    episodeModal.classList.remove('active');
}

modalCloseBtn.addEventListener('click', closeEpisodeModal);
modalCancelBtn.addEventListener('click', closeEpisodeModal);

modalSelectAll.addEventListener('click', () => {
    const checkboxes = modalEpisodesList.querySelectorAll('input[type="checkbox"]:not(:disabled)');
    checkboxes.forEach(cb => cb.checked = true);
});

modalSelectNone.addEventListener('click', () => {
    const checkboxes = modalEpisodesList.querySelectorAll('input[type="checkbox"]');
    checkboxes.forEach(cb => cb.checked = false);
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

async function updateDownloadsProgress() {
    if (!activeUpdates) return;
    try {
        const progressList = await GetProgress();
        
        // Count active/queued downloads for sidebar badge
        let activeCount = 0;
        
        // Sort progress by Anime Title, then Episode Number
        progressList.sort((a, b) => {
            const animeA = a.anime || '';
            const animeB = b.anime || '';
            if (animeA !== animeB) {
                return animeA.localeCompare(animeB);
            }
            return a.epNum - b.epNum;
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

        // Build a set of current active item IDs
        const currentIds = new Set(progressList.map(item => `dl-item-${item.id.replace(/[^a-zA-Z0-9-_]/g, '_')}`));

        // Remove DOM elements that are no longer in the list (e.g. after clearing)
        Array.from(downloadsList.children).forEach(child => {
            if (child.id && child.id.startsWith('dl-item-') && !currentIds.has(child.id)) {
                downloadsList.removeChild(child);
            }
        });

        progressList.forEach((item, index) => {
            if (item.status === 'downloading' || item.status === 'queued') {
                activeCount++;
            }

            const domId = `dl-item-${item.id.replace(/[^a-zA-Z0-9-_]/g, '_')}`;
            let itemEl = document.getElementById(domId);
            const displayTitle = item.anime ? `${item.anime} - E${item.epNum}` : `Episode ${item.epNum}`;
            
            let statusText = item.status;
            let statusClass = `status-${item.status}`;
            if (item.status === 'downloading') {
                statusText = `Downloading ${Math.round(item.progress)}%`;
            }

            let metaText = '';
            if (item.status === 'downloading') {
                metaText = `
                    <span>Speed: ${item.speed || '--'}</span>
                    <span>ETA: ${item.eta || '--'}</span>
                `;
            } else if (item.status === 'failed' && item.error) {
                metaText = `<span style="color: #ef4444; font-size: 0.75rem;">Error: ${item.error}</span>`;
            }

            if (!itemEl) {
                itemEl = document.createElement('div');
                itemEl.id = domId;
                itemEl.className = 'download-item';
                itemEl.innerHTML = `
                    <div class="dl-header">
                        <span class="dl-title">${displayTitle}</span>
                        <span class="dl-status ${statusClass}">${statusText}</span>
                    </div>
                    <div class="progress-track">
                        <div class="progress-bar" style="width: ${item.progress}%"></div>
                    </div>
                    <div class="dl-meta">
                        ${metaText}
                    </div>
                `;
                downloadsList.appendChild(itemEl);
            } else {
                // Update properties in-place if they exist to prevent flickering
                const statusEl = itemEl.querySelector('.dl-status');
                if (statusEl) {
                    statusEl.className = `dl-status ${statusClass}`;
                    statusEl.textContent = statusText;
                }

                const progressBar = itemEl.querySelector('.progress-bar');
                if (progressBar) {
                    progressBar.style.width = `${item.progress}%`;
                }

                const metaEl = itemEl.querySelector('.dl-meta');
                if (metaEl && metaEl.innerHTML !== metaText) {
                    metaEl.innerHTML = metaText;
                }
            }

            // Maintain stable DOM ordering corresponding to the sorted list
            if (downloadsList.children[index] !== itemEl) {
                downloadsList.insertBefore(itemEl, downloadsList.children[index]);
            }
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

clearDownloadsBtn.addEventListener('click', async () => {
    try {
        await ClearProgress();
        updateDownloadsProgress();
    } catch (err) {
        console.error('Failed to clear progress:', err);
    }
});

// Initialization
loadSettings();
updateDownloadsProgress();
setInterval(updateDownloadsProgress, 1000);
