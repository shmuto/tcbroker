(() => {
  'use strict';

  const POLL_MS = 1000;
  const MAX_HISTORY = 30;

  const state = {
    latest: null,
    prevFilterCounters: new Map(),
    prevTimestamp: null,
    ratesByFilter: new Map(),
    ratesByRule: new Map(),
    historyBps: [],
    historyPps: [],
    paused: false,
    selected: null, // { type: 'source'|'rule'|'tool', id: string }
    hovered: null,
    token: sessionStorage.getItem('tcbroker_token') || '',
    pendingAction: null,
  };

  // DOM helpers
  const $ = (id) => document.getElementById(id);

  function escapeHTML(str) {
    return String(str ?? '')
      .replace(/&/g, '&amp;')
      .replace(/</g, '&lt;')
      .replace(/>/g, '&gt;')
      .replace(/"/g, '&quot;');
  }

  function formatBytes(bytes) {
    if (bytes < 1024) return `${bytes} B`;
    const units = ['KB', 'MB', 'GB', 'TB', 'PB'];
    let v = bytes / 1024;
    let i = 0;
    while (v >= 1024 && i < units.length - 1) {
      v /= 1024;
      i++;
    }
    return `${v.toFixed(1)} ${units[i]}`;
  }

  function formatRate(bps) {
    if (bps < 1024) return `${Math.round(bps)} B/s`;
    const units = ['KB/s', 'MB/s', 'GB/s', 'TB/s'];
    let v = bps / 1024;
    let i = 0;
    while (v >= 1024 && i < units.length - 1) {
      v /= 1024;
      i++;
    }
    return `${v.toFixed(1)} ${units[i]}`;
  }

  function formatMatch(f) {
    const parts = [];
    if (f.proto) parts.push(f.proto.toUpperCase());
    if (f.src_ip) parts.push(`src=${f.src_ip}`);
    if (f.dst_ip) parts.push(`dst=${f.dst_ip}`);
    if (f.src_port) parts.push(`sport=${f.src_port}`);
    if (f.dst_port) parts.push(`dport=${f.dst_port}`);
    return parts.length ? parts.join(' ') : 'ALL IPv4';
  }

  function formatRewrite(rw) {
    if (!rw) return '';
    const parts = [];
    if (rw.dst_mac) parts.push(`dmac=${rw.dst_mac}`);
    if (rw.src_mac) parts.push(`smac=${rw.src_mac}`);
    if (rw.dst_ip) parts.push(`dip=${rw.dst_ip}`);
    if (rw.src_ip) parts.push(`sip=${rw.src_ip}`);
    return parts.join(', ');
  }

  let toastTimer = null;
  function showToast(msg, isError = false) {
    const el = $('toast');
    el.textContent = msg;
    el.className = isError ? 'toast toast-error' : 'toast';
    clearTimeout(toastTimer);
    toastTimer = setTimeout(() => el.classList.add('hidden'), 3600);
  }

  // API client
  async function apiRequest(url, options = {}) {
    const headers = Object.assign({ 'Content-Type': 'application/json' }, options.headers || {});
    if (state.token) {
      headers['Authorization'] = `Bearer ${state.token}`;
    }
    const resp = await fetch(url, Object.assign({}, options, { headers }));
    const data = await resp.json().catch(() => ({}));
    if (resp.status === 401) {
      openTokenModal(() => apiRequest(url, options));
      throw new Error(data.error || 'Authentication required');
    }
    if (!resp.ok) {
      throw new Error(data.error || `Request failed (${resp.status})`);
    }
    return data;
  }

  function openTokenModal(retryFn) {
    state.pendingAction = retryFn || null;
    $('f-token').value = '';
    $('modal-token').showModal();
  }

  // Polling & rate computation
  async function refreshStatus() {
    try {
      const data = await apiRequest('/api/v1/status');
      const now = Date.now();
      const elapsedSec = state.prevTimestamp ? Math.max((now - state.prevTimestamp) / 1000, 0.2) : 0;

      const nextFilterCounters = new Map();
      const ratesByFilter = new Map();
      const ratesByRule = new Map();

      let totalPps = 0;
      let totalBps = 0;

      for (const rule of data.rules || []) {
        let rulePps = 0;
        let ruleBps = 0;
        for (const fs of rule.filters || []) {
          const m = fs.managed;
          const key = `${m.src_intf}:${m.hook}:${m.pref}`;
          nextFilterCounters.set(key, { packets: fs.packets, bytes: fs.bytes });

          let pps = 0;
          let bps = 0;
          const prev = state.prevFilterCounters.get(key);
          if (prev && elapsedSec > 0) {
            const dp = fs.packets - prev.packets;
            const db = fs.bytes - prev.bytes;
            if (dp > 0) pps = dp / elapsedSec;
            if (db > 0) bps = db / elapsedSec;
          }
          ratesByFilter.set(key, { pps, bps });
          rulePps += pps;
          ruleBps += bps;
        }
        ratesByRule.set(rule.name, { pps: rulePps, bps: ruleBps });
        totalPps += rulePps;
        totalBps += ruleBps;
      }

      state.prevFilterCounters = nextFilterCounters;
      state.prevTimestamp = now;
      state.ratesByFilter = ratesByFilter;
      state.ratesByRule = ratesByRule;
      state.latest = data;

      state.historyBps.push(totalBps);
      state.historyPps.push(totalPps);
      if (state.historyBps.length > MAX_HISTORY) state.historyBps.shift();
      if (state.historyPps.length > MAX_HISTORY) state.historyPps.shift();

      renderAll();
    } catch (err) {
      showToast(`Status fetch error: ${err.message}`, true);
    }
  }

  function renderAll() {
    const data = state.latest;
    if (!data) return;

    // Header badges & write-mode controls
    $('version-badge').textContent = `v${data.version || '2.0.0'}`;
    const modeBadge = $('mode-badge');
    if (data.demo) {
      modeBadge.textContent = 'DEMO';
      modeBadge.className = 'meta-tag tag-demo';
    } else if (data.read_only) {
      modeBadge.textContent = 'READ-ONLY';
      modeBadge.className = 'meta-tag tag-ro';
    } else {
      modeBadge.classList.add('hidden');
    }

    $('btn-demo-drift').classList.toggle('hidden', !data.demo || data.read_only);
    $('btn-add-rule').disabled = data.read_only;
    $('btn-yaml').disabled = data.read_only;
    $('btn-stop-all').disabled = data.read_only || (data.rules || []).length === 0;
    $('btn-reconcile').disabled = data.read_only;

    // Drift banner
    $('drift-banner').classList.toggle('hidden', !data.has_drift);

    // Interface datalist
    const dl = $('iface-list');
    dl.innerHTML = (data.interfaces || [])
      .map((i) => `<option value="${escapeHTML(i.name)}">${escapeHTML(i.name)} (${escapeHTML(i.state)})</option>`)
      .join('');

    renderKPIs(data);
    renderTopology(data);
    renderInspector(data);
  }

  function renderKPIs(data) {
    const rules = data.rules || [];
    let totalPkts = 0;
    let totalBytes = 0;
    let totalDropped = 0;
    let activeFilters = 0;
    let totalFilters = 0;
    let totalPps = 0;
    let totalBps = 0;

    for (const r of rules) {
      totalPkts += r.packets;
      totalBytes += r.bytes;
      totalDropped += r.dropped;
      activeFilters += r.active_filters;
      totalFilters += r.total_filters;
      const rt = state.ratesByRule.get(r.name);
      if (rt) {
        totalPps += rt.pps;
        totalBps += rt.bps;
      }
    }

    $('kpi-bps').textContent = formatRate(totalBps);
    $('kpi-total-bytes').textContent = `${formatBytes(totalBytes)} total`;
    $('kpi-pps').textContent = `${totalPps.toFixed(1)} pps`;
    $('kpi-total-pkts').textContent = `${totalPkts.toLocaleString()} pkts`;
    $('kpi-rules').textContent = String(rules.length);
    $('kpi-filters').textContent = `${activeFilters}/${totalFilters} filters`;
    $('kpi-dropped').textContent = totalDropped.toLocaleString();
    $('kpi-dropped').classList.toggle('has-drops', totalDropped > 0);
    $('kpi-updated').textContent = new Date().toLocaleTimeString();

    const healthEl = $('kpi-health-text');
    if (rules.length === 0) {
      healthEl.textContent = 'Idle';
      healthEl.className = 'health-summary';
    } else if (data.has_drift) {
      healthEl.textContent = 'Drift detected';
      healthEl.className = 'health-summary warn';
    } else {
      healthEl.textContent = 'All healthy';
      healthEl.className = 'health-summary';
    }

    drawSparkline($('spark-bps'), state.historyBps, '#0f766e');
    drawSparkline($('spark-pps'), state.historyPps, '#0284c7');
  }

  function drawSparkline(svg, values, color) {
    if (!values.length) {
      svg.innerHTML = '';
      return;
    }
    const w = 120;
    const h = 32;
    const max = Math.max(...values, 1);
    const step = values.length > 1 ? w / (values.length - 1) : w;
    const pts = values.map((v, idx) => {
      const x = (idx * step).toFixed(1);
      const y = (h - 3 - (v / max) * (h - 6)).toFixed(1);
      return `${x},${y}`;
    });
    const area = `0,${h} ${pts.join(' ')} ${w},${h}`;
    svg.innerHTML =
      `<polygon points="${area}" fill="${color}" fill-opacity="0.12"></polygon>` +
      `<polyline points="${pts.join(' ')}" fill="none" stroke="${color}" stroke-width="1.75" stroke-linejoin="round" stroke-linecap="round"></polyline>`;
  }

  function portDisplay(data, intf) {
    const p = data.ports && data.ports[intf];
    if (p && p.label) {
      return { primary: p.label, sub: intf, combined: `${p.label} (${intf})` };
    }
    return { primary: intf, sub: '', combined: intf };
  }

  function formatActions(m) {
    const parts = [];
    if (m.rate_limit && m.rate_limit.rate) parts.push(`cap:${m.rate_limit.rate}`);
    if (m.vlan) {
      if (m.vlan.push) parts.push(`vlan:+${m.vlan.push}`);
      else if (m.vlan.pop) parts.push('vlan:pop');
    }
    const rw = formatRewrite(m.rewrite);
    if (rw) parts.push(rw);
    return parts.join(' · ');
  }

  function renderTopology(data) {
    const rules = data.rules || [];
    const emptyEl = $('topology-empty');
    const stageEl = $('topology-stage');

    if (rules.length === 0) {
      emptyEl.classList.remove('hidden');
      stageEl.classList.add('hidden');
      state.selected = null;
      $('btn-clear-selection').classList.add('hidden');
      return;
    }

    emptyEl.classList.add('hidden');
    stageEl.classList.remove('hidden');
    $('btn-clear-selection').classList.toggle('hidden', !state.selected);

    const ifaceMap = new Map((data.interfaces || []).map((i) => [i.name, i]));

    // Aggregate sources & tools
    const sources = new Map();
    const tools = new Map();

    for (const r of rules) {
      const rt = state.ratesByRule.get(r.name) || { pps: 0, bps: 0 };
      if (!sources.has(r.src)) {
        sources.set(r.src, { name: r.src, ruleCount: 0, pps: 0, bps: 0, rules: new Set() });
      }
      const s = sources.get(r.src);
      s.ruleCount++;
      s.pps += rt.pps;
      s.bps += rt.bps;
      s.rules.add(r.name);

      for (const fs of r.filters || []) {
        const dst = fs.managed.dst_intf;
        const frt = state.ratesByFilter.get(`${fs.managed.src_intf}:${fs.managed.hook}:${fs.managed.pref}`) || { pps: 0, bps: 0 };
        if (!tools.has(dst)) {
          tools.set(dst, { name: dst, pps: 0, bps: 0, dropped: 0, rules: new Set() });
        }
        const t = tools.get(dst);
        t.pps += frt.pps;
        t.bps += frt.bps;
        t.dropped += fs.dropped;
        t.rules.add(r.name);
      }
    }

    const focus = state.hovered || state.selected;

    // Render Source column
    $('col-sources').innerHTML = Array.from(sources.values())
      .map((s) => {
        const info = ifaceMap.get(s.name);
        const linkState = info ? info.state : 'UP';
        const disp = portDisplay(data, s.name);
        const related = !focus || (focus.type === 'source' && focus.id === s.name) ||
          (focus.type === 'rule' && s.rules.has(focus.id)) ||
          (focus.type === 'tool' && rules.some((r) => r.src === s.name && (r.destinations || []).includes(focus.id)));
        const sel = state.selected && state.selected.type === 'source' && state.selected.id === s.name;
        return `
          <div class="topo-node ${sel ? 'selected' : ''} ${related ? '' : 'dimmed'}"
               data-node-type="source" data-node-id="${escapeHTML(s.name)}">
            <div class="node-top">
              <span class="node-name ${disp.sub ? '' : 'mono'}">${escapeHTML(disp.primary)}</span>
              <span class="status-pill status-${escapeHTML(linkState)}">${escapeHTML(linkState)}</span>
            </div>
            ${disp.sub ? `<div class="node-sublabel">${escapeHTML(disp.sub)}</div>` : ''}
            <div class="node-meta mono">
              <span>${s.ruleCount} rule${s.ruleCount > 1 ? 's' : ''}</span>
              <span>${formatRate(s.bps)}</span>
            </div>
          </div>`;
      })
      .join('');

    // Render Rules column
    $('col-rules').innerHTML = rules
      .map((r) => {
        const rt = state.ratesByRule.get(r.name) || { pps: 0, bps: 0 };
        const related = !focus ||
          (focus.type === 'rule' && focus.id === r.name) ||
          (focus.type === 'source' && r.src === focus.id) ||
          (focus.type === 'tool' && (r.destinations || []).includes(focus.id));
        const sel = state.selected && state.selected.type === 'rule' && state.selected.id === r.name;

        const matchSet = new Set();
        const vlanSet = new Set();
        const capSet = new Set();
        let hasRewrite = false;
        for (const fs of r.filters || []) {
          matchSet.add(formatMatch(fs.managed.filter || {}));
          if (fs.managed.rewrite) hasRewrite = true;
          if (fs.managed.vlan) {
            if (fs.managed.vlan.push) vlanSet.add(`vlan:+${fs.managed.vlan.push}`);
            else if (fs.managed.vlan.pop) vlanSet.add('vlan:pop');
          }
          if (fs.managed.rate_limit && fs.managed.rate_limit.rate) {
            capSet.add(`cap:${fs.managed.rate_limit.rate}`);
          }
        }
        const chips = Array.from(matchSet).slice(0, 3).map((m) => `<span class="chip">${escapeHTML(m)}</span>`);
        if (matchSet.size > 3) chips.push(`<span class="chip">+${matchSet.size - 3}</span>`);
        for (const v of vlanSet) chips.push(`<span class="chip chip-vlan">${escapeHTML(v)}</span>`);
        for (const c of capSet) chips.push(`<span class="chip chip-cap">${escapeHTML(c)}</span>`);
        if (hasRewrite) chips.push('<span class="chip chip-rw">rewrite</span>');

        return `
          <div class="topo-node ${sel ? 'selected' : ''} ${related ? '' : 'dimmed'}"
               data-node-type="rule" data-node-id="${escapeHTML(r.name)}">
            <div class="node-top">
              <span class="node-name">${escapeHTML(r.name)}</span>
              <span class="status-pill status-${escapeHTML(r.status)}">${escapeHTML(r.status)}</span>
            </div>
            <div class="node-meta mono">
              <span>${escapeHTML((r.hooks || []).join(','))}</span>
              <span>${rt.pps.toFixed(0)} pps · ${formatRate(rt.bps)}</span>
            </div>
            <div class="node-chips">${chips.join('')}</div>
          </div>`;
      })
      .join('');

    // Render Tool Ports column
    $('col-tools').innerHTML = Array.from(tools.values())
      .map((t) => {
        const info = ifaceMap.get(t.name);
        const linkState = info ? info.state : 'UP';
        const disp = portDisplay(data, t.name);
        const related = !focus ||
          (focus.type === 'tool' && focus.id === t.name) ||
          (focus.type === 'rule' && t.rules.has(focus.id)) ||
          (focus.type === 'source' && rules.some((r) => r.src === focus.id && (r.destinations || []).includes(t.name)));
        const sel = state.selected && state.selected.type === 'tool' && state.selected.id === t.name;
        return `
          <div class="topo-node ${sel ? 'selected' : ''} ${related ? '' : 'dimmed'}"
               data-node-type="tool" data-node-id="${escapeHTML(t.name)}">
            <div class="node-top">
              <span class="node-name ${disp.sub ? '' : 'mono'}">${escapeHTML(disp.primary)}</span>
              <span class="status-pill status-${escapeHTML(linkState)}">${escapeHTML(linkState)}</span>
            </div>
            ${disp.sub ? `<div class="node-sublabel">${escapeHTML(disp.sub)}</div>` : ''}
            <div class="node-meta mono">
              <span>${t.pps.toFixed(0)} pps</span>
              <span>${formatRate(t.bps)}</span>
            </div>
          </div>`;
      })
      .join('');

    bindTopologyEvents();
    requestAnimationFrame(() => drawTopologyWires(rules));
  }

  function bindTopologyEvents() {
    document.querySelectorAll('.topo-node').forEach((node) => {
      const type = node.dataset.nodeType;
      const id = node.dataset.nodeId;

      node.addEventListener('mouseenter', () => {
        state.hovered = { type, id };
        updateHighlightOnly();
      });
      node.addEventListener('mouseleave', () => {
        state.hovered = null;
        updateHighlightOnly();
      });
      node.addEventListener('click', () => {
        if (state.selected && state.selected.type === type && state.selected.id === id) {
          state.selected = null;
        } else {
          state.selected = { type, id };
        }
        renderAll();
      });
    });
  }

  function updateHighlightOnly() {
    if (!state.latest) return;
    renderTopology(state.latest);
  }

  function drawTopologyWires(rules) {
    const stage = $('topology-stage');
    const svg = $('topology-wires');
    const stageRect = stage.getBoundingClientRect();
    if (!stageRect.width || !stageRect.height) return;

    svg.setAttribute('viewBox', `0 0 ${stageRect.width} ${stageRect.height}`);

    const posOf = (type, id, side) => {
      const el = stage.querySelector(`.topo-node[data-node-type="${CSS.escape(type)}"][data-node-id="${CSS.escape(id)}"]`);
      if (!el) return null;
      const r = el.getBoundingClientRect();
      return {
        x: (side === 'right' ? r.right : r.left) - stageRect.left,
        y: r.top + r.height / 2 - stageRect.top,
      };
    };

    // Scale stroke width subtly by relative rule throughput (1.6px..4.2px)
    let maxBps = 1;
    for (const r of rules) {
      const rt = state.ratesByRule.get(r.name);
      if (rt && rt.bps > maxBps) maxBps = rt.bps;
    }
    const widthForBps = (bps, highlighted) => {
      const base = 1.6 + Math.min(bps / maxBps, 1) * 2.4;
      return (highlighted ? Math.max(base + 0.6, 2.8) : base).toFixed(2);
    };

    const focus = state.hovered || state.selected;
    const paths = [];

    const makeCurve = (p1, p2) => {
      const dx = Math.max(Math.abs(p2.x - p1.x) * 0.45, 24);
      return `M ${p1.x.toFixed(1)} ${p1.y.toFixed(1)} C ${(p1.x + dx).toFixed(1)} ${p1.y.toFixed(1)}, ${(p2.x - dx).toFixed(1)} ${p2.y.toFixed(1)}, ${p2.x.toFixed(1)} ${p2.y.toFixed(1)}`;
    };

    for (const r of rules) {
      const drifted = r.status !== 'ACTIVE';
      const ruleRate = state.ratesByRule.get(r.name) || { bps: 0 };
      const srcPt = posOf('source', r.src, 'right');
      const ruleL = posOf('rule', r.name, 'left');
      const ruleR = posOf('rule', r.name, 'right');

      const srcRelated = !focus ||
        (focus.type === 'rule' && focus.id === r.name) ||
        (focus.type === 'source' && focus.id === r.src) ||
        (focus.type === 'tool' && (r.destinations || []).includes(focus.id));

      if (srcPt && ruleL) {
        const hl = Boolean(focus && srcRelated);
        const cls = ['wire', drifted ? 'wire-drift' : 'wire-active'];
        if (focus) cls.push(srcRelated ? 'wire-highlight' : 'wire-dim');
        paths.push(`<path class="${cls.join(' ')}" style="stroke-width:${widthForBps(ruleRate.bps, hl)}px" d="${makeCurve(srcPt, ruleL)}"></path>`);
      }

      for (const dst of r.destinations || []) {
        const dstPt = posOf('tool', dst, 'left');
        if (ruleR && dstPt) {
          let dstBps = 0;
          for (const fs of r.filters || []) {
            if (fs.managed.dst_intf === dst) {
              const frt = state.ratesByFilter.get(`${fs.managed.src_intf}:${fs.managed.hook}:${fs.managed.pref}`);
              if (frt) dstBps += frt.bps;
            }
          }
          const dstRelated = !focus ||
            (focus.type === 'rule' && focus.id === r.name) ||
            (focus.type === 'source' && focus.id === r.src) ||
            (focus.type === 'tool' && focus.id === dst);
          const hl = Boolean(focus && dstRelated);
          const cls = ['wire', drifted ? 'wire-drift' : 'wire-active'];
          if (focus) cls.push(dstRelated ? 'wire-highlight' : 'wire-dim');
          paths.push(`<path class="${cls.join(' ')}" style="stroke-width:${widthForBps(dstBps, hl)}px" d="${makeCurve(ruleR, dstPt)}"></path>`);
        }
      }
    }

    svg.innerHTML = paths.join('');
  }

  function renderInspector(data) {
    const rules = data.rules || [];
    const sel = state.selected;
    const actionsEl = $('inspector-actions');
    actionsEl.innerHTML = '';

    let filteredRows = [];
    for (const r of rules) {
      for (const fs of r.filters || []) {
        const m = fs.managed;
        if (sel) {
          if (sel.type === 'rule' && r.name !== sel.id) continue;
          if (sel.type === 'source' && m.src_intf !== sel.id) continue;
          if (sel.type === 'tool' && m.dst_intf !== sel.id) continue;
        }
        filteredRows.push({ rule: r, fs });
      }
    }

    if (!sel) {
      $('inspector-title').textContent = 'All Managed Filters';
      $('inspector-subtitle').textContent = `${filteredRows.length} kernel TC filter(s) correlated by priority and handle.`;
    } else if (sel.type === 'rule') {
      $('inspector-title').textContent = `Rule: ${sel.id}`;
      $('inspector-subtitle').textContent = `${filteredRows.length} filter(s) installed for this rule.`;
      if (!data.read_only) {
        const editBtn = document.createElement('button');
        editBtn.type = 'button';
        editBtn.className = 'btn btn-secondary btn-xs';
        editBtn.textContent = 'Edit YAML';
        editBtn.addEventListener('click', () => $('btn-yaml').click());
        actionsEl.appendChild(editBtn);

        const delBtn = document.createElement('button');
        delBtn.type = 'button';
        delBtn.className = 'btn btn-danger-ghost btn-xs';
        delBtn.textContent = `Delete "${sel.id}"`;
        delBtn.addEventListener('click', () => deleteSingleRule(sel.id));
        actionsEl.appendChild(delBtn);
      }
    } else {
      const disp = portDisplay(data, sel.id);
      $('inspector-title').textContent = `${sel.type === 'source' ? 'Source' : 'Tool'} Port: ${disp.combined}`;
      $('inspector-subtitle').textContent = `${filteredRows.length} filter(s) connected to ${disp.combined}.`;
    }

    const tbody = $('filters-tbody');
    if (filteredRows.length === 0) {
      tbody.innerHTML = `<tr><td colspan="11" style="text-align:center;color:var(--text-muted);padding:24px;">No active filters to display.</td></tr>`;
      return;
    }

    tbody.innerHTML = filteredRows
      .map(({ rule, fs }) => {
        const m = fs.managed;
        const rt = state.ratesByFilter.get(`${m.src_intf}:${m.hook}:${m.pref}`) || { pps: 0, bps: 0 };
        const st = fs.present ? 'ACTIVE' : 'MISSING';
        const actions = formatActions(m) || '—';
        const srcDisp = portDisplay(data, m.src_intf).primary;
        const dstDisp = portDisplay(data, m.dst_intf).primary;
        return `
          <tr>
            <td><strong>${escapeHTML(rule.name)}</strong></td>
            <td class="mono">${m.pref}</td>
            <td><span class="status-pill status-${st}">${st}</span></td>
            <td class="mono">${escapeHTML(srcDisp)} (${escapeHTML(m.hook)}) &rarr; ${escapeHTML(dstDisp)}</td>
            <td class="mono">${escapeHTML(formatMatch(m.filter || {}))}</td>
            <td class="mono">${escapeHTML(actions)}</td>
            <td class="num">${fs.packets.toLocaleString()}</td>
            <td class="num">${formatBytes(fs.bytes)}</td>
            <td class="num">${rt.pps.toFixed(1)}</td>
            <td class="num">${formatRate(rt.bps)}</td>
            <td class="num">${fs.dropped}</td>
          </tr>`;
      })
      .join('');
  }

  // Mutating operations
  async function deleteSingleRule(name) {
    if (!confirm(`Delete rule "${name}"? External TC rules will not be affected.`)) return;
    try {
      await apiRequest(`/api/v1/rules/${encodeURIComponent(name)}`, { method: 'DELETE' });
      if (state.selected && state.selected.type === 'rule' && state.selected.id === name) {
        state.selected = null;
      }
      showToast(`Deleted rule "${name}"`);
      await refreshStatus();
    } catch (err) {
      showToast(err.message, true);
    }
  }

  async function stopAllRules() {
    if (!confirm('Stop all tcbroker-managed mirroring rules? External TC filters remain untouched.')) return;
    try {
      await apiRequest('/api/v1/rules/delete-all', { method: 'POST' });
      state.selected = null;
      showToast('All tcbroker rules stopped');
      await refreshStatus();
    } catch (err) {
      showToast(err.message, true);
    }
  }

  async function reconcileDrift() {
    try {
      const res = await apiRequest('/api/v1/reconcile', { method: 'POST' });
      showToast(`Reconciled ${res.reconciled} drifted filter(s)`);
      await refreshStatus();
    } catch (err) {
      showToast(err.message, true);
    }
  }

  function buildYAMLWithNewRule() {
    const name = $('f-name').value.trim();
    const src = $('f-src').value.trim();
    const direction = $('f-direction').value;
    const dsts = $('f-dst').value.split(',').map((s) => s.trim()).filter(Boolean);
    const proto = $('f-proto').value.trim();
    const sport = $('f-sport').value.trim();
    const dport = $('f-dport').value.trim();
    const sip = $('f-sip').value.trim();
    const dip = $('f-dip').value.trim();
    const vlanPush = $('f-vlan-push').value.trim();
    const rateLimit = $('f-rate-limit').value.trim();
    const rwDmac = $('f-rw-dmac').value.trim();
    const rwSmac = $('f-rw-smac').value.trim();
    const rwDip = $('f-rw-dip').value.trim();
    const rwSip = $('f-rw-sip').value.trim();

    if (!name || !src || dsts.length === 0) {
      throw new Error('Rule Name, Source Interface, and Destination Interface(s) are required.');
    }

    const lines = [];
    lines.push(`  - name: ${JSON.stringify(name)}`);
    lines.push(`    src: ${JSON.stringify(src)}`);
    if (direction && direction !== 'ingress') {
      lines.push(`    direction: ${direction}`);
    }
    if (dsts.length === 1) {
      lines.push(`    dst: ${JSON.stringify(dsts[0])}`);
    } else {
      lines.push('    dst:');
      for (const d of dsts) lines.push(`      - ${JSON.stringify(d)}`);
    }

    if (vlanPush) {
      lines.push('    vlan:');
      lines.push(`      push: ${Number(vlanPush)}`);
    }
    if (rateLimit) {
      lines.push('    rate_limit:');
      lines.push(`      rate: ${JSON.stringify(rateLimit)}`);
    }
    if (rwDmac || rwSmac || rwDip || rwSip) {
      lines.push('    rewrite:');
      if (rwDmac) lines.push(`      dst_mac: ${JSON.stringify(rwDmac)}`);
      if (rwSmac) lines.push(`      src_mac: ${JSON.stringify(rwSmac)}`);
      if (rwDip) lines.push(`      dst_ip: ${JSON.stringify(rwDip)}`);
      if (rwSip) lines.push(`      src_ip: ${JSON.stringify(rwSip)}`);
    }

    if (proto || sport || dport || sip || dip) {
      lines.push('    filters:');
      lines.push('      -');
      if (proto) lines.push(`        proto: ${proto}`);
      if (sip) lines.push(`        src_ip: ${JSON.stringify(sip)}`);
      if (dip) lines.push(`        dst_ip: ${JSON.stringify(dip)}`);
      if (sport) lines.push(`        src_port: ${Number(sport)}`);
      if (dport) lines.push(`        dst_port: ${Number(dport)}`);
    }

    const existing = (state.latest && state.latest.config_yaml || '').trim();
    if (!existing || existing === 'rules: []') {
      return `rules:\n${lines.join('\n')}\n`;
    }
    if (existing.endsWith('rules: []')) {
      return `${existing.slice(0, -'rules: []'.length)}rules:\n${lines.join('\n')}\n`;
    }
    return `${existing}\n${lines.join('\n')}\n`;
  }

  function formatPlanFeedback(res) {
    const prefix = res.dry_run ? '[dry-run] ' : '';
    const added = (res.to_add || []).length;
    const deleted = (res.to_delete || []).length;
    const lines = [`${prefix}Plan: +${added} add, -${deleted} delete, =${res.kept_count} unchanged`];
    for (const f of res.to_add || []) {
      lines.push(`  + [${f.rule_name}] ${f.src_intf} (${f.hook}) -> ${f.dst_intf} pref=${f.pref} (${formatMatch(f.filter || {})})`);
    }
    for (const f of res.to_delete || []) {
      lines.push(`  - [${f.rule_name}] ${f.src_intf} (${f.hook}) -> ${f.dst_intf} pref=${f.pref}`);
    }
    for (const w of res.warnings || []) {
      lines.push(`  ! Warning: ${w}`);
    }
    return lines.join('\n');
  }

  async function submitApplyYAML(yamlText, dryRun, feedbackEl, onSuccessCloseModal) {
    feedbackEl.classList.remove('hidden', 'ok', 'err');
    feedbackEl.textContent = dryRun ? 'Running dry-run...' : 'Applying...';
    try {
      const res = await apiRequest('/api/v1/apply', {
        method: 'POST',
        body: JSON.stringify({ yaml: yamlText, dry_run: dryRun }),
      });
      feedbackEl.classList.add('ok');
      feedbackEl.textContent = formatPlanFeedback(res);
      if (!dryRun) {
        showToast(`Applied (+${(res.to_add || []).length} / -${(res.to_delete || []).length})`);
        await refreshStatus();
        if (onSuccessCloseModal) onSuccessCloseModal();
      }
    } catch (err) {
      feedbackEl.classList.add('err');
      feedbackEl.textContent = `Error: ${err.message}`;
    }
  }

  // Event bindings
  function initEvents() {
    $('btn-pause').addEventListener('click', () => {
      state.paused = !state.paused;
      $('btn-pause').textContent = state.paused ? 'Paused' : 'Live 1s';
    });

    $('btn-clear-selection').addEventListener('click', () => {
      state.selected = null;
      renderAll();
    });

    $('btn-reconcile').addEventListener('click', reconcileDrift);
    $('btn-stop-all').addEventListener('click', stopAllRules);

    $('btn-demo-drift').addEventListener('click', async () => {
      try {
        await apiRequest('/api/v1/demo/drift', { method: 'POST' });
        showToast('Simulated kernel drift (removed 1 filter)');
        await refreshStatus();
      } catch (err) {
        showToast(err.message, true);
      }
    });

    const openAddRule = () => {
      $('form-add-rule').reset();
      $('add-rule-feedback').classList.add('hidden');
      $('modal-add-rule').showModal();
    };
    $('btn-add-rule').addEventListener('click', openAddRule);
    $('btn-empty-add').addEventListener('click', openAddRule);

    const openYaml = () => {
      const current = (state.latest && state.latest.config_yaml) || 'rules:\n  - name: mirror-all\n    src: eth0\n    dst: eth1\n';
      $('yaml-editor').value = current;
      $('yaml-feedback').classList.add('hidden');
      $('modal-yaml').showModal();
    };
    $('btn-yaml').addEventListener('click', openYaml);
    $('btn-empty-yaml').addEventListener('click', openYaml);

    $('btn-add-dryrun').addEventListener('click', () => {
      try {
        const y = buildYAMLWithNewRule();
        submitApplyYAML(y, true, $('add-rule-feedback'));
      } catch (err) {
        const fb = $('add-rule-feedback');
        fb.className = 'feedback err';
        fb.textContent = `Error: ${err.message}`;
      }
    });

    $('form-add-rule').addEventListener('submit', (e) => {
      e.preventDefault();
      try {
        const y = buildYAMLWithNewRule();
        submitApplyYAML(y, false, $('add-rule-feedback'), () => $('modal-add-rule').close());
      } catch (err) {
        const fb = $('add-rule-feedback');
        fb.className = 'feedback err';
        fb.textContent = `Error: ${err.message}`;
      }
    });

    $('btn-yaml-dryrun').addEventListener('click', () => {
      submitApplyYAML($('yaml-editor').value, true, $('yaml-feedback'));
    });

    $('btn-yaml-apply').addEventListener('click', () => {
      submitApplyYAML($('yaml-editor').value, false, $('yaml-feedback'), () => $('modal-yaml').close());
    });

    $('form-token').addEventListener('submit', async (e) => {
      e.preventDefault();
      state.token = $('f-token').value.trim();
      sessionStorage.setItem('tcbroker_token', state.token);
      $('modal-token').close();
      if (state.pendingAction) {
        const fn = state.pendingAction;
        state.pendingAction = null;
        try {
          await fn();
          await refreshStatus();
        } catch (err) {
          showToast(err.message, true);
        }
      }
    });

    document.querySelectorAll('[data-close-modal]').forEach((btn) => {
      btn.addEventListener('click', () => {
        const dlg = btn.closest('dialog');
        if (dlg) dlg.close();
      });
    });

    window.addEventListener('resize', () => {
      if (state.latest) drawTopologyWires(state.latest.rules || []);
    });
  }

  initEvents();
  refreshStatus();
  setInterval(() => {
    if (!state.paused) refreshStatus();
  }, POLL_MS);
})();
