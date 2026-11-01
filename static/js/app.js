'use strict';

/* RefLink frontend: dark single-page UI. No frameworks, no dependencies.
   All dynamic content is written via textContent (never innerHTML), so
   paths or metadata from the server can not inject markup. */

(() => {
  const API = '/api/analyze';
  const knownTypeExpr = /^image\/(jpeg|png|webp|gif|bmp)$/i;
  const knownNameExpr = /\.(jpe?g|png|webp|gif|bmp)$/i;

  const $ = (id) => document.getElementById(id);
  const ui = {
    dz: $('dropzone'),
    dzHint: $('dzHint'),
    input: $('fileInput'),
    message: $('message'),
    busy: $('busy'),
    busyName: $('busyName'),
    preview: $('preview'),
    previewImg: $('previewImg'),
    previewCap: $('previewCap'),
    results: $('results'),
  };

  /* Server-advertised limits, refined from /api/health when it responds. */
  let limits = { maxFileMB: 20, formats: ['JPEG', 'PNG', 'WebP', 'GIF', 'BMP'] };

  fetch('/api/health')
    .then((r) => (r.ok ? r.json() : null))
    .then((cfg) => {
      if (!cfg) return;
      if (Number.isFinite(cfg.max_file_mb) && cfg.max_file_mb > 0) {
        limits.maxFileMB = cfg.max_file_mb;
      }
      if (Array.isArray(cfg.formats) && cfg.formats.length > 0) {
        limits.formats = cfg.formats;
      }
      ui.dzHint.textContent =
        limits.formats.join(' · ') + ' — up to ' + limits.maxFileMB + ' MB';
    })
    .catch(() => { /* defaults stand */ });

  /* ---------- upload wiring ---------- */

  let busy = false;
  const openPicker = () => { if (!busy) ui.input.click(); };

  ui.dz.addEventListener('click', openPicker);
  ui.dz.addEventListener('keydown', (e) => {
    if (e.key === 'Enter' || e.key === ' ') {
      e.preventDefault();
      openPicker();
    }
  });
  ui.input.addEventListener('change', () => {
    if (ui.input.files && ui.input.files[0]) handleFile(ui.input.files[0]);
    ui.input.value = '';
  });

  let dragDepth = 0;
  ui.dz.addEventListener('dragenter', (e) => {
    e.preventDefault();
    dragDepth++;
    ui.dz.classList.add('drag');
  });
  ui.dz.addEventListener('dragover', (e) => e.preventDefault());
  ui.dz.addEventListener('dragleave', () => {
    if (--dragDepth <= 0) {
      dragDepth = 0;
      ui.dz.classList.remove('drag');
    }
  });
  ui.dz.addEventListener('drop', (e) => {
    e.preventDefault();
    dragDepth = 0;
    ui.dz.classList.remove('drag');
    const files = e.dataTransfer && e.dataTransfer.files;
    if (files && files[0]) handleFile(files[0]);
  });

  /* Keep the page from navigating when a file misses the drop zone. */
  window.addEventListener('dragover', (e) => e.preventDefault());
  window.addEventListener('drop', (e) => e.preventDefault());

  /* ---------- state ---------- */

  const setBusy = (on, name) => {
    busy = on;
    ui.busy.classList.toggle('hidden', !on);
    ui.busyName.textContent = on ? name : '';
    ui.dz.classList.toggle('disabled', on);
    if (on) ui.message.classList.add('hidden');
  };

  const showError = (text) => {
    ui.message.textContent = text;
    ui.message.classList.remove('hidden');
  };

  const clearPanel = (node) => {
    node.textContent = '';
    node.classList.add('hidden');
  };

  /* ---------- main flow ---------- */

  async function handleFile(file) {
    if (busy) return;

    const problem = validate(file);
    if (problem) {
      showError(problem);
      return;
    }

    setBusy(true, file.name);
    showPreview(file);
    clearPanel(ui.results);

    const form = new FormData();
    form.append('file', file, file.name);

    try {
      let resp;
      try {
        resp = await fetch(API, { method: 'POST', body: form });
      } catch (err) {
        throw new Error('Network error: the server is unreachable.');
      }

      let body = null;
      try {
        body = await resp.json();
      } catch (err) { /* non-JSON body — fall through to the status text */ }

      if (!resp.ok) {
        throw new Error(
          (body && body.error) || 'Server responded with status ' + resp.status + '.');
      }
      render(body);
    } catch (err) {
      clearPanel(ui.preview);
      showError(String((err && err.message) || err));
    } finally {
      setBusy(false);
    }
  }

  function validate(file) {
    if (file.size === 0) return 'The file is empty.';
    if (!knownTypeExpr.test(file.type) && !knownNameExpr.test(file.name)) {
      return 'Unsupported file type: JPEG, PNG, WebP, GIF or BMP images only.';
    }
    if (file.size > limits.maxFileMB * 1024 * 1024) {
      return 'The file is larger than the ' + limits.maxFileMB + ' MB limit.';
    }
    return '';
  }

  /* ---------- preview ---------- */

  let objectURL = null;

  function showPreview(file) {
    if (objectURL) URL.revokeObjectURL(objectURL);
    objectURL = URL.createObjectURL(file);
    ui.previewImg.src = objectURL;
    ui.previewImg.alt = 'Preview of ' + file.name;
    ui.previewCap.textContent = file.name;
    ui.preview.classList.remove('hidden');
  }

  /* ---------- rendering ---------- */

  const el = (tag, cls) => {
    const n = document.createElement(tag);
    if (cls) n.className = cls;
    return n;
  };

  /* makeCard returns a small builder: row() fills a key/value line,
     note() prints a "nothing here" line. Only text nodes are used. */
  function makeCard(title) {
    const card = el('section', 'card');
    const head = el('h2', 'card-title');
    head.textContent = title;
    const body = el('div', 'card-body');
    card.append(head, body);

    return {
      card,
      body,
      row(label, value) {
        if (value === undefined || value === null || value === '') return null;
        const line = el('div', 'kv');
        const key = el('span', 'k');
        const val = el('span', 'v');
        key.textContent = label;
        val.textContent = String(value);
        line.append(key, val);
        body.appendChild(line);
        return val;
      },
      note(message) {
        const p = el('p', 'note');
        p.textContent = message;
        body.appendChild(p);
      },
      finish() {
        if (!body.querySelector('.kv')) this.note('Not found in file');
      },
    };
  }

  function mapLinkRow(cardBody, label, url) {
    const line = el('div', 'kv');
    const key = el('span', 'k');
    const val = el('span', 'v');
    const a = el('a', '');
    a.href = url;
    a.target = '_blank';
    a.rel = 'noopener noreferrer';
    a.textContent = 'Open on OpenStreetMap ↗';
    key.textContent = label;
    val.appendChild(a);
    line.append(key, val);
    cardBody.appendChild(line);
  }

  async function copyClipboard(content, btn) {
    try {
      await navigator.clipboard.writeText(content);
    } catch (err) {
      /* http:// (no secure context) has no clipboard API — legacy fallback */
      const ta = el('textarea', 'sr-hidden');
      ta.value = content;
      document.body.appendChild(ta);
      ta.select();
      try { document.execCommand('copy'); } catch (err2) { /* best effort */ }
      ta.remove();
    }
    btn.textContent = 'Copied';
    btn.classList.add('ok');
    setTimeout(() => {
      btn.textContent = 'Copy';
      btn.classList.remove('ok');
    }, 1200);
  }

  function copyButton(content) {
    const b = el('button', 'copy-btn');
    b.type = 'button';
    b.textContent = 'Copy';
    b.addEventListener('click', () => copyClipboard(content, b));
    return b;
  }

  const fmtInt = (n) => Number(n).toLocaleString('en-US');
  const fmtMP = (mp) => Number(mp).toLocaleString('en-US', { maximumFractionDigits: 2 }) + ' MP';

  function render(d) {
    if (!d || !d.file || !d.image) {
      throw new Error('Malformed server response.');
    }
    ui.results.textContent = '';

    /* File */
    const f = makeCard('File');
    const hash = f.row('SHA-256', d.file.sha256);
    if (hash) {
      hash.classList.add('mono');
      hash.appendChild(copyButton(d.file.sha256));
    }
    f.row('Name', d.file.name);
    f.row('Size', fmtInt(d.file.size_bytes) + ' B (' + d.file.size_human + ')');

    /* Image */
    const img = d.image;
    const i = makeCard('Image');
    i.row('Format', img.format);
    i.row('Dimensions', img.width + ' × ' + img.height + ' px');
    i.row('Megapixels', fmtMP(img.megapixels));
    i.row('Aspect ratio', img.aspect_ratio);
    i.row('Orientation', img.orientation);
    i.finish();

    /* Camera */
    const ex = d.exif || { present: false, settings: {} };
    const st = ex.settings || {};
    const c = makeCard('Camera');
    c.row('Make', ex.make);
    c.row('Model', ex.model);
    c.row('Lens', ex.lens);
    c.row('Date taken', ex.date_taken);
    c.finish();

    /* Settings */
    const s = makeCard('Settings');
    s.row('ISO', st.iso);
    s.row('Exposure', st.exposure);
    s.row('Aperture', st.aperture);
    s.row('Focal length', st.focal_length);
    s.row('Equivalent (35 mm)', st.focal_length_35mm);
    s.row('Flash', st.flash);
    s.row('Metering', st.metering);
    s.row('White balance', st.white_balance);
    s.row('Software', st.software);
    s.finish();

    /* Location */
    const g = makeCard('Location');
    if (d.gps && d.gps.present) {
      g.row('Latitude', Number(d.gps.latitude).toFixed(6) + '°');
      g.row('Longitude', Number(d.gps.longitude).toFixed(6) + '°');
      if (typeof d.gps.altitude_m === 'number') {
        const below = d.gps.altitude_m < 0;
        const m = Math.abs(d.gps.altitude_m)
          .toLocaleString('en-US', { maximumFractionDigits: 1 });
        g.row('Altitude', m + ' m' + (below ? ' below sea level' : ''));
      }
      g.row('DMS', d.gps.dms);
      mapLinkRow(g.body, 'Map', d.gps.map_url);
    } else {
      g.note('No GPS coordinates in the file');
    }

    ui.results.append(f.card, i.card, c.card, s.card, g.card);
    ui.results.classList.remove('hidden');
  }
})();
