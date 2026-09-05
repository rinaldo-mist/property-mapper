const AREA_NAMES = {
  Sedayu: 'Sedayu City',
  JGC: 'Jakarta Garden City',
  KHI: 'Kota Harapan Indah',
  Metland: 'Metland Menteng'
};

const CATEGORY_META = {
  School: { label: 'School', color: '#3f7ad8', icon: 'S' },
  Hospital: { label: 'Hospital', color: '#db5457', icon: '+' },
  'Showroom Dealer': { label: 'Showroom', color: '#e39b32', icon: 'D' },
  'Gas Station': { label: 'SPBU', color: '#7657c9', icon: 'F' },
  University: { label: 'University', color: '#168b91', icon: 'U' },
  Other: { label: 'Lainnya', color: '#68726c', icon: '•' }
};

const CATEGORY_ALIASES = {
  Sekolah: 'School', School: 'School', Hospital: 'Hospital',
  'Showroom Dealer': 'Showroom Dealer', 'Gas Station': 'Gas Station', University: 'University'
};

const FEATURE_FIXES = {
  'SIS JGC': { area: 'JGC', category: 'School' },
  'Suzuki': { area: 'Sedayu', category: 'Showroom Dealer', name: 'Suzuki Sedayu' }
};

const STORAGE_KEY = 'facility-map-manual-pins-v1';
const state = { features: [], boundaries: [], manualFeatures: [], selectedArea: null, activeCategories: new Set(), query: '', addMode: false, draftLogo: '' };
const map = L.map('map', { zoomControl: false, attributionControl: true }).setView([-6.174, 106.963], 13);
L.control.zoom({ position: 'bottomleft' }).addTo(map);
L.tileLayer('https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png', {
  maxZoom: 20,
  attribution: '&copy; OpenStreetMap contributors'
}).addTo(map);

const markerLayer = L.layerGroup().addTo(map);
const boundaryLayer = L.layerGroup().addTo(map);
const markerById = new Map();

function directChildren(parent, tag) {
  return [...parent.children].filter(el => el.localName === tag);
}

function textOf(parent, tag) {
  const el = directChildren(parent, tag)[0];
  return el ? el.textContent.trim() : '';
}

function parseCoordinates(raw) {
  return raw.trim().split(/\s+/).map(pair => {
    const [lng, lat] = pair.split(',').map(Number);
    return [lat, lng];
  }).filter(([lat, lng]) => Number.isFinite(lat) && Number.isFinite(lng));
}

function normalizeArea(name) {
  if (/sedayu/i.test(name)) return 'Sedayu';
  if (/metland/i.test(name)) return 'Metland';
  if (/\bKHI\b|harapan indah/i.test(name)) return 'KHI';
  if (/\bJGC\b|jakarta garden/i.test(name)) return 'JGC';
  return null;
}

function traverseFolder(folder, context = { area: null, category: null }) {
  const folderName = textOf(folder, 'name');
  const next = { ...context };
  next.area = normalizeArea(folderName) || next.area;
  next.category = CATEGORY_ALIASES[folderName] || next.category;

  directChildren(folder, 'Placemark').forEach((pm, index) => {
    const name = textOf(pm, 'name') || 'Tanpa nama';
    const point = pm.getElementsByTagNameNS('*', 'Point')[0];
    const polygon = pm.getElementsByTagNameNS('*', 'Polygon')[0];
    if (point) {
      const coord = point.getElementsByTagNameNS('*', 'coordinates')[0];
      if (!coord) return;
      const positions = parseCoordinates(coord.textContent);
      if (!positions.length) return;
      const fix = FEATURE_FIXES[name] || {};
      const guessedArea = fix.area || next.area || normalizeArea(name) || null;
      if (!guessedArea) return;
      state.features.push({
        id: `${guessedArea}-${name}-${index}`,
        name: fix.name || name,
        area: guessedArea,
        category: fix.category || next.category || 'Other',
        latlng: positions[0]
      });
    }
    if (polygon) {
      const coord = polygon.getElementsByTagNameNS('*', 'coordinates')[0];
      const area = next.area || normalizeArea(name);
      if (coord && area) state.boundaries.push({ area, name, latlngs: parseCoordinates(coord.textContent) });
    }
  });
  directChildren(folder, 'Folder').forEach(child => traverseFolder(child, next));
  directChildren(folder, 'Document').forEach(doc => {
    directChildren(doc, 'Placemark').forEach(pm => {
      const polygon = pm.getElementsByTagNameNS('*', 'Polygon')[0];
      const name = textOf(pm, 'name');
      const coord = polygon?.getElementsByTagNameNS('*', 'coordinates')[0];
      const area = next.area || normalizeArea(name);
      if (coord && area) state.boundaries.push({ area, name, latlngs: parseCoordinates(coord.textContent) });
    });
  });
}

function visibleFeatures() {
  const q = state.query.toLowerCase();
  return [...state.features, ...state.manualFeatures].filter(f =>
    (!state.selectedArea || f.area === state.selectedArea) &&
    state.activeCategories.has(f.category) &&
    (!q || f.name.toLowerCase().includes(q) || AREA_NAMES[f.area].toLowerCase().includes(q))
  );
}

function escapeHtml(value) {
  return String(value).replace(/[&<>'"]/g, char => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', "'": '&#39;', '"': '&quot;' }[char]));
}

function allFeatures() { return [...state.features, ...state.manualFeatures]; }

function loadManualFeatures() {
  try {
    const saved = JSON.parse(localStorage.getItem(STORAGE_KEY) || '[]');
    state.manualFeatures = Array.isArray(saved) ? saved.filter(f => f && AREA_NAMES[f.area] && CATEGORY_META[f.category] && Array.isArray(f.latlng)) : [];
  } catch { state.manualFeatures = []; }
}

function saveManualFeatures() { localStorage.setItem(STORAGE_KEY, JSON.stringify(state.manualFeatures)); }

function makeMarker(feature) {
  const meta = CATEGORY_META[feature.category] || CATEGORY_META.Other;
  const logo = feature.logo ? `<img src="${escapeHtml(feature.logo)}" alt="" />` : `<i>${escapeHtml(meta.icon)}</i>`;
  const icon = L.divIcon({
    className: '', iconSize: [32, 32], iconAnchor: [16, 32], popupAnchor: [0, -30],
    html: `<div class="marker-pin" style="background:${meta.color}">${logo}</div>`
  });
  return L.marker(feature.latlng, { icon, title: feature.name }).bindPopup(
    `<div class="popup-category" style="color:${meta.color}">${meta.label}</div>` +
    `<h3 class="popup-name">${escapeHtml(feature.name)}</h3>` +
    `<div class="popup-area">${AREA_NAMES[feature.area]}${feature.manual ? ' · Pin manual' : ''}</div>` +
    (feature.manual ? `<button class="popup-edit" data-edit-id="${escapeHtml(feature.id)}">Edit pin / logo</button>` : '')
  );
}

function renderMap() {
  markerLayer.clearLayers(); boundaryLayer.clearLayers(); markerById.clear();
  const visible = visibleFeatures();
  state.boundaries.filter(b => !state.selectedArea || b.area === state.selectedArea).forEach(b => {
    L.polygon(b.latlngs, { color: '#d45d5d', weight: 2, opacity: .82, fillColor: '#f3a6a6', fillOpacity: .5, dashArray: '7 5' })
      .bindTooltip(AREA_NAMES[b.area], { sticky: true }).addTo(boundaryLayer);
  });
  visible.forEach(f => {
    const marker = makeMarker(f).addTo(markerLayer);
    if (f.manual) marker.on('popupopen', () => {
      document.querySelector(`[data-edit-id="${CSS.escape(f.id)}"]`)?.addEventListener('click', () => openPinDialog(f));
    });
    markerById.set(f.id, marker);
  });
  document.getElementById('visibleCount').textContent = `${visible.length} titik`;
  renderResults(visible);
}

function renderResults(features) {
  const list = document.getElementById('resultList');
  if (!features.length) {
    list.innerHTML = '<div class="empty">Tidak ada fasilitas yang cocok dengan filter.</div>';
    return;
  }
  list.innerHTML = features.sort((a,b) => a.name.localeCompare(b.name)).map(f => {
    const meta = CATEGORY_META[f.category] || CATEGORY_META.Other;
    return `<button class="result-item" data-id="${f.id}">
      <span class="result-icon" style="background:${meta.color}">${f.logo ? `<img src="${escapeHtml(f.logo)}" alt="" />` : meta.icon}</span>
      <span><strong>${escapeHtml(f.name)}</strong><small>${AREA_NAMES[f.area]} · ${meta.label}${f.manual ? ' · Manual' : ''}</small></span>
      <span class="result-arrow">›</span>
    </button>`;
  }).join('');
  list.querySelectorAll('.result-item').forEach(btn => btn.addEventListener('click', () => {
    const feature = state.features.find(f => f.id === btn.dataset.id);
    const marker = markerById.get(btn.dataset.id);
    if (feature && marker) {
      map.flyTo(feature.latlng, 17, { duration: .8 });
      setTimeout(() => marker.openPopup(), 700);
      document.getElementById('sidebar').classList.remove('open');
    }
  }));
}

function renderControls() {
  const developerList = document.getElementById('developerList');
  developerList.innerHTML = Object.entries(AREA_NAMES).map(([key, label]) => {
    const count = allFeatures().filter(f => f.area === key).length;
    return `<button class="developer-button" data-area="${key}"><span class="dev-code">${key === 'Metland' ? 'MTL' : key}</span><span>${label}<small>${count} fasilitas</small></span></button>`;
  }).join('');
  developerList.querySelectorAll('button').forEach(btn => btn.addEventListener('click', () => {
    state.selectedArea = state.selectedArea === btn.dataset.area ? null : btn.dataset.area;
    syncControls(); renderMap(); fitVisible();
  }));

  const categories = [...new Set(allFeatures().map(f => f.category))];
  categories.forEach(c => state.activeCategories.add(c));
  document.getElementById('categoryList').innerHTML = categories.map(c => {
    const meta = CATEGORY_META[c] || CATEGORY_META.Other;
    return `<button class="category-chip active" data-category="${c}"><span class="chip-dot" style="background:${meta.color}"></span>${meta.label}</button>`;
  }).join('');
  document.querySelectorAll('.category-chip').forEach(btn => btn.addEventListener('click', () => {
    const c = btn.dataset.category;
    state.activeCategories.has(c) ? state.activeCategories.delete(c) : state.activeCategories.add(c);
    syncControls(); renderMap();
  }));

  document.getElementById('legend').innerHTML = categories.map(c => {
    const meta = CATEGORY_META[c] || CATEGORY_META.Other;
    return `<span><i class="chip-dot" style="background:${meta.color}"></i>${meta.label}</span>`;
  }).join('');
  syncControls();
}

function syncControls() {
  document.querySelectorAll('.developer-button').forEach(btn => btn.classList.toggle('active', btn.dataset.area === state.selectedArea));
  document.querySelectorAll('.category-chip').forEach(btn => btn.classList.toggle('active', state.activeCategories.has(btn.dataset.category)));
  document.getElementById('mapTitle').textContent = state.selectedArea ? AREA_NAMES[state.selectedArea] : 'Semua kawasan';
}

function fitVisible() {
  const points = visibleFeatures().map(f => f.latlng);
  const boundaryPoints = state.boundaries.filter(b => !state.selectedArea || b.area === state.selectedArea).flatMap(b => b.latlngs);
  const all = [...points, ...boundaryPoints];
  if (all.length) map.fitBounds(L.latLngBounds(all), { padding: [55, 55], maxZoom: 15 });
}

async function loadData() {
  try {
    const response = await fetch('data/facility-mapping.kmz');
    if (!response.ok) throw new Error('KMZ tidak ditemukan');
    const zip = await JSZip.loadAsync(await response.arrayBuffer());
    const kmlFile = zip.file('doc.kml') || Object.values(zip.files).find(f => f.name.endsWith('.kml'));
    if (!kmlFile) throw new Error('KML tidak ditemukan');
    const xml = new DOMParser().parseFromString(await kmlFile.async('text'), 'application/xml');
    const rootDocument = xml.getElementsByTagNameNS('*', 'Document')[0];
    directChildren(rootDocument, 'Folder').forEach(folder => traverseFolder(folder));
    loadManualFeatures(); renderControls(); renderMap(); fitVisible();
    document.getElementById('loading').classList.add('hidden');
  } catch (error) {
    console.error(error);
    document.getElementById('loading').classList.add('hidden');
    document.getElementById('errorCard').classList.remove('hidden');
  }
}

function setAddMode(active) {
  state.addMode = active;
  document.getElementById('addMode').classList.toggle('hidden', !active);
  document.getElementById('map').classList.toggle('map-picking', active);
}

function updateLogoPreview() {
  const preview = document.getElementById('logoPreview');
  preview.innerHTML = state.draftLogo ? `<img src="${escapeHtml(state.draftLogo)}" alt="Preview logo" />` : 'Tanpa logo';
}

function openPinDialog(feature = null, latlng = null) {
  setAddMode(false);
  document.getElementById('dialogTitle').textContent = feature ? 'Edit fasilitas' : 'Tambah fasilitas';
  document.getElementById('pinId').value = feature?.id || '';
  document.getElementById('pinName').value = feature?.name || '';
  document.getElementById('pinArea').value = feature?.area || state.selectedArea || 'JGC';
  document.getElementById('pinCategory').value = feature?.category || 'Showroom Dealer';
  document.getElementById('pinLat').value = feature?.latlng?.[0] ?? latlng?.lat ?? '';
  document.getElementById('pinLng').value = feature?.latlng?.[1] ?? latlng?.lng ?? '';
  document.getElementById('pinLogo').value = '';
  state.draftLogo = feature?.logo || '';
  updateLogoPreview();
  document.getElementById('deletePin').classList.toggle('hidden', !feature);
  document.getElementById('pinDialog').showModal();
}

function closePinDialog() { document.getElementById('pinDialog').close(); }

document.getElementById('pinArea').innerHTML = Object.entries(AREA_NAMES).map(([key, label]) => `<option value="${key}">${label}</option>`).join('');
document.getElementById('pinCategory').innerHTML = Object.entries(CATEGORY_META).map(([key, meta]) => `<option value="${key}">${meta.label}</option>`).join('');
document.getElementById('addPinButton').addEventListener('click', () => { setAddMode(true); document.getElementById('sidebar').classList.remove('open'); });
document.getElementById('cancelAddMode').addEventListener('click', () => setAddMode(false));
map.on('click', e => { if (state.addMode) openPinDialog(null, e.latlng); });
document.getElementById('closePinDialog').addEventListener('click', closePinDialog);
document.getElementById('cancelPin').addEventListener('click', closePinDialog);
document.getElementById('removeLogo').addEventListener('click', () => { state.draftLogo = ''; updateLogoPreview(); });
document.getElementById('pinLogo').addEventListener('change', event => {
  const file = event.target.files[0];
  if (!file) return;
  if (file.size > 500000) { alert('Ukuran logo maksimal 500 KB.'); event.target.value = ''; return; }
  const reader = new FileReader();
  reader.onload = () => { state.draftLogo = reader.result; updateLogoPreview(); };
  reader.readAsDataURL(file);
});
document.getElementById('pinForm').addEventListener('submit', event => {
  event.preventDefault();
  const id = document.getElementById('pinId').value || `manual-${Date.now()}`;
  const feature = { id, manual: true, name: document.getElementById('pinName').value.trim(), area: document.getElementById('pinArea').value, category: document.getElementById('pinCategory').value, latlng: [Number(document.getElementById('pinLat').value), Number(document.getElementById('pinLng').value)], logo: state.draftLogo };
  if (!feature.name || feature.latlng.some(n => !Number.isFinite(n))) return;
  const index = state.manualFeatures.findIndex(f => f.id === id);
  if (index >= 0) state.manualFeatures[index] = feature; else state.manualFeatures.push(feature);
  saveManualFeatures(); closePinDialog(); renderControls(); renderMap();
  map.flyTo(feature.latlng, 17, { duration: .7 });
});
document.getElementById('deletePin').addEventListener('click', () => {
  const id = document.getElementById('pinId').value;
  if (!id || !confirm('Hapus pin manual ini?')) return;
  state.manualFeatures = state.manualFeatures.filter(f => f.id !== id);
  saveManualFeatures(); closePinDialog(); renderControls(); renderMap();
});

document.getElementById('searchInput').addEventListener('input', e => { state.query = e.target.value.trim(); renderMap(); });
document.getElementById('showAllDevelopers').addEventListener('click', () => { state.selectedArea = null; syncControls(); renderMap(); fitVisible(); });
document.getElementById('fitMap').addEventListener('click', fitVisible);
document.getElementById('openSidebar').addEventListener('click', () => document.getElementById('sidebar').classList.add('open'));
document.getElementById('closeSidebar').addEventListener('click', () => document.getElementById('sidebar').classList.remove('open'));

loadData();
