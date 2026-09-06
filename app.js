const AREA_NAMES = {
  Sedayu: 'Sedayu City',
  JGC: 'Jakarta Garden City',
  KHI: 'Kota Harapan Indah',
  Metland: 'Metland Menteng'
};

// `icon` is the inner markup of a 24x24 SVG (Material Design geometry), not a letter.
// It is trusted constant markup, so it is injected without escaping — never build one
// of these from user input.
const CATEGORY_META = {
  School: { label: 'School', color: '#3f7ad8', icon: '<path d="M5 13.18v4L12 21l7-3.82v-4L12 17l-7-3.82zM12 3L1 9l11 6 9-4.91V17h2V9L12 3z"/>' },
  Hospital: { label: 'Hospital', color: '#db5457', icon: '<path d="M19 3H5c-1.1 0-2 .9-2 2v14c0 1.1.9 2 2 2h14c1.1 0 2-.9 2-2V5c0-1.1-.9-2-2-2zm-2 11h-3v3h-4v-3H7v-4h3V7h4v3h3v4z"/>' },
  'Showroom Dealer': { label: 'Showroom', color: '#e39b32', icon: '<path d="M18.92 6.01C18.72 5.42 18.16 5 17.5 5h-11c-.66 0-1.21.42-1.42 1.01L3 12v8c0 .55.45 1 1 1h1c.55 0 1-.45 1-1v-1h12v1c0 .55.45 1 1 1h1c.55 0 1-.45 1-1v-8l-2.08-5.99zM6.5 16c-.83 0-1.5-.67-1.5-1.5S5.67 13 6.5 13s1.5.67 1.5 1.5S7.33 16 6.5 16zm11 0c-.83 0-1.5-.67-1.5-1.5s.67-1.5 1.5-1.5 1.5.67 1.5 1.5-.67 1.5-1.5 1.5zM5 11l1.5-4.5h11L19 11H5z"/>' },
  'Gas Station': { label: 'SPBU', color: '#7657c9', icon: '<path d="M19.77 7.23l.01-.01-3.72-3.72L15 4.56l2.11 2.11c-.94.36-1.61 1.26-1.61 2.33 0 1.38 1.12 2.5 2.5 2.5.36 0 .69-.08 1-.21v7.21c0 .55-.45 1-1 1s-1-.45-1-1V14c0-1.1-.9-2-2-2h-1V5c0-1.1-.9-2-2-2H6c-1.1 0-2 .9-2 2v16h10v-7.5h1.5v5c0 1.38 1.12 2.5 2.5 2.5s2.5-1.12 2.5-2.5V9c0-.69-.28-1.32-.73-1.77zM12 10H6V5h6v5zm6 0c-.55 0-1-.45-1-1s.45-1 1-1 1 .45 1 1-.45 1-1 1z"/>' },
  University: { label: 'University', color: '#168b91', icon: '<path d="M4 10v7h3v-7H4zm6 0v7h3v-7h-3zM2 22h19v-3H2v3zm14-12v7h3v-7h-3zm-4.5-9L2 6v2h19V6l-9.5-5z"/>' },
  'Housing Complex': { label: 'Perumahan', color: '#4a9d6e', icon: '<path d="M10 20v-6h4v6h5v-8h3L12 3 2 12h3v8z"/>' },
  Other: { label: 'Lainnya', color: '#68726c', icon: '<circle cx="12" cy="12" r="5"/>' }
};

function categoryIcon(meta) {
  return `<svg class="cat-icon" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">${meta.icon}</svg>`;
}

// Bands are non-overlapping. The stated brackets (<=1, 1-2, 2-3, >=3) collide at
// exactly 1, 2 and 3; each boundary is assigned to the lower bracket.
const PRICE_BANDS = [
  { id: 'lte1', label: '≤ 1 M', test: p => p <= 1 },
  { id: '1to2', label: '1 – 2 M', test: p => p > 1 && p <= 2 },
  { id: '2to3', label: '2 – 3 M', test: p => p > 2 && p <= 3 },
  { id: 'gte3', label: '> 3 M', test: p => p > 3 }
];

// Scaled to the real KMZ figures (Sedayu ~9-10rb, Metland ~10rb, JGC ~20-29rb,
// KHI ~60-125rb). Bounds are (min, max] so a value on a boundary falls in the lower
// band, matching PRICE_BANDS. Population arrives as a RANGE, so a band matches when
// the range overlaps it.
const POPULATION_BANDS = [
  { id: 'lte10k', label: '≤ 10rb', min: -Infinity, max: 10000 },
  { id: '10to30k', label: '10 – 30rb', min: 10000, max: 30000 },
  { id: '30to60k', label: '30 – 60rb', min: 30000, max: 60000 },
  { id: 'gt60k', label: '> 60rb', min: 60000, max: Infinity }
];

// Accepts a number or a {min,max} range; returns the bands it touches.
function populationBandsFor(value) {
  const range = normalizePopulation(value);
  if (!range) return [];
  return POPULATION_BANDS.filter(b => range.max > b.min && range.min <= b.max).map(b => b.id);
}

function normalizePopulation(value) {
  if (Number.isFinite(value)) return { min: value, max: value };
  if (value && Number.isFinite(value.min) && Number.isFinite(value.max)) return value;
  return null;
}

// KMZ labels look like "± 20.000 s/d 29.000 jiwa" or "± 10.000 jiwa".
// Indonesian thousands separator is '.', so digits are joined before parsing.
function parsePopulationLabel(text) {
  const numbers = (String(text).match(/\d[\d.]*/g) || [])
    .map(n => Number(n.replace(/\./g, '')))
    .filter(n => Number.isFinite(n) && n > 0);
  if (!numbers.length) return null;
  return { min: Math.min(...numbers), max: Math.max(...numbers) };
}

function formatPopulation(value) {
  const range = normalizePopulation(value);
  if (!range) return '';
  const fmt = n => n.toLocaleString('id-ID');
  return range.min === range.max ? `${fmt(range.min)} jiwa` : `${fmt(range.min)}–${fmt(range.max)} jiwa`;
}

const CATEGORY_ALIASES = {
  Sekolah: 'School', School: 'School', Hospital: 'Hospital',
  'Showroom Dealer': 'Showroom Dealer', 'Gas Station': 'Gas Station', University: 'University'
};

const FEATURE_FIXES = {
  'SIS JGC': { area: 'JGC', category: 'School' },
  'Suzuki': { area: 'Sedayu', category: 'Showroom Dealer', name: 'Suzuki Sedayu' }
};

// Versioned separately so a malformed value in one key cannot take the others down with it.
const STORAGE_KEY = 'facility-map-manual-pins-v1';
const COMPLEX_KEY = 'facility-map-complexes-v1';
const POPULATION_KEY = 'facility-map-population-v1';
const GROUPS_KEY = 'facility-map-groups-v1';

const state = {
  features: [], boundaries: [], manualFeatures: [], complexes: [],
  areaPopulation: {}, customGroups: [],
  kmzPopulation: {},          // as parsed from the KMZ, so a manual override can be undone
  complexDraft: { latlng: null, catalog: [], custom: {} },
  groupsDraft: [],            // working copy edited by the manage-categories dialog
  kmlDoc: null,               // the parsed KMZ, kept so export mutates it instead of rebuilding
  kmlName: 'doc.kml',
  kmzExtras: {},              // non-KML zip entries, carried through on export
  dirty: false,               // local state has diverged from the last export
  pickTarget: 'pin',          // what a map click in addMode is picking a location for
  selectedArea: null, query: '', addMode: false, draftLogo: '',
  // Empty set = the group is not constraining. See `groupPasses`.
  filters: { facilities: new Set(), price: new Set(), population: new Set() },
  multi: { facilities: true, price: true, population: true }
};
const map = L.map('map', { zoomControl: false, attributionControl: true }).setView([-6.174, 106.963], 13);
L.control.zoom({ position: 'bottomleft' }).addTo(map);
L.tileLayer('https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png', {
  maxZoom: 20,
  attribution: '&copy; OpenStreetMap contributors'
}).addTo(map);

const markerLayer = L.layerGroup().addTo(map);
const boundaryLayer = L.layerGroup().addTo(map);
const markerById = new Map();

// Everything this app writes into the KML lives under one root folder and one set of
// `pm:`-prefixed ExtendedData names, so an export can remove its own previous output
// without touching a single node of the original Google Earth tree.
const PM_FOLDER = 'Property Mapper';
const PM_PREFIX = 'pm:';

function directChildren(parent, tag) {
  return [...parent.children].filter(el => el.localName === tag);
}

// Elements must be created in the document's own namespace; createElement() would emit
// them with xmlns="" and Google Earth would ignore the subtree.
function kmlEl(doc, tag, text) {
  const el = doc.createElementNS(doc.documentElement.namespaceURI, tag);
  if (text !== undefined) el.textContent = text;
  return el;
}

function pmValues(node) {
  const values = {};
  const extended = directChildren(node, 'ExtendedData')[0];
  if (!extended) return values;
  directChildren(extended, 'Data').forEach(data => {
    const name = data.getAttribute('name') || '';
    if (!name.startsWith(PM_PREFIX)) return;
    const value = directChildren(data, 'value')[0];
    values[name.slice(PM_PREFIX.length)] = value ? value.textContent : '';
  });
  return values;
}

function setPmData(doc, container, entries) {
  let extended = directChildren(container, 'ExtendedData')[0];
  if (extended) {
    directChildren(extended, 'Data')
      .filter(d => (d.getAttribute('name') || '').startsWith(PM_PREFIX))
      .forEach(d => extended.removeChild(d));
  }
  const names = Object.keys(entries).filter(k => entries[k] !== undefined && entries[k] !== '');
  if (!names.length) {
    if (extended && !extended.children.length) container.removeChild(extended);
    return;
  }
  if (!extended) { extended = kmlEl(doc, 'ExtendedData'); container.appendChild(extended); }
  names.forEach(name => {
    const data = kmlEl(doc, 'Data');
    data.setAttribute('name', PM_PREFIX + name);
    data.appendChild(kmlEl(doc, 'value', entries[name]));
    extended.appendChild(data);
  });
}

function parseJsonOr(text, fallback) {
  try { const v = JSON.parse(text); return v ?? fallback; } catch { return fallback; }
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

// The "Population" folder holds one point per area whose *name* carries the figure
// (e.g. "± 20.000 s/d 29.000 jiwa"), plus RBI desa boundary polygons. Without this
// branch those points parse as facilities in category Other, inflating 42 pins to 46.
function parsePopulationFolder(folder) {
  directChildren(folder, 'Folder').forEach(sub => {
    const area = normalizeArea(textOf(sub, 'name'));
    if (!area) return;
    const placemark = directChildren(sub, 'Placemark')[0];
    if (!placemark) return;
    const parsed = parsePopulationLabel(textOf(placemark, 'name'));
    if (parsed) { state.areaPopulation[area] = parsed; state.kmzPopulation[area] = parsed; }
  });
}

function adoptOwnPlacemark(pm, own, name, context) {
  const coord = pm.getElementsByTagNameNS('*', 'coordinates')[0];
  if (!coord) return;
  const positions = parseCoordinates(coord.textContent);
  if (!positions.length) return;
  const area = own.area && AREA_NAMES[own.area] ? own.area : context.area;
  if (!area) return;
  const base = {
    id: own.id || `${own.kind}-${crypto.randomUUID()}`,
    name, area, latlng: positions[0],
    custom: parseJsonOr(own.custom, {}),
    logo: own.logo || ''
  };
  if (own.kind === 'housing-complex') {
    state.complexes.push({ ...base, kind: 'complex', category: 'Housing Complex', catalog: parseJsonOr(own.catalog, []) });
  } else if (own.kind === 'manual-pin') {
    state.manualFeatures.push({ ...base, manual: true, category: CATEGORY_META[own.category] ? own.category : 'Other' });
  }
}

function traverseFolder(folder, context = { area: null, category: null }) {
  const folderName = textOf(folder, 'name');
  if (/^(population|populasi)$/i.test(folderName)) { parsePopulationFolder(folder); return; }
  const next = { ...context };
  next.area = normalizeArea(folderName) || next.area;
  next.category = CATEGORY_ALIASES[folderName] || next.category;

  directChildren(folder, 'Placemark').forEach((pm, index) => {
    const name = textOf(pm, 'name') || 'Tanpa nama';
    // Placemarks this app wrote are adopted as complexes / manual pins, never as facilities.
    const own = pmValues(pm);
    if (own.kind) { adoptOwnPlacemark(pm, own, name, next); return; }
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

// Every group — built-in or user-defined — is described by this one shape, so the
// filtering, rendering and reset code never special-cases any of them.
//   optionsOf() -> [{ id, label, color? }]
//   valueOf(f)  -> array of option ids this feature matches; [] means "no value",
//                  which can never intersect a non-empty selection, so a pin
//                  lacking a value is hidden whenever the group is constraining.
function filterGroups() {
  return [
    {
      id: 'facilities', label: 'Fasilitas', builtin: true,
      optionsOf: () => {
        const present = new Set(allFeatures().map(f => f.category));
        return Object.keys(CATEGORY_META).filter(k => present.has(k))
          .map(k => ({ id: k, label: CATEGORY_META[k].label, color: CATEGORY_META[k].color }));
      },
      valueOf: f => [f.category]
    },
    {
      id: 'price', label: 'Range Harga', builtin: true,
      optionsOf: () => PRICE_BANDS.map(b => ({ id: b.id, label: b.label })),
      // A complex spans every bracket its catalog covers — "any unit matches".
      valueOf: f => Array.isArray(f.catalog)
        ? PRICE_BANDS.filter(b => f.catalog.some(u => Number.isFinite(u.price) && b.test(u.price))).map(b => b.id)
        : []
    },
    {
      id: 'population', label: 'Populasi', builtin: true,
      optionsOf: () => POPULATION_BANDS.map(b => ({ id: b.id, label: b.label })),
      // Population is an attribute of the township, inherited by everything in it.
      valueOf: f => populationBandsFor(state.areaPopulation[f.area])
    },
    ...state.customGroups.map(g => ({
      id: g.id, label: g.label, builtin: false,
      optionsOf: () => g.options,
      valueOf: f => (f.custom && f.custom[g.id]) ? [f.custom[g.id]] : []
    }))
  ];
}

function groupPasses(group, feature) {
  const active = state.filters[group.id];
  if (!active || active.size === 0) return true;
  return group.valueOf(feature).some(v => active.has(v));
}

function visibleFeatures() {
  const q = state.query.toLowerCase();
  const groups = filterGroups();
  return allFeatures().filter(f =>
    (!state.selectedArea || f.area === state.selectedArea) &&
    (!q || f.name.toLowerCase().includes(q) || AREA_NAMES[f.area].toLowerCase().includes(q)) &&
    groups.every(g => groupPasses(g, f))
  );
}

function escapeHtml(value) {
  return String(value).replace(/[&<>'"]/g, char => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', "'": '&#39;', '"': '&quot;' }[char]));
}

function allFeatures() { return [...state.features, ...state.manualFeatures, ...state.complexes]; }

function readStored(key, fallback) {
  try {
    const value = JSON.parse(localStorage.getItem(key) || 'null');
    return value === null ? fallback : value;
  } catch { return fallback; }
}

// Seed-if-absent: with no stored key the KMZ is the source and gets persisted as the
// working copy; once the key exists it is authoritative, so deletions survive a reload.
function loadManualFeatures() {
  const saved = readStored(STORAGE_KEY, null);
  if (saved === null) { saveManualFeatures(); return; }
  state.manualFeatures = Array.isArray(saved) ? saved.filter(f => f && AREA_NAMES[f.area] && CATEGORY_META[f.category] && Array.isArray(f.latlng)) : [];
}

function saveManualFeatures() { localStorage.setItem(STORAGE_KEY, JSON.stringify(state.manualFeatures)); }

function loadComplexes() {
  const saved = readStored(COMPLEX_KEY, null);
  if (saved === null) { saveComplexes(); return; }
  state.complexes = Array.isArray(saved)
    ? saved.filter(c => c && AREA_NAMES[c.area] && Array.isArray(c.latlng) && Array.isArray(c.catalog))
      .map(c => ({ ...c, kind: 'complex', category: 'Housing Complex' }))
    : [];
}

function saveComplexes() { localStorage.setItem(COMPLEX_KEY, JSON.stringify(state.complexes)); }

// Merged OVER whatever the KMZ supplied rather than replacing it, so a manual edit
// wins but an empty localStorage never wipes the imported figures.
function loadAreaPopulation() {
  const saved = readStored(POPULATION_KEY, {});
  if (!saved || typeof saved !== 'object') return;
  Object.keys(AREA_NAMES).forEach(key => {
    const raw = saved[key];
    if (raw === undefined || raw === null) return;
    const parsed = normalizePopulation(typeof raw === 'object' ? raw : Number(raw));
    if (parsed && parsed.min >= 0) state.areaPopulation[key] = parsed;
  });
}

function saveAreaPopulation() { localStorage.setItem(POPULATION_KEY, JSON.stringify(state.areaPopulation)); }

function loadCustomGroups() {
  const saved = readStored(GROUPS_KEY, null);
  if (saved !== null) {
    state.customGroups = Array.isArray(saved)
      ? saved.filter(g => g && g.id && g.label && Array.isArray(g.options))
      : [];
  } else {
    saveCustomGroups();
  }
  // A custom group needs a selection set the moment it exists, or groupPasses reads undefined.
  state.customGroups.forEach(g => {
    if (!state.filters[g.id]) state.filters[g.id] = new Set();
    if (state.multi[g.id] === undefined) state.multi[g.id] = true;
  });
}

function saveCustomGroups() { localStorage.setItem(GROUPS_KEY, JSON.stringify(state.customGroups)); }

function makeMarker(feature) {
  const meta = CATEGORY_META[feature.category] || CATEGORY_META.Other;
  const logo = feature.logo ? `<img src="${escapeHtml(feature.logo)}" alt="" />` : `<i>${categoryIcon(meta)}</i>`;
  const icon = L.divIcon({
    className: '', iconSize: [32, 32], iconAnchor: [16, 32], popupAnchor: [0, -30],
    html: `<div class="marker-pin" style="background:${meta.color}">${logo}</div>`
  });
  const isComplex = feature.kind === 'complex';
  const badge = isComplex ? ' · Perumahan' : (feature.manual ? ' · Pin manual' : '');
  return L.marker(feature.latlng, { icon, title: feature.name }).bindPopup(
    `<div class="popup-category" style="color:${meta.color}">${meta.label}</div>` +
    `<h3 class="popup-name">${escapeHtml(feature.name)}</h3>` +
    `<div class="popup-area">${AREA_NAMES[feature.area]}${badge}</div>` +
    (isComplex ? catalogSummary(feature) : '') +
    (isComplex || feature.manual
      ? `<button class="popup-edit" data-edit-id="${escapeHtml(feature.id)}">${isComplex ? 'Edit perumahan' : 'Edit pin / logo'}</button>`
      : '')
  );
}

function formatPrice(value) {
  return `${value.toLocaleString('id-ID', { maximumFractionDigits: 2 })} M`;
}

function catalogSummary(complex) {
  const prices = (complex.catalog || []).map(u => u.price).filter(Number.isFinite);
  if (!prices.length) return '';
  const low = Math.min(...prices), high = Math.max(...prices);
  const span = low === high ? formatPrice(low) : `${formatPrice(low)} – ${formatPrice(high)}`;
  return `<div class="popup-catalog">${complex.catalog.length} tipe unit · ${escapeHtml(span)}</div>`;
}

function renderMap() {
  markerLayer.clearLayers(); boundaryLayer.clearLayers(); markerById.clear();
  const visible = visibleFeatures();
  state.boundaries.filter(b => !state.selectedArea || b.area === state.selectedArea).forEach(b => {
    L.polygon(b.latlngs, { color: '#d45d5d', weight: 2, opacity: .82, fillColor: '#f3a6a6', fillOpacity: .5, dashArray: '7 5' })
      .bindTooltip(AREA_NAMES[b.area], { sticky: true })
      .addTo(boundaryLayer);
  });
  // No per-marker popupopen binding: one delegated listener on the map container
  // handles every Edit button, so reopening a popup cannot stack handlers.
  visible.forEach(f => {
    markerById.set(f.id, makeMarker(f).addTo(markerLayer));
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
      <span class="result-icon" style="background:${meta.color}">${f.logo ? `<img src="${escapeHtml(f.logo)}" alt="" />` : categoryIcon(meta)}</span>
      <span><strong>${escapeHtml(f.name)}</strong><small>${AREA_NAMES[f.area]} · ${meta.label}${f.manual ? ' · Manual' : ''}</small></span>
      <span class="result-arrow">›</span>
    </button>`;
  }).join('');
  list.querySelectorAll('.result-item').forEach(btn => btn.addEventListener('click', () => {
    // allFeatures(), not state.features — manual pins and complexes are clickable too.
    const feature = allFeatures().find(f => f.id === btn.dataset.id);
    const marker = markerById.get(btn.dataset.id);
    if (feature && marker) {
      map.flyTo(feature.latlng, 17, { duration: .8 });
      setTimeout(() => marker.openPopup(), 700);
      document.getElementById('sidebar').classList.remove('open');
    }
  }));
}

function renderControls() {
  renderDeveloperList();
  renderFilterGroups();
  renderLegend();
  syncControls();
}

function renderDeveloperList() {
  document.getElementById('developerList').innerHTML = Object.entries(AREA_NAMES).map(([key, label]) => {
    const count = allFeatures().filter(f => f.area === key).length;
    const population = formatPopulation(state.areaPopulation[key]);
    const meta = `${count} fasilitas${population ? ` · ${population}` : ''}`;
    // The populasi control is a sibling, not a child: nesting a button inside a button
    // is invalid HTML and the inner one would not receive clicks reliably.
    return `<div class="developer-item">
      <button class="developer-button" data-area="${escapeHtml(key)}"><span class="dev-code">${key === 'Metland' ? 'MTL' : escapeHtml(key)}</span><span>${escapeHtml(label)}<small>${meta}</small></span></button>
      <button class="developer-more" data-pop-area="${escapeHtml(key)}" title="Set populasi ${escapeHtml(label)}" aria-label="Set populasi ${escapeHtml(label)}">
        <svg viewBox="0 0 24 24" fill="currentColor" aria-hidden="true"><path d="M16 11c1.66 0 2.99-1.34 2.99-3S17.66 5 16 5c-1.66 0-3 1.34-3 3s1.34 3 3 3zm-8 0c1.66 0 2.99-1.34 2.99-3S9.66 5 8 5C6.34 5 5 6.34 5 8s1.34 3 3 3zm0 2c-2.33 0-7 1.17-7 3.5V19h14v-2.5c0-2.33-4.67-3.5-7-3.5zm8 0c-.29 0-.62.02-.97.05 1.16.84 1.97 1.97 1.97 3.45V19h6v-2.5c0-2.33-4.67-3.5-7-3.5z"/></svg>
      </button>
    </div>`;
  }).join('');
}

function renderFilterGroups() {
  document.getElementById('filterGroups').innerHTML = filterGroups().map(group => {
    const options = group.optionsOf();
    if (!options.length) return '';
    const active = state.filters[group.id] || new Set();
    const isMulti = !!state.multi[group.id];
    const chips = options.map(o =>
      `<button class="category-chip${active.has(o.id) ? ' active' : ''}" data-group="${escapeHtml(group.id)}" data-option="${escapeHtml(o.id)}">` +
      `${o.color ? `<span class="chip-dot" style="background:${o.color}"></span>` : ''}${escapeHtml(o.label)}</button>`
    ).join('');
    return `<div class="filter-group">
      <div class="group-head">
        <h3>${escapeHtml(group.label)}</h3>
        <div class="group-actions">
          <button class="multi-toggle${isMulti ? ' on' : ''}" data-multi="${escapeHtml(group.id)}" role="switch" aria-checked="${isMulti}" title="Pilih lebih dari satu">Multi</button>
          <button class="reset-button" data-reset="${escapeHtml(group.id)}"${active.size ? '' : ' disabled'}>Reset</button>
        </div>
      </div>
      <div class="category-list">${chips}</div>
    </div>`;
  }).join('');
}

function renderLegend() {
  const options = filterGroups().find(g => g.id === 'facilities').optionsOf();
  document.getElementById('legend').innerHTML = options.map(o =>
    `<span><i class="chip-dot" style="background:${o.color}"></i>${escapeHtml(o.label)}</span>`).join('');
}

function toggleOption(groupId, optionId) {
  const active = state.filters[groupId];
  if (!active) return;
  if (state.multi[groupId]) {
    active.has(optionId) ? active.delete(optionId) : active.add(optionId);
  } else {
    // Single-select: a click replaces the selection; clicking the sole active chip clears it.
    const wasOnlySelection = active.size === 1 && active.has(optionId);
    active.clear();
    if (!wasOnlySelection) active.add(optionId);
  }
  renderFilterGroups(); renderMap();
}

function resetGroup(groupId) {
  const active = state.filters[groupId];
  if (!active) return;
  active.clear();
  renderFilterGroups(); renderMap();
}

function toggleMulti(groupId) {
  state.multi[groupId] = !state.multi[groupId];
  const active = state.filters[groupId];
  // Collapsing to single-select must leave at most one selection, or the UI would
  // show chips the mode cannot produce.
  if (!state.multi[groupId] && active && active.size > 1) {
    const first = active.values().next().value;
    active.clear(); active.add(first);
  }
  renderFilterGroups(); renderMap();
}

function syncControls() {
  document.querySelectorAll('.developer-button').forEach(btn => btn.classList.toggle('active', btn.dataset.area === state.selectedArea));
  document.getElementById('mapTitle').textContent = state.selectedArea ? AREA_NAMES[state.selectedArea] : 'Semua kawasan';
}

// animate: true glides (developer toggle, "Lihat semua"); false snaps (first paint).
function fitVisible({ animate = false } = {}) {
  const points = visibleFeatures().map(f => f.latlng);
  const boundaryPoints = state.boundaries.filter(b => !state.selectedArea || b.area === state.selectedArea).flatMap(b => b.latlngs);
  const all = [...points, ...boundaryPoints];
  if (!all.length) return;
  const bounds = L.latLngBounds(all);
  const options = { padding: [55, 55], maxZoom: 15 };
  if (animate) map.flyToBounds(bounds, { ...options, duration: .9 });
  else map.fitBounds(bounds, options);
}

async function loadData() {
  try {
    const response = await fetch('data/facility-mapping.kmz');
    if (!response.ok) throw new Error('KMZ tidak ditemukan');
    const zip = await JSZip.loadAsync(await response.arrayBuffer());
    const kmlFile = zip.file('doc.kml') || Object.values(zip.files).find(f => f.name.endsWith('.kml'));
    if (!kmlFile) throw new Error('KML tidak ditemukan');
    state.kmlName = kmlFile.name;
    // Non-KML entries (icons, overlays) are carried through untouched on export.
    await Promise.all(Object.values(zip.files)
      .filter(file => !file.dir && file.name !== kmlFile.name)
      .map(async file => { state.kmzExtras[file.name] = await file.async('uint8array'); }));

    const xml = new DOMParser().parseFromString(await kmlFile.async('text'), 'application/xml');
    if (xml.getElementsByTagName('parsererror').length) throw new Error('KML tidak valid');
    state.kmlDoc = xml;
    const rootDocument = xml.getElementsByTagNameNS('*', 'Document')[0];
    directChildren(rootDocument, 'Folder').forEach(folder => traverseFolder(folder));

    // Read after the walk so an exported value wins over the name-derived Population folder.
    const rootOwn = pmValues(rootDocument);
    if (rootOwn.groups) state.customGroups = parseJsonOr(rootOwn.groups, []);
    if (rootOwn.population) {
      const saved = parseJsonOr(rootOwn.population, {});
      Object.keys(AREA_NAMES).forEach(area => {
        const parsed = normalizePopulation(saved[area]);
        if (parsed) { state.areaPopulation[area] = parsed; state.kmzPopulation[area] = parsed; }
      });
    }

    loadManualFeatures(); loadComplexes(); loadAreaPopulation(); loadCustomGroups();
    renderControls(); renderMap(); fitVisible();
    document.getElementById('loading').classList.add('hidden');
  } catch (error) {
    console.error(error);
    document.getElementById('loading').classList.add('hidden');
    document.getElementById('errorCard').classList.remove('hidden');
  }
}

function setAddMode(active, target = 'pin') {
  state.addMode = active;
  if (active) state.pickTarget = target;
  document.getElementById('addModeLabel').textContent = target === 'complex'
    ? 'Klik lokasi perumahan di dalam boundary kawasan' : 'Klik lokasi pin baru di peta';
  document.getElementById('addMode').classList.remove('warn');
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
  renderCustomGroupSelects('pinCustomGroups', feature?.custom || {});
  document.getElementById('deletePin').classList.toggle('hidden', !feature);
  document.getElementById('pinDialog').showModal();
}

function closePinDialog() { document.getElementById('pinDialog').close(); }

/* ---------- area context menu ---------- */

// Ray casting over the boundary ring. Hit-testing by geography rather than by DOM target
// means a right-click still resolves to its township when it lands on a marker, a
// tooltip, or any other layer stacked above the polygon.
function pointInRing(latlng, ring) {
  const x = latlng.lng, y = latlng.lat;
  let inside = false;
  for (let i = 0, j = ring.length - 1; i < ring.length; j = i++) {
    const xi = ring[i][1], yi = ring[i][0];
    const xj = ring[j][1], yj = ring[j][0];
    if ((yi > y) !== (yj > y) && x < ((xj - xi) * (y - yi)) / (yj - yi) + xi) inside = !inside;
  }
  return inside;
}

// Every township, regardless of the current filter — used to validate a saved location,
// which must stay correct even while a different area is selected.
function areaContaining(latlng, boundaries = state.boundaries) {
  const hit = boundaries.find(b => pointInRing(latlng, b.latlngs));
  return hit ? hit.area : null;
}

// Only the boundaries currently drawn, so neither the menu nor pick-mode can resolve to
// an area the user cannot see.
function areaAt(latlng) {
  return areaContaining(latlng, state.boundaries.filter(b => !state.selectedArea || b.area === state.selectedArea));
}

function openAreaMenu(area, originalEvent) {
  const menu = document.getElementById('areaMenu');
  menu.innerHTML =
    `<div class="area-menu-title">${escapeHtml(AREA_NAMES[area])}</div>` +
    `<button type="button" role="menuitem" data-menu="complex">＋ Tambah perumahan</button>` +
    `<button type="button" role="menuitem" data-menu="population">Set populasi</button>`;
  menu.dataset.area = area;
  const rect = document.getElementById('map').getBoundingClientRect();
  // Clamped so the menu never opens past the map's right/bottom edge.
  const x = Math.min(originalEvent.clientX - rect.left, rect.width - 190);
  const y = Math.min(originalEvent.clientY - rect.top, rect.height - 110);
  menu.style.left = `${Math.max(8, x)}px`;
  menu.style.top = `${Math.max(8, y)}px`;
  menu.classList.remove('hidden');
}

function closeAreaMenu() { document.getElementById('areaMenu').classList.add('hidden'); }

/* ---------- housing complex dialog ---------- */

function blankCatalogRow() { return { lt: '', lb: '', price: '' }; }

function renderCatalogRows(rows) {
  document.getElementById('catalogBody').innerHTML = rows.map((row, i) => `
    <tr>
      <td><input type="number" step="0.01" min="0" data-cat="lt" data-row="${i}" value="${row.lt ?? ''}" /></td>
      <td><input type="number" step="0.01" min="0" data-cat="lb" data-row="${i}" value="${row.lb ?? ''}" /></td>
      <td><input type="number" step="0.01" min="0" data-cat="price" data-row="${i}" value="${row.price ?? ''}" /></td>
      <td><button type="button" class="row-remove" data-remove="${i}" aria-label="Hapus baris">×</button></td>
    </tr>`).join('');
}

function readCatalogRows() {
  return [...document.querySelectorAll('#catalogBody tr')].map(tr => {
    const get = key => tr.querySelector(`[data-cat="${key}"]`).value;
    return { lt: get('lt'), lb: get('lb'), price: get('price') };
  });
}

// A row counts only when LT, LB and Harga are all present and greater than zero.
function completeCatalogRows(rows) {
  return rows.map(r => ({ lt: Number(r.lt), lb: Number(r.lb), price: Number(r.price) }))
    .filter(r => [r.lt, r.lb, r.price].every(n => Number.isFinite(n) && n > 0))
    .map(r => ({ lt: round2(r.lt), lb: round2(r.lb), price: round2(r.price) }));
}

function round2(n) { return Math.round(n * 100) / 100; }

// Inline message rather than alert(), and the save button stays disabled until valid.
function validateComplexForm() {
  const rows = readCatalogRows();
  const complete = completeCatalogRows(rows);
  const named = document.getElementById('complexName').value.trim().length > 0;
  const latlng = state.complexDraft.latlng;
  const located = Array.isArray(latlng);
  const chosenArea = document.getElementById('complexArea').value;
  // Checked against every boundary, not just the drawn ones, so editing a complex while
  // a different area is filtered does not falsely fail.
  const containing = located ? areaContaining({ lat: latlng[0], lng: latlng[1] }) : null;
  const hint = document.getElementById('catalogHint');
  let message = '';
  if (!named) message = 'Nama perumahan wajib diisi.';
  else if (!located) message = 'Pilih lokasi perumahan di peta.';
  else if (containing !== chosenArea) {
    message = containing
      ? `Lokasi berada di dalam ${AREA_NAMES[containing]}, bukan ${AREA_NAMES[chosenArea]}.`
      : `Lokasi berada di luar boundary ${AREA_NAMES[chosenArea]}.`;
  }
  else if (!complete.length) message = 'Isi minimal satu baris katalog dengan LT, LB, dan Harga lebih dari 0.';
  hint.textContent = message;
  hint.classList.toggle('error', !!message);
  document.getElementById('saveComplex').disabled = !!message;
  return message ? null : complete;
}

// Shared by the complex dialog and the manual pin dialog.
function renderCustomGroupSelects(containerId, selected = {}) {
  document.getElementById(containerId).innerHTML = state.customGroups.map(group => `
    <label>${escapeHtml(group.label)}<select data-custom="${escapeHtml(group.id)}">
      <option value="">— tidak diisi —</option>
      ${group.options.map(o => `<option value="${escapeHtml(o.id)}"${selected[group.id] === o.id ? ' selected' : ''}>${escapeHtml(o.label)}</option>`).join('')}
    </select></label>`).join('');
}

function readCustomGroupSelects(containerId) {
  const custom = {};
  document.querySelectorAll(`#${containerId} [data-custom]`).forEach(select => {
    if (select.value) custom[select.dataset.custom] = select.value;
  });
  return custom;
}

function openComplexDialog(draft) {
  closeAreaMenu();
  setAddMode(false);
  state.complexDraft = {
    id: draft.id || null,
    name: draft.name || '',
    area: draft.area || state.selectedArea || 'JGC',
    latlng: draft.latlng || null,
    catalog: (draft.catalog && draft.catalog.length ? draft.catalog : [blankCatalogRow()]),
    custom: draft.custom || {},
    logo: draft.logo || ''
  };
  const d = state.complexDraft;
  document.getElementById('complexDialogTitle').textContent = d.id ? 'Edit perumahan' : 'Tambah perumahan';
  document.getElementById('complexId').value = d.id || '';
  document.getElementById('complexName').value = d.name;
  document.getElementById('complexArea').value = d.area;
  document.getElementById('complexLatLng').textContent = d.latlng
    ? `${d.latlng[0].toFixed(5)}, ${d.latlng[1].toFixed(5)}` : '—';
  renderCatalogRows(d.catalog);
  renderCustomGroupSelects('complexCustomGroups', d.custom);
  document.getElementById('deleteComplex').classList.toggle('hidden', !d.id);
  validateComplexForm();
  document.getElementById('complexDialog').showModal();
}

// Keeps the draft current so a trip through "Pilih di peta" does not lose typed input.
function syncComplexDraftFromForm() {
  state.complexDraft.name = document.getElementById('complexName').value;
  state.complexDraft.area = document.getElementById('complexArea').value;
  state.complexDraft.catalog = readCatalogRows();
  state.complexDraft.custom = readCustomGroupSelects('complexCustomGroups');
}

function closeComplexDialog() { document.getElementById('complexDialog').close(); }

/* ---------- population dialog ---------- */

function openPopulationDialog(area) {
  closeAreaMenu();
  const current = normalizePopulation(state.areaPopulation[area]);
  document.getElementById('populationDialogTitle').textContent = `Populasi ${AREA_NAMES[area]}`;
  document.getElementById('populationArea').value = area;
  document.getElementById('populationMin').value = current ? current.min : '';
  document.getElementById('populationMax').value = current && current.max !== current.min ? current.max : '';
  document.getElementById('resetPopulation').classList.toggle('hidden', !state.kmzPopulation[area]);
  document.getElementById('populationDialog').showModal();
}

function closePopulationDialog() { document.getElementById('populationDialog').close(); }

/* ---------- KMZ write-back ---------- */

let kmzFileHandle = null;   // held so the second save in a session needs no picker

function markDirty() {
  state.dirty = true;
  document.getElementById('kmzDirty').classList.remove('hidden');
}

function clearDirty() {
  state.dirty = false;
  document.getElementById('kmzDirty').classList.add('hidden');
}

function buildOwnPlacemark(doc, feature, kind) {
  const placemark = kmlEl(doc, 'Placemark');
  placemark.appendChild(kmlEl(doc, 'name', feature.name));
  const point = kmlEl(doc, 'Point');
  // KML is lng,lat,alt — the reverse of Leaflet's [lat, lng].
  point.appendChild(kmlEl(doc, 'coordinates', `${feature.latlng[1]},${feature.latlng[0]},0`));
  placemark.appendChild(point);
  const entries = {
    kind, id: feature.id, area: feature.area,
    custom: JSON.stringify(feature.custom || {}),
    logo: feature.logo || ''
  };
  if (kind === 'housing-complex') entries.catalog = JSON.stringify(feature.catalog || []);
  else entries.category = feature.category;
  setPmData(doc, placemark, entries);
  return placemark;
}

function appendAreaGrouped(doc, parent, label, items, kind) {
  if (!items.length) return;
  const section = kmlEl(doc, 'Folder');
  section.appendChild(kmlEl(doc, 'name', label));
  Object.keys(AREA_NAMES).forEach(area => {
    const inArea = items.filter(item => item.area === area);
    if (!inArea.length) return;
    const areaFolder = kmlEl(doc, 'Folder');
    areaFolder.appendChild(kmlEl(doc, 'name', area));
    inArea.forEach(item => areaFolder.appendChild(buildOwnPlacemark(doc, item, kind)));
    section.appendChild(areaFolder);
  });
  parent.appendChild(section);
}

// Mutates the parsed document in place: the original styles, descriptions and folder
// structure survive, which regenerating the KML from scratch would silently drop.
function buildExportDocument() {
  const doc = state.kmlDoc;
  const root = doc.getElementsByTagNameNS('*', 'Document')[0];

  // Idempotent: drop the previous export's subtree before writing the current one.
  directChildren(root, 'Folder')
    .filter(folder => textOf(folder, 'name') === PM_FOLDER)
    .forEach(folder => root.removeChild(folder));

  setPmData(doc, root, {
    groups: JSON.stringify(state.customGroups),
    population: JSON.stringify(state.areaPopulation)
  });

  if (state.complexes.length || state.manualFeatures.length) {
    const folder = kmlEl(doc, 'Folder');
    folder.appendChild(kmlEl(doc, 'name', PM_FOLDER));
    appendAreaGrouped(doc, folder, 'Perumahan', state.complexes, 'housing-complex');
    appendAreaGrouped(doc, folder, 'Pin Manual', state.manualFeatures, 'manual-pin');
    root.appendChild(folder);
  }
  return doc;
}

async function buildKmzBlob() {
  const xml = new XMLSerializer().serializeToString(buildExportDocument());
  const zip = new JSZip();
  zip.file(state.kmlName, xml);
  Object.entries(state.kmzExtras).forEach(([name, bytes]) => zip.file(name, bytes));
  return zip.generateAsync({ type: 'blob', compression: 'DEFLATE' });
}

// Chrome/Edge overwrite the real file after a one-time grant; Firefox/Safari download it.
async function writeKmzBlob(blob) {
  if (window.showSaveFilePicker) {
    if (!kmzFileHandle) {
      kmzFileHandle = await window.showSaveFilePicker({
        suggestedName: 'facility-mapping.kmz',
        types: [{ description: 'KMZ', accept: { 'application/vnd.google-earth.kmz': ['.kmz'] } }]
      });
    }
    const writable = await kmzFileHandle.createWritable();
    await writable.write(blob);
    await writable.close();
    return 'saved';
  }
  const url = URL.createObjectURL(blob);
  const link = document.createElement('a');
  link.href = url;
  link.download = 'facility-mapping.kmz';
  document.body.appendChild(link);
  link.click();
  link.remove();
  setTimeout(() => URL.revokeObjectURL(url), 1000);
  return 'downloaded';
}

async function saveToKmz() {
  const button = document.getElementById('saveKmzButton');
  const note = document.getElementById('storageNote');
  if (!state.kmlDoc) { note.textContent = 'KMZ belum dimuat.'; return; }
  button.disabled = true;
  try {
    const outcome = await writeKmzBlob(await buildKmzBlob());
    clearDirty();
    note.textContent = outcome === 'saved'
      ? 'Tersimpan ke file KMZ.'
      : 'KMZ diunduh — ganti file di folder data/.';
  } catch (error) {
    // AbortError just means the user dismissed the picker.
    if (error && error.name === 'AbortError') return;
    console.error(error);
    note.textContent = 'Gagal menyimpan KMZ.';
  } finally {
    button.disabled = false;
  }
}

/* ---------- manage custom categories ---------- */

// Only hand-authored features carry tags; imported facilities are rebuilt from the KMZ
// on every load, so a tag written onto one would silently vanish.
function taggableFeatures() { return [...state.manualFeatures, ...state.complexes]; }

function openGroupsDialog() {
  // Edited as a working copy so Cancel is a real cancel.
  state.groupsDraft = JSON.parse(JSON.stringify(state.customGroups));
  renderGroupsEditor();
  document.getElementById('groupsDialog').showModal();
}

function closeGroupsDialog() { document.getElementById('groupsDialog').close(); }

function renderGroupsEditor() {
  const editor = document.getElementById('groupsEditor');
  if (!state.groupsDraft.length) {
    editor.innerHTML = '<div class="empty">Belum ada kategori kustom.</div>';
    validateGroupsForm();
    return;
  }
  editor.innerHTML = state.groupsDraft.map((group, gi) => `
    <div class="group-editor">
      <div class="group-editor-head">
        <input data-group-label="${gi}" value="${escapeHtml(group.label)}" maxlength="40" placeholder="Nama kategori" aria-label="Nama kategori" />
        <button type="button" class="row-remove" data-group-remove="${gi}" aria-label="Hapus kategori">×</button>
      </div>
      <div class="group-editor-options">
        ${group.options.map((option, oi) => `
          <div class="option-row">
            <input data-option-label="${gi}:${oi}" value="${escapeHtml(option.label)}" maxlength="40" placeholder="Nama subkategori" aria-label="Nama subkategori" />
            <button type="button" class="row-remove" data-option-remove="${gi}:${oi}" aria-label="Hapus subkategori">×</button>
          </div>`).join('')}
      </div>
      <button type="button" class="text-button" data-option-add="${gi}">＋ Tambah subkategori</button>
    </div>`).join('');
  validateGroupsForm();
}

// Reads the live inputs back into the draft so a re-render never loses typing.
function syncGroupsDraftFromForm() {
  document.querySelectorAll('[data-group-label]').forEach(input => {
    state.groupsDraft[Number(input.dataset.groupLabel)].label = input.value;
  });
  document.querySelectorAll('[data-option-label]').forEach(input => {
    const [gi, oi] = input.dataset.optionLabel.split(':').map(Number);
    state.groupsDraft[gi].options[oi].label = input.value;
  });
}

function validateGroupsForm() {
  const draft = state.groupsDraft;
  let message = '';
  if (draft.some(g => !g.label.trim())) message = 'Setiap kategori butuh nama.';
  else if (draft.some(g => !g.options.length)) message = 'Setiap kategori butuh minimal satu subkategori.';
  else if (draft.some(g => g.options.some(o => !o.label.trim()))) message = 'Setiap subkategori butuh nama.';
  const hint = document.getElementById('groupsHint');
  hint.textContent = message;
  hint.classList.toggle('error', !!message);
  document.getElementById('saveGroups').disabled = !!message;
  return !message;
}

// Ids are generated once and survive renames, so retagging is never needed — the same
// reasoning that keeps category out of the refactor's import_key.
function applyGroupsDraft() {
  const draft = state.groupsDraft.map(g => ({
    id: g.id, label: g.label.trim(),
    options: g.options.map(o => ({ id: o.id, label: o.label.trim() }))
  }));

  const survivingGroups = new Set(draft.map(g => g.id));
  const survivingOptions = new Map(draft.map(g => [g.id, new Set(g.options.map(o => o.id))]));

  // Strip tags whose group or option no longer exists, so nothing orphaned accumulates.
  taggableFeatures().forEach(feature => {
    if (!feature.custom) return;
    Object.keys(feature.custom).forEach(groupId => {
      if (!survivingGroups.has(groupId) || !survivingOptions.get(groupId).has(feature.custom[groupId])) {
        delete feature.custom[groupId];
      }
    });
  });

  // Drop filter state for removed groups; prune selections pointing at removed options.
  Object.keys(state.filters).forEach(id => {
    if (['facilities', 'price', 'population'].includes(id)) return;
    if (!survivingGroups.has(id)) { delete state.filters[id]; delete state.multi[id]; return; }
    [...state.filters[id]].forEach(optionId => {
      if (!survivingOptions.get(id).has(optionId)) state.filters[id].delete(optionId);
    });
  });

  state.customGroups = draft;
  state.customGroups.forEach(g => {
    if (!state.filters[g.id]) state.filters[g.id] = new Set();
    if (state.multi[g.id] === undefined) state.multi[g.id] = true;
  });

  saveCustomGroups(); saveManualFeatures(); saveComplexes();
}

document.getElementById('pinArea').innerHTML = Object.entries(AREA_NAMES).map(([key, label]) => `<option value="${key}">${label}</option>`).join('');
// 'Housing Complex' is excluded: complexes are created by right-clicking an area, not
// as a manual pin, and they carry a unit catalog this dialog does not collect.
document.getElementById('pinCategory').innerHTML = Object.entries(CATEGORY_META)
  .filter(([key]) => key !== 'Housing Complex')
  .map(([key, meta]) => `<option value="${key}">${meta.label}</option>`).join('');

// Bound once, never inside a render, so repeated renders cannot stack handlers.
document.getElementById('developerList').addEventListener('click', event => {
  const populationButton = event.target.closest('[data-pop-area]');
  if (populationButton) { openPopulationDialog(populationButton.dataset.popArea); return; }
  const btn = event.target.closest('[data-area]');
  if (!btn) return;
  state.selectedArea = state.selectedArea === btn.dataset.area ? null : btn.dataset.area;
  syncControls(); renderMap(); fitVisible({ animate: true });
});

document.getElementById('filterGroups').addEventListener('click', event => {
  const chip = event.target.closest('[data-option]');
  if (chip) return toggleOption(chip.dataset.group, chip.dataset.option);
  const reset = event.target.closest('[data-reset]');
  if (reset) return resetGroup(reset.dataset.reset);
  const multi = event.target.closest('[data-multi]');
  if (multi) return toggleMulti(multi.dataset.multi);
});

// One delegated listener for every popup Edit button (bug: popupopen used to bind per reopen).
document.getElementById('map').addEventListener('click', event => {
  const btn = event.target.closest('[data-edit-id]');
  if (!btn) return;
  const feature = allFeatures().find(f => f.id === btn.dataset.editId);
  if (!feature) return;
  if (feature.kind === 'complex') openComplexDialog(feature); else openPinDialog(feature);
});

document.getElementById('complexArea').innerHTML = Object.entries(AREA_NAMES)
  .map(([key, label]) => `<option value="${key}">${label}</option>`).join('');

document.getElementById('areaMenu').addEventListener('click', event => {
  const btn = event.target.closest('[data-menu]');
  if (!btn) return;
  const area = document.getElementById('areaMenu').dataset.area;
  closeAreaMenu();
  if (btn.dataset.menu === 'complex') openComplexDialog({ area });
  else openPopulationDialog(area);
});

// Dismiss the menu on any outside interaction, Escape, or map movement.
document.addEventListener('click', event => {
  // event.target is not always an Element (it can be document), so .closest may not exist.
  const target = event.target;
  if (!(target instanceof Element) || !target.closest('#areaMenu')) closeAreaMenu();
});
document.addEventListener('keydown', event => { if (event.key === 'Escape') closeAreaMenu(); });
map.on('movestart zoomstart', closeAreaMenu);

// One map-level handler decides everything, so there is no dependence on whether a layer
// or the map receives the event first — the previous per-polygon binding opened the menu
// and a map-level close handler immediately hid it again.
map.on('contextmenu', event => {
  const area = areaAt(event.latlng);
  if (!area) { closeAreaMenu(); return; }
  L.DomEvent.preventDefault(event.originalEvent);
  openAreaMenu(area, event.originalEvent);
});

document.getElementById('catalogBody').addEventListener('click', event => {
  const remove = event.target.closest('[data-remove]');
  if (!remove) return;
  const rows = readCatalogRows();
  rows.splice(Number(remove.dataset.remove), 1);
  state.complexDraft.catalog = rows.length ? rows : [blankCatalogRow()];
  renderCatalogRows(state.complexDraft.catalog);
  validateComplexForm();
});
document.getElementById('catalogBody').addEventListener('input', validateComplexForm);
document.getElementById('complexName').addEventListener('input', validateComplexForm);
document.getElementById('complexArea').addEventListener('change', validateComplexForm);

document.getElementById('addCatalogRow').addEventListener('click', () => {
  state.complexDraft.catalog = [...readCatalogRows(), blankCatalogRow()];
  renderCatalogRows(state.complexDraft.catalog);
  validateComplexForm();
});

document.getElementById('pickComplexLocation').addEventListener('click', () => {
  syncComplexDraftFromForm();
  closeComplexDialog();
  setAddMode(true, 'complex');
  document.getElementById('sidebar').classList.remove('open');
});

document.getElementById('closeComplexDialog').addEventListener('click', closeComplexDialog);
document.getElementById('cancelComplex').addEventListener('click', closeComplexDialog);

document.getElementById('complexForm').addEventListener('submit', event => {
  event.preventDefault();
  syncComplexDraftFromForm();
  const catalog = validateComplexForm();
  if (!catalog) return;
  const draft = state.complexDraft;
  const complex = {
    id: draft.id || `complex-${crypto.randomUUID()}`,
    kind: 'complex', category: 'Housing Complex',
    name: draft.name.trim(), area: draft.area, latlng: draft.latlng,
    catalog, custom: draft.custom, logo: draft.logo || ''
  };
  const index = state.complexes.findIndex(c => c.id === complex.id);
  if (index >= 0) state.complexes[index] = complex; else state.complexes.push(complex);
  saveComplexes(); markDirty(); closeComplexDialog(); renderControls(); renderMap();
  map.flyTo(complex.latlng, 16, { duration: .7 });
});

document.getElementById('deleteComplex').addEventListener('click', () => {
  const id = document.getElementById('complexId').value;
  if (!id || !confirm('Hapus perumahan ini?')) return;
  state.complexes = state.complexes.filter(c => c.id !== id);
  saveComplexes(); markDirty(); closeComplexDialog(); renderControls(); renderMap();
});

document.getElementById('manageGroupsButton').addEventListener('click', () => {
  openGroupsDialog();
  document.getElementById('sidebar').classList.remove('open');
});
document.getElementById('closeGroupsDialog').addEventListener('click', closeGroupsDialog);
document.getElementById('cancelGroups').addEventListener('click', closeGroupsDialog);

document.getElementById('addGroup').addEventListener('click', () => {
  syncGroupsDraftFromForm();
  state.groupsDraft.push({
    id: `group-${crypto.randomUUID()}`, label: '',
    options: [{ id: `option-${crypto.randomUUID()}`, label: '' }]
  });
  renderGroupsEditor();
});

document.getElementById('groupsEditor').addEventListener('input', () => {
  syncGroupsDraftFromForm();
  validateGroupsForm();
});

document.getElementById('groupsEditor').addEventListener('click', event => {
  const target = event.target;
  const groupRemove = target.closest('[data-group-remove]');
  const optionRemove = target.closest('[data-option-remove]');
  const optionAdd = target.closest('[data-option-add]');
  if (!groupRemove && !optionRemove && !optionAdd) return;
  syncGroupsDraftFromForm();
  if (groupRemove) {
    state.groupsDraft.splice(Number(groupRemove.dataset.groupRemove), 1);
  } else if (optionRemove) {
    const [gi, oi] = optionRemove.dataset.optionRemove.split(':').map(Number);
    state.groupsDraft[gi].options.splice(oi, 1);
  } else {
    state.groupsDraft[Number(optionAdd.dataset.optionAdd)].options
      .push({ id: `option-${crypto.randomUUID()}`, label: '' });
  }
  renderGroupsEditor();
});

document.getElementById('groupsForm').addEventListener('submit', event => {
  event.preventDefault();
  syncGroupsDraftFromForm();
  if (!validateGroupsForm()) return;
  applyGroupsDraft(); markDirty();
  closeGroupsDialog(); renderControls(); renderMap();
});

document.getElementById('saveKmzButton').addEventListener('click', saveToKmz);
document.getElementById('closePopulationDialog').addEventListener('click', closePopulationDialog);
document.getElementById('cancelPopulation').addEventListener('click', closePopulationDialog);

document.getElementById('populationForm').addEventListener('submit', event => {
  event.preventDefault();
  const area = document.getElementById('populationArea').value;
  const min = Number(document.getElementById('populationMin').value);
  const rawMax = document.getElementById('populationMax').value;
  const max = rawMax === '' ? min : Number(rawMax);
  if (!Number.isFinite(min) || min < 0 || !Number.isFinite(max) || max < min) return;
  state.areaPopulation[area] = { min, max };
  saveAreaPopulation(); markDirty(); closePopulationDialog(); renderControls(); renderMap();
});

document.getElementById('resetPopulation').addEventListener('click', () => {
  const area = document.getElementById('populationArea').value;
  const original = state.kmzPopulation[area];
  if (!original) return;
  state.areaPopulation[area] = original;
  // Dropped from the override store so the KMZ stays authoritative on reload.
  const saved = readStored(POPULATION_KEY, {});
  if (saved && typeof saved === 'object') { delete saved[area]; localStorage.setItem(POPULATION_KEY, JSON.stringify(saved)); }
  markDirty(); closePopulationDialog(); renderControls(); renderMap();
});
document.getElementById('addPinButton').addEventListener('click', () => { setAddMode(true); document.getElementById('sidebar').classList.remove('open'); });
document.getElementById('addComplexButton').addEventListener('click', () => {
  state.complexDraft = { id: null, name: '', area: state.selectedArea || 'JGC', latlng: null, catalog: [], custom: {}, logo: '' };
  setAddMode(true, 'complex');
  document.getElementById('sidebar').classList.remove('open');
});
document.getElementById('cancelAddMode').addEventListener('click', () => setAddMode(false));
map.on('click', e => {
  if (!state.addMode) return;
  if (state.pickTarget === 'complex') {
    // A perumahan must sit inside a township, so an outside click is rejected and
    // pick-mode stays active rather than opening a dialog that cannot be saved.
    const area = areaAt(e.latlng);
    if (!area) {
      const banner = document.getElementById('addMode');
      banner.classList.add('warn');
      document.getElementById('addModeLabel').textContent = 'Klik di dalam boundary kawasan (area berwarna).';
      return;
    }
    state.complexDraft.latlng = [e.latlng.lat, e.latlng.lng];
    state.complexDraft.area = area;
    openComplexDialog(state.complexDraft);
  } else {
    openPinDialog(null, e.latlng);
  }
});
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
  const feature = { id, manual: true, name: document.getElementById('pinName').value.trim(), area: document.getElementById('pinArea').value, category: document.getElementById('pinCategory').value, latlng: [Number(document.getElementById('pinLat').value), Number(document.getElementById('pinLng').value)], logo: state.draftLogo, custom: readCustomGroupSelects('pinCustomGroups') };
  if (!feature.name || feature.latlng.some(n => !Number.isFinite(n))) return;
  const index = state.manualFeatures.findIndex(f => f.id === id);
  if (index >= 0) state.manualFeatures[index] = feature; else state.manualFeatures.push(feature);
  saveManualFeatures(); markDirty(); closePinDialog(); renderControls(); renderMap();
  map.flyTo(feature.latlng, 17, { duration: .7 });
});
document.getElementById('deletePin').addEventListener('click', () => {
  const id = document.getElementById('pinId').value;
  if (!id || !confirm('Hapus pin manual ini?')) return;
  state.manualFeatures = state.manualFeatures.filter(f => f.id !== id);
  saveManualFeatures(); markDirty(); closePinDialog(); renderControls(); renderMap();
});

document.getElementById('searchInput').addEventListener('input', e => { state.query = e.target.value.trim(); renderMap(); });
document.getElementById('showAllDevelopers').addEventListener('click', () => { state.selectedArea = null; syncControls(); renderMap(); fitVisible({ animate: true }); });
document.getElementById('fitMap').addEventListener('click', () => fitVisible({ animate: true }));
document.getElementById('openSidebar').addEventListener('click', () => document.getElementById('sidebar').classList.add('open'));
document.getElementById('closeSidebar').addEventListener('click', () => document.getElementById('sidebar').classList.remove('open'));

loadData();
