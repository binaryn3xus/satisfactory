import { Parser } from '@etothepii/satisfactory-file-parser';

// Helper: Format seconds to HH:MM:SS
function formatDuration(sec) {
    if (!sec || sec <= 0) return '0m';
    const h = Math.floor(sec / 3600);
    const m = Math.floor((sec % 3600) / 60);
    const s = sec % 60;
    if (h > 0) return `${h}h ${m.toString().padStart(2, '0')}m`;
    return `${m}m ${s.toString().padStart(2, '0')}s`;
}

// Helper: Format byte sizes
function formatBytes(bytes) {
    if (bytes === 0) return '0 B';
    const k = 1024;
    const sizes = ['B', 'KB', 'MB', 'GB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + ' ' + sizes[i];
}

// Helper: Convert raw uint8 array to SteamID64 / hex string
function rawDataToSteamId(raw) {
    if (!raw || raw.length < 14) return null;
    const bytes = raw.slice(raw.length - 8);
    let hex = '';
    for (let i = bytes.length - 1; i >= 0; i--) {
        hex += bytes[i].toString(16).padStart(2, '0');
    }
    try {
        const big = BigInt('0x' + hex);
        return big.toString();
    } catch {
        return '0x' + hex;
    }
}

// =============================================================================
// Extensible Diagnostics Pipeline
// =============================================================================
export const DIAGNOSTIC_RULES = [
    {
        id: 'respawn_inventory_bounds',
        name: 'Player Respawn Inventory Check',
        run: (ctx) => {
            const bad = ctx.rawPlayerStates.filter(p => p.hasCorruptRespawn);
            if (bad.length > 0) {
                return {
                    status: 'error',
                    title: 'Corrupted Respawn Inventory Table Detected',
                    details: `${bad.length} player(s) (${bad.map(p => p.name).join(', ')}) have a corrupt mInventoryToRespawnWith table (63 slots). This triggers a fatal engine crash on connect (AFGPlayerController::FinishRespawn array bounds assertion).`,
                    fixable: true,
                    affected: bad.map(p => p.name)
                };
            }
            return {
                status: 'pass',
                title: 'Player Respawn Tables Healthy',
                details: 'All player states have clean respawn parameters. No connect-crash conditions detected.'
            };
        }
    },
    {
        id: 'dormant_clones_check',
        name: 'Offline Pioneer Clones in World',
        run: (ctx) => {
            const clones = ctx.rawPlayerStates.filter(p => !p.isPrimary);
            if (clones.length > 0) {
                const breakdown = ctx.pioneers.filter(p => p.cloneCount > 0).map(p => `${p.name} (${p.cloneCount} clone${p.cloneCount > 1 ? 's' : ''})`).join(', ');
                return {
                    status: 'info',
                    title: `${clones.length} Dormant Pioneer Clone(s) Found`,
                    details: `World contains ${clones.length} offline pioneer body/bodies from earlier sessions: ${breakdown}. These are harmless sitting clones created when players reconnected after session or network changes.`,
                    fixable: true
                };
            }
            return {
                status: 'pass',
                title: 'Clean Player Roster',
                details: 'Every player state corresponds directly to an active pioneer with no orphaned ghost clones.'
            };
        }
    },
    {
        id: 'modded_level_check',
        name: 'Custom Map Level & Mod Partitions',
        run: (ctx) => {
            const modLevels = ctx.levels.filter(l => !l.name.includes('PersistentLevel') && !l.name.includes('HUBLevelInstance'));
            if (modLevels.length > 0) {
                return {
                    status: 'info',
                    title: 'Custom Map / Skyblock Level Detected',
                    details: `Save loads custom level: ${modLevels.map(l => l.name).join(', ')}. Standard external web visualizers (like SCIM) may reject this save due to non-vanilla level schemas.`
                };
            }
            return {
                status: 'pass',
                title: 'Vanilla Map Partition',
                details: 'Save uses standard FactoryGame map level partitions.'
            };
        }
    },
    {
        id: 'save_version_compat',
        name: 'Save Engine Version Compatibility',
        run: (ctx) => {
            if (ctx.header.saveVersion >= 40) {
                return {
                    status: 'pass',
                    title: 'Unreal Engine 5 / v1.0+ Format',
                    details: `Save format v${ctx.header.saveVersion} (Build ${ctx.header.buildVersion}) is current and supported.`
                };
            }
            return {
                status: 'warn',
                title: 'Legacy Save Format',
                details: `Save version ${ctx.header.saveVersion} is from an earlier engine build.`
            };
        }
    }
];

// =============================================================================
// Core Inspector Parser (Extracts AST & Metadata)
// =============================================================================
export function inspectSaveAST(save, originalFileSize = 0, parseTimeMs = 0) {
    const header = {
        saveHeaderVersion: save.header.saveHeaderVersion,
        saveVersion: save.header.saveVersion,
        buildVersion: save.header.buildVersion,
        mapName: save.header.mapName,
        mapOptions: save.header.mapOptions,
        sessionName: save.header.sessionName,
        playDurationSeconds: save.header.playDurationSeconds,
        formattedPlaytime: formatDuration(save.header.playDurationSeconds),
        saveName: save.header.saveName,
        fileSize: originalFileSize,
        formattedSize: formatBytes(originalFileSize),
        parseTimeMs
    };

    // Extract mod session settings from mapOptions if present
    const modSettings = {};
    if (header.mapOptions) {
        const parts = header.mapOptions.split('?');
        for (const part of parts) {
            if (part.startsWith('SessionSettings=')) {
                const sub = part.slice('SessionSettings='.length).split(',');
                for (const item of sub) {
                    const [k, v] = item.split('=');
                    if (k) modSettings[k] = v;
                }
            }
        }
    }

    // Find PlayerInfoCache, Space Elevator, HUB, and Game Phase
    let playerCache = [];
    let isSpaceElevatorBuilt = false;
    let isTradingPostBuilt = false;
    let hubTransform = null;
    let targetGamePhase = '';
    let totalObjects = 0;
    const levels = [];

    for (const levelName in save.levels) {
        const lvl = save.levels[levelName];
        totalObjects += lvl.objects.length;
        levels.push({
            name: levelName,
            objectsCount: lvl.objects.length
        });

        for (const obj of lvl.objects) {
            const tp = obj.typePath || '';
            if (tp.includes('Build_SpaceElevator')) {
                isSpaceElevatorBuilt = true;
            }
            if (tp.includes('Build_TradingPost')) {
                isTradingPostBuilt = true;
                if (obj.transform?.translation) {
                    hubTransform = obj.transform.translation;
                }
            }
            if (tp.includes('GamePhaseManager')) {
                const target = obj.properties?.mTargetGamePhase?.value?.pathName || '';
                const curr = obj.properties?.mCurrentGamePhase?.value?.pathName || '';
                const tMatch = target.match(/Phase_(\d+)/);
                const cMatch = curr.match(/Phase_(\d+)/);
                if (tMatch) {
                    targetGamePhase = `Phase ${tMatch[1]}`;
                } else if (cMatch) {
                    targetGamePhase = `Phase ${cMatch[1]}`;
                }
            }

            if (obj.instanceName && obj.instanceName.includes('BP_GameState')) {
                const props = obj.properties || {};
                if (props.mPlayerInfoCache?.values) {
                    playerCache = props.mPlayerInfoCache.values;
                }
            }
        }
    }

    // Extract Raw Pioneers / PlayerStates
    const rawPlayerStates = [];
    for (const levelName in save.levels) {
        const lvl = save.levels[levelName];
        for (const obj of lvl.objects) {
            if (obj.typePath && obj.typePath.includes('BP_PlayerState')) {
                const props = obj.properties || {};
                const infoHandle = props.mPlatformPlayerInfoHandle?.value;
                const idx = infoHandle?.playerInfoTableIndex;

                let name = 'Unknown Pioneer';
                let platform = 'STEAM';
                let steamId = null;

                if (idx !== undefined && playerCache[idx]) {
                    const cached = playerCache[idx].properties || {};
                    platform = cached.LastSeenPlatformName?.value || 'STEAM';
                    const perPlatform = cached.PerPlatformInfo?.values?.[0]?.properties || {};
                    if (perPlatform.PlayerName?.value) {
                        name = perPlatform.PlayerName.value;
                    }
                    const rawId = perPlatform.UniqueNetId?.value?.rawData;
                    if (rawId) {
                        steamId = rawDataToSteamId(rawId);
                    }
                }

                // Check for corrupt respawn table
                let hasCorruptRespawn = false;
                let respawnSlots = 0;
                if (props.mInventoryToRespawnWith) {
                    hasCorruptRespawn = true;
                    try {
                        const stacks = props.mInventoryToRespawnWith.value.properties.mInventoryStacks.values;
                        respawnSlots = stacks.length;
                    } catch {
                        respawnSlots = -1;
                    }
                }

                // Coordinates & Active Pawn
                const hasPawn = Boolean(props.mOwnedPawn?.value?.pathName);
                const hasClientIdentity = Boolean(props.mClientIdentityInfo);
                const loc = props.mLastSafeCharacterLocation?.value;
                let locationStr = 'At Hub / Respawning';
                if (loc && typeof loc.x === 'number') {
                    locationStr = `X: ${Math.round(loc.x / 100)}m, Y: ${Math.round(loc.y / 100)}m, Z: ${Math.round(loc.z / 100)}m`;
                }

                const rawSlots = props.mNumObservedInventorySlots?.value;
                const inventorySlots = typeof rawSlots === 'number' ? rawSlots : 0;

                rawPlayerStates.push({
                    name,
                    platform,
                    steamId,
                    instanceName: obj.instanceName,
                    levelName,
                    hasPawn,
                    hasClientIdentity,
                    hasCorruptRespawn,
                    respawnSlots,
                    locationStr,
                    inventorySlots
                });
            }
        }
    }

    // Group player states by Pioneer name to detect active character vs dormant clones
    const pioneerGroups = new Map();
    for (const p of rawPlayerStates) {
        if (!pioneerGroups.has(p.name)) {
            pioneerGroups.set(p.name, []);
        }
        pioneerGroups.get(p.name).push(p);
    }

    const pioneers = [];
    let totalClones = 0;

    for (const [name, states] of pioneerGroups.entries()) {
        // Sort: hasClientIdentity first, hasPawn first, highest inventory slots first
        states.sort((a, b) => {
            if (a.hasClientIdentity !== b.hasClientIdentity) return b.hasClientIdentity ? 1 : -1;
            if (a.hasPawn !== b.hasPawn) return b.hasPawn ? 1 : -1;
            return b.inventorySlots - a.inventorySlots;
        });

        const primary = states[0];
        primary.isPrimary = true;
        primary.statusLabel = primary.hasPawn ? 'Active In-World' : 'Offline';

        const clones = states.slice(1);
        clones.forEach(c => {
            c.isPrimary = false;
            c.statusLabel = 'Dormant Clone';
        });
        totalClones += clones.length;

        pioneers.push({
            name,
            platform: primary.platform,
            steamId: primary.steamId,
            primaryState: primary,
            clones,
            cloneCount: clones.length
        });
    }

    // Run Diagnostics Pipeline
    const context = { header, levels, pioneers, rawPlayerStates, modSettings, totalObjects };
    const diagnostics = DIAGNOSTIC_RULES.map(rule => ({
        id: rule.id,
        name: rule.name,
        ...rule.run(context)
    }));

    return {
        rawSave: save,
        header,
        levels,
        totalObjects,
        pioneers,
        rawPlayerStates,
        totalClones,
        diagnostics,
        worldInfo: {
            isSpaceElevatorBuilt,
            isTradingPostBuilt,
            hubTransform,
            targetGamePhase,
            modSettings
        }
    };
}

export function inspectSaveBuffer(arrayBuffer, filename = 'save.sav') {
    const t0 = performance.now();
    const save = Parser.ParseSave(filename, arrayBuffer);
    const parseTimeMs = Math.round(performance.now() - t0);
    return inspectSaveAST(save, arrayBuffer.byteLength, parseTimeMs);
}

// =============================================================================
// UI Modal & Event Controller
// =============================================================================
let activeTab = 'summary';
let showDormantClones = false;
let currentInspectionData = null;
let editNotice = null;
let selectedEditPioneerName = '';
let searchResults = [];
let selectedEditObject = null;
let editCounter = 0;
let isAdminMode = false;
let targetContainerId = null;
let currentSaveName = '';

function ensureRenderRoot() {
    if (targetContainerId) {
        const el = document.getElementById(targetContainerId);
        if (el) return el;
    }
    return ensureModalRoot();
}

function recomputeAndRender(noticeMsg = '') {
    if (!currentInspectionData || !currentInspectionData.rawSave) return;
    if (noticeMsg) {
        editNotice = { type: 'success', msg: noticeMsg };
        editCounter++;
    }
    const oldSize = currentInspectionData.header.fileSize;
    currentInspectionData = inspectSaveAST(currentInspectionData.rawSave, oldSize, 0);
    const root = ensureRenderRoot();
    root.innerHTML = renderModalContent(currentInspectionData);
}

function renderModalContent(data) {
    currentInspectionData = data;
    const { header, pioneers, totalClones, diagnostics } = data;

    // Badges for diagnostics
    const errors = diagnostics.filter(d => d.status === 'error');
    const warns = diagnostics.filter(d => d.status === 'warn');
    let healthBadge = `<span class="health-badge ok">✅ Healthy</span>`;
    if (errors.length > 0) {
        healthBadge = `<span class="health-badge error">❌ ${errors.length} Issue(s)</span>`;
    } else if (warns.length > 0) {
        healthBadge = `<span class="health-badge warn">⚠️ Notice</span>`;
    }

    const editBadge = editCounter > 0 
        ? `<span style="background:var(--accent-primary); color:#fff; font-size:0.75rem; font-weight:700; padding:2px 7px; border-radius:10px; margin-left:0.35rem;">${editCounter}</span>` 
        : '';

    return `
    <div class="inspector-card">
        <div class="inspector-header">
            <div>
                <h2 style="margin:0; font-size:1.35rem; display:flex; align-items:center; gap:0.5rem;">
                    🔍 Save Inspector: <span style="color:var(--accent-primary);">${header.saveName || header.sessionName}</span>
                </h2>
                <div style="font-size:0.85rem; color:var(--text-muted); margin-top:0.25rem;">
                    Session: <strong>${header.sessionName}</strong> &bull; Size: ${header.formattedSize}
                    ${header.parseTimeMs ? ` &bull; Parsed in ${header.parseTimeMs}ms` : ''}
                </div>
            </div>
            <div style="display:flex; align-items:center; gap:0.75rem;">
                ${healthBadge}
                ${editCounter > 0 ? `<button class="btn btn-primary" onclick="FicsitInspector.downloadEditedSave()" style="padding:0.4rem 0.8rem; font-size:0.8rem;">💾 Download Edited .sav</button>` : ''}
                ${!targetContainerId ? `<button class="modal-close-btn" onclick="FicsitInspector.close()">&times;</button>` : ''}
            </div>
        </div>

        <div class="inspector-tabs">
            <button class="inspector-tab-btn ${activeTab === 'summary' ? 'active' : ''}" onclick="FicsitInspector.switchTab('summary')">📊 Overview</button>
            <button class="inspector-tab-btn ${activeTab === 'pioneers' ? 'active' : ''}" onclick="FicsitInspector.switchTab('pioneers')">👷 Pioneers (${pioneers.length}${totalClones > 0 ? ' + ' + totalClones + ' clones' : ''})</button>
            <button class="inspector-tab-btn ${activeTab === 'diagnostics' ? 'active' : ''}" onclick="FicsitInspector.switchTab('diagnostics')">🩺 Health &amp; Diagnostics (${diagnostics.length})</button>
            <button class="inspector-tab-btn ${activeTab === 'world' ? 'active' : ''}" onclick="FicsitInspector.switchTab('world')">🗺️ World &amp; Mods</button>
            ${isAdminMode ? `<button class="inspector-tab-btn ${activeTab === 'editor' ? 'active' : ''}" onclick="FicsitInspector.switchTab('editor')">✏️ Save Editor${editBadge}</button>` : ''}
        </div>

        <div class="inspector-body">
            ${renderNotification()}
            ${activeTab === 'summary' ? renderSummaryTab(data) : ''}
            ${activeTab === 'pioneers' ? renderPioneersTab(data) : ''}
            ${activeTab === 'diagnostics' ? renderDiagnosticsTab(data) : ''}
            ${activeTab === 'world' ? renderWorldTab(data) : ''}
            ${isAdminMode && activeTab === 'editor' ? renderEditorTab(data) : ''}
        </div>
    </div>`;
}

function renderNotification() {
    if (!editNotice) return '';
    return `
    <div style="margin-bottom:1rem; padding:0.75rem 1rem; border-radius:6px; background:rgba(34,197,94,0.15); border:1px solid rgba(34,197,94,0.4); color:#4ade80; display:flex; justify-content:space-between; align-items:center;">
        <div><strong>Success:</strong> ${editNotice.msg}</div>
        <button style="background:transparent; border:none; color:#4ade80; cursor:pointer; font-size:1.1rem;" onclick="FicsitInspector.dismissNotice()">&times;</button>
    </div>`;
}

function renderSummaryTab({ header, totalObjects, worldInfo, pioneers, totalClones }) {
    return `
    <div class="inspector-grid">
        <div class="inspector-box">
            <div class="box-label">SESSION NAME</div>
            <div class="box-value">${header.sessionName}</div>
        </div>
        <div class="inspector-box">
            <div class="box-label">PLAYTIME</div>
            <div class="box-value">${header.formattedPlaytime}</div>
        </div>
        <div class="inspector-box">
            <div class="box-label">BUILD VERSION</div>
            <div class="box-value">CL# ${header.buildVersion}</div>
        </div>
        <div class="inspector-box">
            <div class="box-label">TOTAL OBJECTS</div>
            <div class="box-value">${totalObjects.toLocaleString()}</div>
        </div>
        <div class="inspector-box">
            <div class="box-label">MAP LEVEL</div>
            <div class="box-value">${header.mapName}</div>
        </div>
        <div class="inspector-box">
            <div class="box-label">PIONEERS ROSTER</div>
            <div class="box-value">${pioneers.length} Pioneer(s)${totalClones > 0 ? ` <span style="font-size:0.8rem; color:var(--text-muted); font-weight:normal;">(${totalClones} clones)</span>` : ''}</div>
        </div>
        <div class="inspector-box">
            <div class="box-label">SPACE ELEVATOR</div>
            <div class="box-value">${worldInfo.isSpaceElevatorBuilt ? `🏗️ Operational${worldInfo.targetGamePhase ? ` (${worldInfo.targetGamePhase})` : ''}` : '❌ Not Built Yet'}</div>
        </div>
        <div class="inspector-box">
            <div class="box-label">HUB / TRADING POST</div>
            <div class="box-value">${worldInfo.isTradingPostBuilt ? '🏠 Landed &amp; Operational' : '❌ Not Placed'}</div>
        </div>
    </div>`;
}

function renderPioneersTab({ pioneers, totalClones }) {
    if (pioneers.length === 0) {
        return `<p style="color:var(--text-muted); text-align:center; padding:1.5rem;">No pioneers recorded in this save file.</p>`;
    }

    let rows = '';
    for (const p of pioneers) {
        const s = p.primaryState;
        const statusIcon = s.hasPawn ? '🟢' : '⚪';
        const cloneBadge = p.cloneCount > 0 
            ? `<span class="clone-pill" title="${p.cloneCount} dormant body/bodies in world">👻 ${p.cloneCount} Clone${p.cloneCount > 1 ? 's' : ''}</span>` 
            : '';

        rows += `
        <tr>
            <td style="font-weight:600;">
                <span style="margin-right:0.35rem;" title="${s.hasPawn ? 'Active In-World' : 'Offline'}">${statusIcon}</span>
                ${p.name}
                ${cloneBadge}
                ${s.hasCorruptRespawn ? '<span style="margin-left:0.5rem; color:#ef4444; font-size:0.75rem; border:1px solid #ef4444; padding:2px 6px; border-radius:4px;">⚠️ Respawn Bug</span>' : ''}
            </td>
            <td><span class="platform-pill">${p.platform}</span></td>
            <td style="font-family:monospace; font-size:0.85rem; color:var(--text-muted);">${p.steamId || '—'}</td>
            <td style="font-size:0.85rem;">${s.locationStr}</td>
            <td style="font-size:0.85rem; font-weight:600;">${s.inventorySlots} slots</td>
        </tr>`;

        // If user wants to inspect dormant clones, render indented clone rows
        if (showDormantClones && p.clones.length > 0) {
            for (const c of p.clones) {
                rows += `
                <tr style="background: rgba(0,0,0,0.2); font-size:0.83rem; color:var(--text-muted);">
                    <td style="padding-left:2.2rem;">
                        ↳ <span style="color:#c084fc;">👻 Dormant Clone Body</span>
                        ${c.hasCorruptRespawn ? '<span style="margin-left:0.5rem; color:#ef4444; font-size:0.72rem; border:1px solid #ef4444; padding:1px 4px; border-radius:3px;">⚠️ Corrupt</span>' : ''}
                    </td>
                    <td><span style="font-size:0.75rem; color:var(--text-muted);">Clone</span></td>
                    <td style="font-family:monospace; font-size:0.75rem; opacity:0.8;">${c.instanceName.split('.').pop()}</td>
                    <td>${c.locationStr}</td>
                    <td>${c.inventorySlots} slots</td>
                </tr>`;
            }
        }
    }

    const cloneToggle = totalClones > 0 ? `
        <label style="cursor:pointer; display:inline-flex; align-items:center; gap:0.45rem; font-size:0.85rem; color:var(--text-muted); user-select:none;">
            <input type="checkbox" ${showDormantClones ? 'checked' : ''} onchange="FicsitInspector.toggleDormantClones()" style="cursor:pointer;">
            Show ${totalClones} dormant clone bodies in world
        </label>` : '';

    return `
    <div style="display:flex; justify-content:space-between; align-items:center; margin-bottom:0.75rem; flex-wrap:wrap; gap:0.5rem;">
        <div style="font-size:0.9rem; color:var(--text-main);">
            <strong>${pioneers.length} Unique Pioneer${pioneers.length > 1 ? 's' : ''}</strong>
        </div>
        ${cloneToggle}
    </div>
    <table class="inspector-table">
        <thead>
            <tr>
                <th>Pioneer Name</th>
                <th>Platform</th>
                <th>Platform ID</th>
                <th>Current Position</th>
                <th>Inventory</th>
            </tr>
        </thead>
        <tbody>${rows}</tbody>
    </table>`;
}

function renderDiagnosticsTab({ diagnostics }) {
    const list = diagnostics.map(d => {
        let icon = '✅';
        let cls = 'diag-pass';
        if (d.status === 'error') { icon = '❌'; cls = 'diag-error'; }
        if (d.status === 'warn') { icon = '⚠️'; cls = 'diag-warn'; }
        if (d.status === 'info') { icon = 'ℹ️'; cls = 'diag-info'; }

        return `
        <div class="diag-item ${cls}">
            <div style="display:flex; justify-content:space-between; align-items:center; margin-bottom:0.35rem;">
                <strong>${icon} ${d.title}</strong>
                <span class="diag-tag">${d.status.toUpperCase()}</span>
            </div>
            <div style="font-size:0.9rem; line-height:1.4; color:var(--text-muted);">${d.details}</div>
        </div>`;
    }).join('');

    return `
    <div style="display:flex; flex-direction:column; gap:0.75rem;">
        ${list}
        <div style="margin-top:0.75rem; padding:0.75rem; background:rgba(255,255,255,0.03); border-radius:6px; font-size:0.8rem; color:var(--text-muted);">
            💡 <em>Rule pipeline is extensible. Use the ✏️ Save Editor tab to automatically fix flagged issues or tune properties.</em>
        </div>
    </div>`;
}

function decodeSMLSetting(raw) {
    if (!raw || typeof raw !== 'string') return raw;
    try {
        const binStr = atob(raw);
        const bytes = new Uint8Array(binStr.length);
        for (let i = 0; i < binStr.length; i++) {
            bytes[i] = binStr.charCodeAt(i);
        }
        if (bytes.length < 9) return raw;
        const view = new DataView(bytes.buffer);
        const typeTag = view.getUint32(0, true);

        if (typeTag === 2) { // Boolean
            return view.getUint8(8) ? 'true' : 'false';
        }
        if (typeTag === 10 && bytes.length >= 12) { // Float
            const val = view.getFloat32(8, true);
            return Number.isInteger(val) ? val.toString() : val.toFixed(2).replace(/\.?0+$/, '');
        }
        if (typeTag === 3 && bytes.length >= 12) { // Int32
            return view.getInt32(8, true).toString();
        }
    } catch {
        // fallback to raw string
    }
    return raw;
}

function renderWorldTab({ levels, worldInfo }) {
    const lvlRows = levels.map(l => `
        <tr>
            <td style="font-family:monospace; font-size:0.85rem;">${l.name}</td>
            <td style="text-align:right; font-weight:600;">${l.objectsCount.toLocaleString()}</td>
        </tr>
    `).join('');

    const modKeys = Object.keys(worldInfo.modSettings || {});
    let modHtml = `<p style="color:var(--text-muted); font-size:0.9rem;">No custom mod session variables detected.</p>`;
    if (modKeys.length > 0) {
        modHtml = `<div class="mod-settings-grid">${modKeys.map(k => {
            const rawVal = worldInfo.modSettings[k];
            const decoded = decodeSMLSetting(rawVal);
            const isDecoded = decoded !== rawVal;
            return `
            <div class="mod-setting-pill" title="Raw: ${rawVal}" style="background:#131a24; border:1px solid var(--border); padding:0.6rem 0.75rem; border-radius:6px; overflow-wrap:anywhere; word-break:break-word;">
                <div style="color:var(--accent-primary); font-family:monospace; font-weight:600; font-size:0.8rem; line-height:1.3; margin-bottom:0.35rem;">${k}</div>
                <div style="display:flex; justify-content:space-between; align-items:baseline; flex-wrap:wrap; gap:0.4rem;">
                    <span style="color:#a7f3d0; font-family:monospace; font-weight:700; font-size:0.92rem;">${decoded}</span>
                    ${isDecoded ? `<span style="font-size:0.7rem; color:var(--text-muted); font-family:monospace; word-break:break-all;">(raw: ${rawVal})</span>` : ''}
                </div>
            </div>`;
        }).join('')}</div>`;
    }

    return `
    <h3 style="margin:0 0 0.5rem 0; font-size:1rem;">World Partitions &amp; Sublevels</h3>
    <table class="inspector-table" style="margin-bottom:1.5rem;">
        <thead>
            <tr>
                <th>Level Partition</th>
                <th style="text-align:right;">Actor Count</th>
            </tr>
        </thead>
        <tbody>${lvlRows}</tbody>
    </table>

    <h3 style="margin:0 0 0.5rem 0; font-size:1rem;">Mod Session Parameters</h3>
    ${modHtml}`;
}

// =============================================================================
// Editor Tab Rendering & Modification Handlers
// =============================================================================
const CATEGORY_DEFINITIONS = [
    { id: 'subsystems', label: '⚙️ Subsystems & Managers (Game Phase, Research/MAM, Power Grids, Trains)', match: (tp, inst) => tp.includes('Subsystem') || tp.includes('Manager') || tp.includes('GameState') },
    { id: 'production', label: '🏭 Production & Machines (Smelters, Constructors, Assemblers, Manufacturers, Elevators)', match: (tp, inst) => tp.includes('Constructor') || tp.includes('Smelter') || tp.includes('Assembler') || tp.includes('Manufacturer') || tp.includes('Refinery') || tp.includes('Foundry') || tp.includes('SpaceElevator') || tp.includes('TradingPost') },
    { id: 'power', label: '⚡ Power & Energy (Coal Generators, Fuel, Power Poles, Circuits)', match: (tp, inst) => tp.includes('Generator') || tp.includes('Power') || tp.includes('Circuit') },
    { id: 'storage', label: '📦 Storage & Crates (Containers, Death Crates, Dimensional Depot)', match: (tp, inst) => tp.includes('Storage') || tp.includes('Crate') },
    { id: 'pioneers', label: '👷 Pioneers & Characters (Player States & Pawns)', match: (tp, inst) => tp.includes('PlayerState') || tp.includes('Char_Player') },
    { id: 'logistics', label: '🚂 Logistics & Vehicles (Trains, Trucks, Belts, Pipes)', match: (tp, inst) => tp.includes('Train') || tp.includes('Locomotive') || tp.includes('Vehicle') || tp.includes('Drone') || tp.includes('Conveyor') || tp.includes('Pipeline') },
    { id: 'modded', label: '🧩 Modded Actors (PEP / Skyblock)', match: (tp, inst) => tp.includes('PEP') || tp.includes('SKBK') || inst.includes('SKBK') },
    { id: 'all', label: '🌐 All World Objects (Raw Search)', match: () => true }
];

let selectedObjectCategory = 'subsystems';
let objectSearchQuery = '';

function formatFriendlyName(obj) {
    const inst = obj.instanceName ? obj.instanceName.split('.').pop() : 'Object';
    const tp = obj.typePath ? obj.typePath.split('/').pop().replace(/_C$/, '') : '';

    const subsystemNames = {
        'GamePhaseManager': 'Game Phase Manager (Project Assembly)',
        'CircuitSubsystem': 'Circuit Subsystem (Power Grids)',
        'ResearchManager': 'Research Manager (MAM Tech Tree)',
        'RailroadSubsystem': 'Railroad Subsystem (Trains & Signals)',
        'StorySubsystem': 'Story & Narrative Subsystem',
        'BP_GameState': 'Global Game State',
        'MapManager': 'Map & Beacon Manager',
        'TutorialIntroManager': 'Tutorial & Onboarding Manager',
        'CentralStorageSubsystem': 'Dimensional Depot / Central Storage',
        'TimeSubsystem': 'Time of Day Subsystem',
        'ResourceSinkSubsystem': 'AWESOME Sink & Shop Subsystem',
        'Build_SpaceElevator': 'Space Elevator',
        'Build_TradingPost': 'HUB / Trading Post'
    };
    for (const k in subsystemNames) {
        if (inst.includes(k) || tp.includes(k)) return subsystemNames[k];
    }

    let clean = tp.replace(/^Build_/, '').replace(/^BP_/, '').replace(/Mk(\d)/, ' Mk.$1');
    clean = clean.replace(/([a-z])([A-Z])/g, '$1 $2').trim();
    const id = inst.split('_').pop();
    return `${clean || inst} (#${id})`;
}

function getFilteredObjects() {
    if (!currentInspectionData || !currentInspectionData.rawSave) return [];
    const save = currentInspectionData.rawSave;
    const cat = CATEGORY_DEFINITIONS.find(c => c.id === selectedObjectCategory) || CATEGORY_DEFINITIONS[0];
    const query = objectSearchQuery.toLowerCase().trim();

    const results = [];
    for (const lvlName in save.levels) {
        for (const obj of save.levels[lvlName].objects) {
            const tp = obj.typePath || '';
            const inst = obj.instanceName || '';
            if (cat.match(tp, inst)) {
                if (!query || inst.toLowerCase().includes(query) || tp.toLowerCase().includes(query) || formatFriendlyName(obj).toLowerCase().includes(query)) {
                    results.push(obj);
                    if (results.length >= 75) break;
                }
            }
        }
        if (results.length >= 75) break;
    }
    return results;
}

function renderEditorTab(data) {
    const { header, pioneers, rawPlayerStates, totalClones } = data;
    const corruptPlayers = rawPlayerStates.filter(p => p.hasCorruptRespawn);

    if (!selectedEditPioneerName && pioneers.length > 0) {
        selectedEditPioneerName = pioneers[0].name;
    }
    const currentPioneer = pioneers.find(p => p.name === selectedEditPioneerName) || pioneers[0];
    const currentSlots = currentPioneer?.primaryState?.inventorySlots || 68;

    const filteredObjects = getFilteredObjects();
    if (!selectedEditObject && filteredObjects.length > 0) {
        selectedEditObject = filteredObjects[0];
    }

    return `
    <div style="display:flex; flex-direction:column; gap:1.25rem;">
        <!-- Banner Actions -->
        <div style="background:#131a24; border:1px solid var(--border); border-radius:8px; padding:1.25rem; display:flex; justify-content:space-between; align-items:center; flex-wrap:wrap; gap:1rem;">
            <div>
                <h3 style="margin:0 0 0.25rem 0; font-size:1.05rem; color:var(--text-main);">✏️ Interactive Save Editor</h3>
                <div style="font-size:0.85rem; color:var(--text-muted);">
                    Modify properties, grant care packages, or fix corruptions. Changes apply in memory until saved.
                </div>
            </div>
            <div style="display:flex; gap:0.6rem; flex-wrap:wrap;">
                <button class="btn btn-primary" onclick="FicsitInspector.saveToServer()" style="font-size:0.95rem; padding:0.6rem 1.25rem;">
                    💾 Save Directly to Server
                </button>
                <button class="btn btn-secondary" onclick="FicsitInspector.downloadEditedSave()" style="font-size:0.95rem; padding:0.6rem 1.25rem;">
                    ⬇️ Export .sav
                </button>
            </div>
        </div>

        <!-- 1. One-Click Quick Fixes (Save Doctor) -->
        <div class="editor-section">
            <h4 style="margin:0 0 0.75rem 0; font-size:0.95rem; color:var(--accent-primary);">🩺 1. Quick Fixes &amp; Health Utilities</h4>
            <div style="display:grid; grid-template-columns:repeat(auto-fit, minmax(280px, 1fr)); gap:0.85rem;">
                <div class="editor-action-card">
                    <div>
                        <strong>⚡ Fix Corrupted Respawns</strong>
                        <div style="font-size:0.8rem; color:var(--text-muted); margin-top:0.25rem;">
                            ${corruptPlayers.length > 0 
                                ? `<span style="color:#ef4444;">⚠️ Found ${corruptPlayers.length} player(s) (${corruptPlayers.map(p => p.name).join(', ')}) with crashing respawn tables.</span>`
                                : 'All respawn tables healthy.'}
                        </div>
                    </div>
                    <button class="btn ${corruptPlayers.length > 0 ? 'btn-primary' : 'btn-secondary'}" 
                            ${corruptPlayers.length === 0 ? 'disabled' : ''} 
                            onclick="FicsitInspector.fixCorruptRespawns()" 
                            style="margin-top:0.75rem; width:100%; justify-content:center;">
                        Fix Corrupt Respawns
                    </button>
                </div>

                <div class="editor-action-card">
                    <div>
                        <strong>👻 Purge Dormant Ghost Clones</strong>
                        <div style="font-size:0.8rem; color:var(--text-muted); margin-top:0.25rem;">
                            ${totalClones > 0 
                                ? `<span style="color:#c084fc;">Found ${totalClones} dormant clone bodies left behind from previous sessions.</span>`
                                : 'No dormant clones in world.'}
                        </div>
                    </div>
                    <button class="btn ${totalClones > 0 ? 'btn-primary' : 'btn-secondary'}" 
                            ${totalClones === 0 ? 'disabled' : ''} 
                            onclick="FicsitInspector.purgeGhostClones()" 
                            style="margin-top:0.75rem; width:100%; justify-content:center;">
                        Purge ${totalClones} Clone(s)
                    </button>
                </div>
            </div>
        </div>

        <!-- 2. Pioneer Tuning -->
        <div class="editor-section">
            <h4 style="margin:0 0 0.75rem 0; font-size:0.95rem; color:var(--accent-primary);">👷 2. Pioneer Tuning, Care Packages &amp; Teleport</h4>
            <div style="display:grid; grid-template-columns:repeat(auto-fit, minmax(240px, 1fr)); gap:1rem; align-items:end; margin-bottom:1rem;">
                <div>
                    <label style="font-size:0.8rem; color:var(--text-muted); display:block; margin-bottom:0.35rem;">Select Pioneer:</label>
                    <select id="edit-pioneer-select" onchange="FicsitInspector.changeSelectedPioneer(this.value)" 
                            style="width:100%; padding:0.5rem; background:#0f141c; color:#fff; border:1px solid var(--border); border-radius:6px;">
                        ${pioneers.map(p => `<option value="${p.name}" ${p.name === selectedEditPioneerName ? 'selected' : ''}>${p.name} (${p.primaryState.inventorySlots} slots)</option>`).join('')}
                    </select>
                </div>

                <div>
                    <label style="font-size:0.8rem; color:var(--text-muted); display:block; margin-bottom:0.35rem;">Inventory Slots:</label>
                    <div style="display:flex; gap:0.5rem;">
                        <input type="number" id="edit-pioneer-slots" value="${currentSlots}" min="10" max="500" 
                               style="width:90px; padding:0.5rem; background:#0f141c; color:#fff; border:1px solid var(--border); border-radius:6px;">
                        <button class="btn btn-secondary" onclick="FicsitInspector.applyPioneerSlots()">Set Slots</button>
                    </div>
                </div>

                <div>
                    <label style="font-size:0.8rem; color:var(--text-muted); display:block; margin-bottom:0.35rem;">Unstick / Recovery:</label>
                    <button class="btn btn-secondary" onclick="FicsitInspector.teleportSelectedPioneer()" style="width:100%; justify-content:center;">
                        🏠 Teleport to HUB
                    </button>
                </div>
            </div>

            <!-- Care Package / Item Granter -->
            <div style="background:rgba(0,0,0,0.25); border:1px dashed var(--border); border-radius:6px; padding:0.85rem;">
                <div style="font-size:0.85rem; font-weight:600; color:var(--text-main); margin-bottom:0.6rem; display:flex; align-items:center; gap:0.4rem;">
                    <span>🎁 Grant Items to Pioneer's Inventory</span>
                </div>
                <div style="display:grid; grid-template-columns:2fr 1fr auto; gap:0.65rem; align-items:end;">
                    <div>
                        <label style="font-size:0.75rem; color:var(--text-muted); display:block; margin-bottom:0.25rem;">Choose Item / Perk:</label>
                        <select id="grant-item-select" onchange="FicsitInspector.changeGrantItemPreset(this.value)"
                                style="width:100%; padding:0.5rem; background:#0f141c; color:#fff; border:1px solid var(--border); border-radius:6px; font-size:0.85rem;">
                            <option value="/Game/FactoryGame/Resource/Parts/ResourceSinkCoupon/Desc_ResourceSinkCoupon.Desc_ResourceSinkCoupon_C" data-qty="50">🎫 FICSIT Coupons (50x)</option>
                            <option value="/Game/FactoryGame/Resource/Environment/CrashSites/Desc_HardDrive.Desc_HardDrive_C" data-qty="1">💾 Hard Drive (1x)</option>
                            <option value="/Game/FactoryGame/Prototype/WAT/Desc_WAT1.Desc_WAT1_C" data-qty="5">🔮 Mercer Sphere (5x)</option>
                            <option value="/Game/FactoryGame/Prototype/WAT/Desc_WAT2.Desc_WAT2_C" data-qty="5">🌀 Somersloop (5x)</option>
                            <option value="/Game/FactoryGame/Resource/Parts/Cement/Desc_Cement.Desc_Cement_C" data-qty="500">🧱 Concrete (500x)</option>
                            <option value="/Game/FactoryGame/Resource/Parts/IronPlateReinforced/Desc_IronPlateReinforced.Desc_IronPlateReinforced_C" data-qty="100">⚙️ Reinforced Plate (100x)</option>
                            <option value="/Game/FactoryGame/Resource/Parts/ModularFrame/Desc_ModularFrame.Desc_ModularFrame_C" data-qty="50">🔩 Modular Frame (50x)</option>
                            <option value="/Game/FactoryGame/Resource/Parts/SAMFluctuator/Desc_SAMFluctuator.Desc_SAMFluctuator_C" data-qty="50">🔋 SAM Fluctuator (50x)</option>
                        </select>
                    </div>

                    <div>
                        <label style="font-size:0.75rem; color:var(--text-muted); display:block; margin-bottom:0.25rem;">Quantity:</label>
                        <input type="number" id="grant-item-qty" value="50" min="1" max="5000" 
                               style="width:100%; padding:0.5rem; background:#0f141c; color:#fff; border:1px solid var(--border); border-radius:6px; box-sizing:border-box; font-size:0.85rem;">
                    </div>

                    <button class="btn btn-primary" onclick="FicsitInspector.grantItemToSelectedPioneer()" style="font-size:0.85rem; padding:0.55rem 1rem;">
                        🎁 Add to Inventory
                    </button>
                </div>
            </div>
        </div>

        <!-- 3. Session Settings -->
        <div class="editor-section">
            <h4 style="margin:0 0 0.75rem 0; font-size:0.95rem; color:var(--accent-primary);">🗺️ 3. Session Metadata</h4>
            <div style="display:flex; gap:0.75rem; flex-wrap:wrap; align-items:end;">
                <div style="flex:1; min-width:240px;">
                    <label style="font-size:0.8rem; color:var(--text-muted); display:block; margin-bottom:0.35rem;">Session Name:</label>
                    <input type="text" id="edit-session-name" value="${header.sessionName}" 
                           style="width:100%; padding:0.5rem; background:#0f141c; color:#fff; border:1px solid var(--border); border-radius:6px; box-sizing:border-box;">
                </div>
                <button class="btn btn-secondary" onclick="FicsitInspector.applySessionName()">Rename Session</button>
            </div>
        </div>

        <!-- 4. Category-Based World Object & Property Inspector -->
        <div class="editor-section">
            <h4 style="margin:0 0 0.75rem 0; font-size:0.95rem; color:var(--accent-primary);">🔍 4. World Object Browser &amp; Property Editor</h4>
            <div style="display:grid; grid-template-columns:repeat(auto-fit, minmax(260px, 1fr)); gap:0.75rem; margin-bottom:0.85rem;">
                <div>
                    <label style="font-size:0.8rem; color:var(--text-muted); display:block; margin-bottom:0.35rem;">Browse by Friendly Category:</label>
                    <select onchange="FicsitInspector.changeObjectCategory(this.value)" 
                            style="width:100%; padding:0.5rem; background:#0f141c; color:#fff; border:1px solid var(--border); border-radius:6px; font-size:0.85rem;">
                        ${CATEGORY_DEFINITIONS.map(c => `<option value="${c.id}" ${c.id === selectedObjectCategory ? 'selected' : ''}>${c.label}</option>`).join('')}
                    </select>
                </div>

                <div>
                    <label style="font-size:0.8rem; color:var(--text-muted); display:block; margin-bottom:0.35rem;">Optional Filter (e.g. coal, phase, sink):</label>
                    <input type="text" id="object-search-input" value="${objectSearchQuery}" placeholder="Type to filter..." 
                           oninput="FicsitInspector.changeSearchFilter(this.value)"
                           style="width:100%; padding:0.5rem; background:#0f141c; color:#fff; border:1px solid var(--border); border-radius:6px; box-sizing:border-box; font-size:0.85rem;">
                </div>
            </div>

            <div style="margin-bottom:0.75rem;">
                <label style="font-size:0.8rem; color:var(--text-muted); display:block; margin-bottom:0.35rem;">
                    Select Object (${filteredObjects.length} found):
                </label>
                <select onchange="FicsitInspector.selectObjectForEdit(this.value)" 
                        style="width:100%; padding:0.55rem; background:#0f141c; color:#fff; border:1px solid var(--border); border-radius:6px; font-size:0.85rem;">
                    ${filteredObjects.map(o => `<option value="${o.instanceName}" ${selectedEditObject?.instanceName === o.instanceName ? 'selected' : ''}>${formatFriendlyName(o)}</option>`).join('')}
                </select>
            </div>

            ${selectedEditObject ? `
                <div style="margin-top:1rem;">
                    <div style="display:flex; justify-content:space-between; align-items:center; margin-bottom:0.4rem; flex-wrap:wrap; gap:0.5rem;">
                        <span style="font-size:0.85rem; font-weight:600; color:var(--accent-primary);">${formatFriendlyName(selectedEditObject)}</span>
                        <span style="font-size:0.75rem; color:var(--text-muted); font-family:monospace;">${selectedEditObject.instanceName.split('.').pop()}</span>
                    </div>
                    <textarea id="raw-object-json" rows="12" 
                              style="width:100%; font-family:monospace; font-size:0.8rem; background:#0b0f16; color:#a7f3d0; border:1px solid var(--border); border-radius:6px; padding:0.75rem; box-sizing:border-box;">${JSON.stringify(selectedEditObject.properties || {}, null, 2)}</textarea>
                    <div style="margin-top:0.5rem; display:flex; justify-content:space-between; align-items:center;">
                        <span style="font-size:0.75rem; color:var(--text-muted);">Edit JSON values directly, then click Apply.</span>
                        <button class="btn btn-primary" onclick="FicsitInspector.saveObjectProperties()">💾 Apply Changes to Object</button>
                    </div>
                </div>
            ` : ''}
        </div>
    </div>`;
}

const INSPECTOR_STYLES = `
    .inspector-modal-backdrop {
        position: fixed; top: 0; left: 0; right: 0; bottom: 0;
        background: rgba(10, 14, 20, 0.82);
        backdrop-filter: blur(4px); -webkit-backdrop-filter: blur(4px);
        z-index: 9999; display: flex; align-items: center; justify-content: center;
        padding: 1.5rem; box-sizing: border-box;
    }
    .inspector-card {
        background: var(--bg-surface, #131a24); border: 1px solid var(--border, rgba(255,255,255,0.1));
        border-radius: 12px; box-shadow: 0 20px 40px rgba(0,0,0,0.6);
        width: 100%; max-width: 100%; max-height: 90vh;
        display: flex; flex-direction: column; overflow: hidden;
        color: var(--text-main, #e2e8f0); animation: modalFadeIn 0.15s ease-out;
    }
    @keyframes modalFadeIn { from { opacity: 0; transform: scale(0.97); } to { opacity: 1; transform: scale(1); } }
    .inspector-header { padding: 1.25rem 1.5rem; border-bottom: 1px solid var(--border, rgba(255,255,255,0.1)); background: #131a24; display: flex; justify-content: space-between; align-items: center; }
    .modal-close-btn { background: transparent; border: none; color: var(--text-muted, #94a3b8); font-size: 1.5rem; cursor: pointer; padding: 0.25rem 0.5rem; border-radius: 4px; line-height: 1; }
    .modal-close-btn:hover { color: #ffffff; background: rgba(255, 255, 255, 0.1); }
    .inspector-tabs { display: flex; background: #111722; border-bottom: 1px solid var(--border, rgba(255,255,255,0.1)); padding: 0.35rem 1rem 0 1rem; gap: 0.5rem; overflow-x: auto; }
    .inspector-tab-btn { background: transparent; border: none; border-bottom: 2px solid transparent; color: var(--text-muted, #94a3b8); font-size: 0.88rem; font-weight: 500; padding: 0.6rem 0.9rem; cursor: pointer; white-space: nowrap; transition: all 0.15s; }
    .inspector-tab-btn:hover { color: var(--text-main, #e2e8f0); }
    .inspector-tab-btn.active { color: var(--accent-primary, #fa8029); border-bottom-color: var(--accent-primary, #fa8029); }
    .inspector-body { padding: 1.5rem; overflow-y: auto; flex: 1; }
    .inspector-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(180px, 1fr)); gap: 0.85rem; }
    .inspector-box { background: #131a24; border: 1px solid var(--border, rgba(255,255,255,0.1)); border-radius: 6px; padding: 0.85rem 1rem; }
    .inspector-box .box-label { font-size: 0.72rem; font-weight: 600; color: var(--text-muted, #94a3b8); letter-spacing: 0.05em; margin-bottom: 0.3rem; }
    .inspector-box .box-value { font-size: 0.98rem; font-weight: 600; color: var(--text-main, #e2e8f0); word-break: break-word; }
    .inspector-table { width: 100%; border-collapse: collapse; border: 1px solid var(--border, rgba(255,255,255,0.1)); border-radius: 6px; overflow: hidden; background: #131a24; }
    .inspector-table th, .inspector-table td { padding: 0.6rem 0.85rem; text-align: left; border-bottom: 1px solid var(--border, rgba(255,255,255,0.1)); }
    .inspector-table th { background: #0e141c; color: var(--text-muted, #94a3b8); font-size: 0.8rem; }
    .platform-pill { background: rgba(59, 130, 246, 0.15); color: #60a5fa; border: 1px solid rgba(59, 130, 246, 0.4); font-size: 0.75rem; font-weight: 600; padding: 0.15rem 0.45rem; border-radius: 4px; }
    .clone-pill { background: rgba(168, 85, 247, 0.15); color: #c084fc; border: 1px solid rgba(168, 85, 247, 0.4); font-size: 0.72rem; font-weight: 600; padding: 0.15rem 0.45rem; border-radius: 4px; margin-left: 0.4rem; }
    .health-badge { font-size: 0.8rem; font-weight: 600; padding: 0.3rem 0.65rem; border-radius: 6px; white-space: nowrap; }
    .health-badge.ok { background: rgba(34, 197, 94, 0.15); border: 1px solid rgba(34, 197, 94, 0.4); color: #4ade80; }
    .health-badge.error { background: rgba(239, 68, 68, 0.15); border: 1px solid rgba(239, 68, 68, 0.4); color: #f87171; }
    .health-badge.warn { background: rgba(245, 158, 11, 0.15); border: 1px solid rgba(245, 158, 11, 0.4); color: #fbbf24; }
    .diag-item { padding: 0.9rem 1rem; border-radius: 6px; border: 1px solid var(--border, rgba(255,255,255,0.1)); background: #131a24; }
    .diag-item.diag-pass { border-left: 4px solid #22c55e; }
    .diag-item.diag-error { border-left: 4px solid #ef4444; background: rgba(239, 68, 68, 0.05); }
    .diag-item.diag-warn { border-left: 4px solid #f59e0b; background: rgba(245, 158, 11, 0.05); }
    .diag-item.diag-info { border-left: 4px solid #3b82f6; }
    .diag-tag { font-size: 0.7rem; font-weight: 700; padding: 0.1rem 0.4rem; border-radius: 4px; background: rgba(255, 255, 255, 0.08); color: var(--text-muted, #94a3b8); }
    .mod-settings-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(280px, 1fr)); gap: 0.65rem; }
    .mod-setting-pill { background: #131a24; border: 1px solid var(--border, rgba(255,255,255,0.1)); padding: 0.6rem 0.75rem; border-radius: 6px; font-size: 0.82rem; overflow-wrap: anywhere; word-break: break-word; }
    .editor-section { background: #111722; border: 1px solid var(--border, rgba(255,255,255,0.1)); border-radius: 8px; padding: 1.25rem; }
    .editor-action-card { background: #131a24; border: 1px solid var(--border, rgba(255,255,255,0.1)); border-radius: 6px; padding: 1rem; display: flex; flex-direction: column; justify-content: space-between; }
    .inspector-spinner { width: 42px; height: 42px; border: 3px solid rgba(250, 128, 41, 0.2); border-top-color: var(--accent-primary, #fa8029); border-radius: 50%; animation: spin 0.8s linear infinite; margin: 0 auto; }
    @keyframes spin { to { transform: rotate(360deg); } }
`;

function ensureStylesInjected() {
    if (typeof document === 'undefined') return;
    if (document.getElementById('ficsit-inspector-styles')) return;
    const style = document.createElement('style');
    style.id = 'ficsit-inspector-styles';
    style.textContent = INSPECTOR_STYLES;
    document.head.appendChild(style);
}

// Global modal overlay element
function ensureModalRoot() {
    let root = document.getElementById('ficsit-inspector-modal');
    if (!root) {
        root = document.createElement('div');
        root.id = 'ficsit-inspector-modal';
        root.className = 'inspector-modal-backdrop';
        root.onclick = (e) => {
            if (e.target === root) FicsitInspector.close();
        };
        document.body.appendChild(root);
    }
    return root;
}

export function openSaveInspector(urlOrFile, saveName, options = {}) {
    ensureStylesInjected();
    isAdminMode = Boolean(options.admin);
    targetContainerId = options.containerId || null;
    currentSaveName = saveName || (typeof urlOrFile === 'string' ? urlOrFile.split('/').pop() : 'save.sav');
    const root = ensureRenderRoot();
    activeTab = 'summary';
    editCounter = 0;
    editNotice = null;
    selectedEditObject = null;
    searchResults = [];
    if (!targetContainerId) {
        root.style.display = 'flex';
    }
    root.innerHTML = `
        <div class="inspector-card" style="text-align:center; padding:3rem;">
            <div class="inspector-spinner"></div>
            <h3 style="margin-top:1.25rem;">Inspecting Save File...</h3>
            <p id="inspector-loading-status" style="color:var(--text-muted); font-size:0.9rem;">Downloading save into memory...</p>
        </div>
    `;

    const updateStatus = (msg) => {
        const el = document.getElementById('inspector-loading-status');
        if (el) el.textContent = msg;
    };

    if (urlOrFile instanceof File || urlOrFile instanceof Blob) {
        updateStatus('Reading local file...');
        urlOrFile.arrayBuffer().then(buf => {
            updateStatus('Decompressing GVAS save chunks &amp; parsing AST...');
            setTimeout(() => {
                try {
                    const data = inspectSaveBuffer(buf, urlOrFile.name || 'local.sav');
                    root.innerHTML = renderModalContent(data);
                } catch (err) {
                    root.innerHTML = `<div class="inspector-card" style="padding:2rem; color:#ef4444;"><h3>Inspection Failed</h3><p>${err.message}</p><button class="btn btn-secondary" onclick="FicsitInspector.close()">Close</button></div>`;
                }
            }, 30);
        });
    } else {
        updateStatus(`Fetching ${urlOrFile}...`);
        fetch(urlOrFile)
            .then(res => {
                if (!res.ok) throw new Error(`HTTP ${res.status}: ${res.statusText}`);
                return res.arrayBuffer();
            })
            .then(buf => {
                updateStatus('Decompressing GVAS save chunks &amp; analyzing objects...');
                setTimeout(() => {
                    try {
                        const data = inspectSaveBuffer(buf, saveName || urlOrFile.split('/').pop());
                        root.innerHTML = renderModalContent(data);
                    } catch (err) {
                        root.innerHTML = `<div class="inspector-card" style="padding:2rem; color:#ef4444;"><h3>Inspection Failed</h3><p>${err.message}</p><button class="btn btn-secondary" onclick="FicsitInspector.close()">Close</button></div>`;
                    }
                }, 30);
            })
            .catch(err => {
                root.innerHTML = `<div class="inspector-card" style="padding:2rem; color:#ef4444;"><h3>Network or Load Error</h3><p>${err.message}</p><button class="btn btn-secondary" onclick="FicsitInspector.close()">Close</button></div>`;
            });
    }
}

export function switchTab(tab) {
    if (tab === 'editor' && !isAdminMode) return;
    activeTab = tab;
    if (currentInspectionData) {
        const root = ensureRenderRoot();
        root.innerHTML = renderModalContent(currentInspectionData);
    }
}

export function toggleDormantClones() {
    showDormantClones = !showDormantClones;
    if (currentInspectionData) {
        const root = ensureRenderRoot();
        root.innerHTML = renderModalContent(currentInspectionData);
    }
}

export function dismissNotice() {
    editNotice = null;
    if (currentInspectionData) {
        const root = ensureRenderRoot();
        root.innerHTML = renderModalContent(currentInspectionData);
    }
}

export function changeSelectedPioneer(name) {
    selectedEditPioneerName = name;
    if (currentInspectionData) {
        const root = ensureRenderRoot();
        root.innerHTML = renderModalContent(currentInspectionData);
    }
}

// 1-Click Fix Corrupt Respawns
export function fixCorruptRespawns() {
    if (!currentInspectionData || !currentInspectionData.rawSave) return;
    const save = currentInspectionData.rawSave;
    let fixed = 0;
    for (const lvlName in save.levels) {
        for (const obj of save.levels[lvlName].objects) {
            if (obj.typePath && obj.typePath.includes('BP_PlayerState')) {
                if (obj.properties?.mInventoryToRespawnWith) {
                    delete obj.properties.mInventoryToRespawnWith;
                    fixed++;
                }
            }
        }
    }
    recomputeAndRender(`Successfully cleared corrupted respawn inventory on ${fixed} player state(s)!`);
}

// 1-Click Purge Ghost Clones
export function purgeGhostClones() {
    if (!currentInspectionData || !currentInspectionData.rawSave) return;
    const save = currentInspectionData.rawSave;
    const clones = currentInspectionData.rawPlayerStates.filter(p => !p.isPrimary);
    if (clones.length === 0) return;

    let totalDeleted = 0;
    for (const clone of clones) {
        const target = clone.instanceName;
        for (const lvlName in save.levels) {
            const before = save.levels[lvlName].objects.length;
            save.levels[lvlName].objects = save.levels[lvlName].objects.filter(
                o => o.instanceName !== target && !o.instanceName.startsWith(target + '.')
            );
            totalDeleted += (before - save.levels[lvlName].objects.length);
        }
    }
    recomputeAndRender(`Successfully purged ${clones.length} dormant clone body/bodies (${totalDeleted} total actors & sub-objects removed)!`);
}

// Teleport Pioneer to HUB
export function teleportSelectedPioneer() {
    if (!currentInspectionData || !currentInspectionData.rawSave) return;
    const targetPioneer = currentInspectionData.pioneers.find(p => p.name === selectedEditPioneerName);
    if (!targetPioneer) return;

    const hubPos = currentInspectionData.worldInfo.hubTransform || { x: 1400, y: 700, z: 50100 };
    const inst = targetPioneer.primaryState.instanceName;
    const save = currentInspectionData.rawSave;

    for (const lvlName in save.levels) {
        for (const obj of save.levels[lvlName].objects) {
            if (obj.instanceName === inst) {
                if (!obj.properties) obj.properties = {};
                obj.properties.mLastSafeCharacterLocation = {
                    type: 'StructProperty',
                    name: 'mLastSafeCharacterLocation',
                    propertyTagType: {
                        name: 'StructProperty',
                        children: [{ name: 'Vector', children: [{ name: '/Script/CoreUObject', children: [] }] }]
                    },
                    flags: 16,
                    value: { x: hubPos.x, y: hubPos.y, z: hubPos.z + 150 }
                };
            }
        }
    }
    recomputeAndRender(`Teleported ${targetPioneer.name} back to HUB safe coordinates!`);
}

// Update Pioneer Inventory Slots
export function applyPioneerSlots() {
    if (!currentInspectionData || !currentInspectionData.rawSave) return;
    const input = document.getElementById('edit-pioneer-slots');
    if (!input) return;
    const slots = parseInt(input.value, 10);
    if (isNaN(slots) || slots <= 0) return;

    const targetPioneer = currentInspectionData.pioneers.find(p => p.name === selectedEditPioneerName);
    if (!targetPioneer) return;

    const inst = targetPioneer.primaryState.instanceName;
    const save = currentInspectionData.rawSave;

    for (const lvlName in save.levels) {
        for (const obj of save.levels[lvlName].objects) {
            if (obj.instanceName === inst) {
                if (!obj.properties) obj.properties = {};
                obj.properties.mNumObservedInventorySlots = {
                    type: 'IntProperty',
                    name: 'mNumObservedInventorySlots',
                    propertyTagType: { name: 'IntProperty', children: [] },
                    flags: 16,
                    value: slots
                };
            }
        }
    }
    recomputeAndRender(`Updated ${targetPioneer.name}'s inventory capacity to ${slots} slots!`);
}

// Preset Handler for Item Granter
export function changeGrantItemPreset() {
    const sel = document.getElementById('grant-item-select');
    const qtyInput = document.getElementById('grant-item-qty');
    if (!sel || !qtyInput) return;
    const opt = sel.options[sel.selectedIndex];
    const defaultQty = opt?.getAttribute('data-qty');
    if (defaultQty) {
        qtyInput.value = defaultQty;
    }
}

// Helper to create an InventoryStack struct for Satisfactory saves
function createInventoryItemStruct(itemPath) {
    return {
        type: 'StructProperty',
        name: 'Item',
        propertyTagType: {
            name: 'StructProperty',
            children: [{ name: 'InventoryItem', children: [{ name: '/Script/FactoryGame', children: [] }] }]
        },
        flags: 8,
        value: {
            itemReference: { levelName: '', pathName: itemPath },
            itemState: { hasValidStruct: false }
        }
    };
}

// Grant Item to Pioneer (Option 3: Safe space check & warning on full inventory)
export function grantItemToSelectedPioneer() {
    if (!currentInspectionData || !currentInspectionData.rawSave) return;
    const save = currentInspectionData.rawSave;
    const sel = document.getElementById('grant-item-select');
    const qtyInput = document.getElementById('grant-item-qty');
    if (!sel || !qtyInput) return;

    const itemPath = sel.value;
    const opt = sel.options[sel.selectedIndex];
    const itemLabel = opt?.text || 'Item';
    const qty = parseInt(qtyInput.value, 10);
    if (isNaN(qty) || qty <= 0) {
        alert('Please enter a valid positive quantity.');
        return;
    }

    const targetPioneer = currentInspectionData.pioneers.find(p => p.name === selectedEditPioneerName);
    if (!targetPioneer) return;

    const ps = getObjectRef(targetPioneer.primaryState.instanceName);
    let invObj = null;

    // 1. Check active pawn inventory
    const pawnPath = ps?.properties?.mOwnedPawn?.value?.pathName;
    if (pawnPath) {
        invObj = getObjectRef(pawnPath + '.inventory');
    }

    // 2. Fallback search for Char_Player matching pioneer name
    if (!invObj) {
        for (const lvl in save.levels) {
            for (const obj of save.levels[lvl].objects) {
                if (obj.typePath && obj.typePath.includes('Char_Player') && obj.properties?.mCachedPlayerName?.value === targetPioneer.name) {
                    invObj = getObjectRef(obj.instanceName + '.inventory');
                    if (invObj) break;
                }
            }
            if (invObj) break;
        }
    }

    // 3. Fallback to offline respawn table if pioneer has no live pawn
    let stacksContainer = null;
    let isRespawnTable = false;
    if (invObj && invObj.properties?.mInventoryStacks?.values) {
        stacksContainer = invObj.properties.mInventoryStacks.values;
    } else if (ps?.properties?.mInventoryToRespawnWith?.value?.properties?.mInventoryStacks?.values) {
        stacksContainer = ps.properties.mInventoryToRespawnWith.value.properties.mInventoryStacks.values;
        isRespawnTable = true;
    }

    if (!stacksContainer) {
        alert(`Could not find an active inventory component for ${targetPioneer.name}. They may need to log into the world once.`);
        return;
    }

    // A. Check if the item already exists in a stack (merge / top-up)
    let existingStack = stacksContainer.find(s => s.properties?.Item?.value?.itemReference?.pathName === itemPath);
    if (existingStack && existingStack.properties?.NumItems) {
        const cur = existingStack.properties.NumItems.value || 0;
        existingStack.properties.NumItems.value = cur + qty;
        recomputeAndRender(`Added ${qty}x to existing stack for ${targetPioneer.name} (now ${cur + qty} total)!`);
        return;
    }

    // B. Check for first empty slot
    let emptySlot = stacksContainer.find(s => !s.properties?.Item?.value?.itemReference?.pathName || s.properties?.NumItems?.value === 0);
    if (emptySlot) {
        emptySlot.properties = {
            Item: createInventoryItemStruct(itemPath),
            NumItems: {
                type: 'IntProperty',
                name: 'NumItems',
                propertyTagType: { name: 'IntProperty', children: [] },
                value: qty
            }
        };
        recomputeAndRender(`Granted ${qty}x ${itemLabel.split('(')[0].trim()} to ${targetPioneer.name}!`);
        return;
    }

    // C. Inventory is 100% full (Option 3: Safe Validation Prompt)
    const totalSlots = stacksContainer.length;
    const expandConfirmed = confirm(
        `${targetPioneer.name}'s inventory is completely full (${totalSlots}/${totalSlots} slots occupied)!\n\n` +
        `Would you like to expand their inventory by 1 slot to fit this care package?`
    );

    if (!expandConfirmed) {
        return; // User canceled; leave untouched
    }

    // Auto-expand inventory by 1 slot
    if (invObj && invObj.properties?.mAdjustedSizeDiff) {
        invObj.properties.mAdjustedSizeDiff.value = (invObj.properties.mAdjustedSizeDiff.value || 0) + 1;
    }
    if (ps && ps.properties?.mNumObservedInventorySlots) {
        ps.properties.mNumObservedInventorySlots.value = (ps.properties.mNumObservedInventorySlots.value || totalSlots) + 1;
    }

    stacksContainer.push({
        type: 'InventoryStack',
        properties: {
            Item: createInventoryItemStruct(itemPath),
            NumItems: {
                type: 'IntProperty',
                name: 'NumItems',
                propertyTagType: { name: 'IntProperty', children: [] },
                value: qty
            }
        }
    });

    recomputeAndRender(`Expanded inventory (+1 slot) and granted ${qty}x ${itemLabel.split('(')[0].trim()} to ${targetPioneer.name}!`);
}

// Apply Session Rename
export function applySessionName() {
    if (!currentInspectionData || !currentInspectionData.rawSave) return;
    const input = document.getElementById('edit-session-name');
    if (!input) return;
    const newName = input.value.trim();
    if (!newName) return;

    const save = currentInspectionData.rawSave;
    save.header.sessionName = newName;
    save.header.saveName = newName;

    // Update in GameState if present
    for (const lvlName in save.levels) {
        for (const obj of save.levels[lvlName].objects) {
            if (obj.instanceName && obj.instanceName.includes('BP_GameState')) {
                if (obj.properties?.mReplicatedSessionName) {
                    obj.properties.mReplicatedSessionName.value = newName;
                }
            }
        }
    }
    recomputeAndRender(`Renamed session to "${newName}"!`);
}

// Category and Search Handlers for Object Browser
export function changeObjectCategory(catId) {
    selectedObjectCategory = catId;
    const filtered = getFilteredObjects();
    selectedEditObject = filtered.length > 0 ? filtered[0] : null;
    if (currentInspectionData) {
        const root = ensureRenderRoot();
        root.innerHTML = renderModalContent(currentInspectionData);
    }
}

export function changeSearchFilter(query) {
    objectSearchQuery = query;
    const filtered = getFilteredObjects();
    selectedEditObject = filtered.length > 0 ? filtered[0] : null;
    if (currentInspectionData) {
        const root = ensureRenderRoot();
        root.innerHTML = renderModalContent(currentInspectionData);
        // Restore focus and cursor to the search input so typing is uninterrupted
        const input = document.getElementById('object-search-input');
        if (input) {
            input.focus();
            input.selectionStart = input.selectionEnd = input.value.length;
        }
    }
}

// Search World Objects (legacy fallback)
export function searchObjects(query) {
    changeSearchFilter(query);
}

function getObjectRef(instanceName) {
    if (!currentInspectionData || !currentInspectionData.rawSave) return null;
    const save = currentInspectionData.rawSave;
    for (const lvlName in save.levels) {
        for (const obj of save.levels[lvlName].objects) {
            if (obj.instanceName === instanceName) {
                return obj;
            }
        }
    }
    return null;
}

export function selectObjectForEdit(instanceName) {
    selectedEditObject = getObjectRef(instanceName);
    if (currentInspectionData) {
        const root = ensureRenderRoot();
        root.innerHTML = renderModalContent(currentInspectionData);
    }
}

export function saveObjectProperties() {
    if (!selectedEditObject) return;
    const textarea = document.getElementById('raw-object-json');
    if (!textarea) return;
    try {
        const parsed = JSON.parse(textarea.value);
        selectedEditObject.properties = parsed;
        recomputeAndRender(`Successfully updated properties on ${selectedEditObject.instanceName}!`);
    } catch (e) {
        alert('Invalid JSON: ' + e.message);
    }
}

// Re-serialize and trigger browser download
export function downloadEditedSave() {
    if (!currentInspectionData || !currentInspectionData.rawSave) return;
    const save = currentInspectionData.rawSave;
    const chunks = [];
    let headerArray = null;

    Parser.WriteSave(
        save,
        (hdr) => { headerArray = new Uint8Array(hdr); },
        (chunk) => { chunks.push(new Uint8Array(chunk)); }
    );

    const totalLen = headerArray.byteLength + chunks.reduce((acc, c) => acc + c.byteLength, 0);
    const merged = new Uint8Array(totalLen);
    merged.set(headerArray, 0);
    let offset = headerArray.byteLength;
    for (const c of chunks) {
        merged.set(c, offset);
        offset += c.byteLength;
    }

    const blob = new Blob([merged], { type: 'application/octet-stream' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    const baseName = (save.header.saveName || save.header.sessionName || 'satisfactory_save').replace(/\.sav$/i, '');
    a.href = url;
    a.download = `${baseName}_edited.sav`;
    document.body.appendChild(a);
    a.click();
    document.body.removeChild(a);
    URL.revokeObjectURL(url);
}

// Save modified AST directly back to the server via Admin API
export async function saveToServer() {
    if (!currentInspectionData || !currentInspectionData.rawSave) return;
    const save = currentInspectionData.rawSave;
    const chunks = [];
    let headerArray = null;

    Parser.WriteSave(
        save,
        (hdr) => { headerArray = new Uint8Array(hdr); },
        (chunk) => { chunks.push(new Uint8Array(chunk)); }
    );

    const totalLen = headerArray.byteLength + chunks.reduce((acc, c) => acc + c.byteLength, 0);
    const merged = new Uint8Array(totalLen);
    merged.set(headerArray, 0);
    let offset = headerArray.byteLength;
    for (const c of chunks) {
        merged.set(c, offset);
        offset += c.byteLength;
    }

    const filename = currentSaveName || (save.header.saveName || save.header.sessionName || 'server') + '.sav';
    try {
        const res = await fetch(`/admin/api/upload-save?filename=${encodeURIComponent(filename)}`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/octet-stream' },
            body: merged
        });
        const result = await res.json();
        if (result.success) {
            editCounter = 0;
            recomputeAndRender(`Successfully saved ${filename} directly to server! Backup created at ${result.backup}`);
        } else {
            alert('Failed to save to server: ' + (result.error || 'Unknown error'));
        }
    } catch (e) {
        alert('Error saving to server: ' + e.message);
    }
}

export function openAdminWorkspace(urlOrFile, saveName, containerId) {
    openSaveInspector(urlOrFile, saveName, { admin: true, containerId });
}

export function close() {
    if (targetContainerId) return;
    const root = document.getElementById('ficsit-inspector-modal');
    if (root) root.style.display = 'none';
}

// Attach to window
if (typeof window !== 'undefined') {
    window.FicsitInspector = {
        open: openSaveInspector,
        openAdminWorkspace,
        close,
        switchTab,
        toggleDormantClones,
        dismissNotice,
        changeSelectedPioneer,
        fixCorruptRespawns,
        purgeGhostClones,
        teleportSelectedPioneer,
        applyPioneerSlots,
        changeGrantItemPreset,
        grantItemToSelectedPioneer,
        applySessionName,
        changeObjectCategory,
        changeSearchFilter,
        searchObjects,
        selectObjectForEdit,
        saveObjectProperties,
        saveToServer,
        downloadEditedSave,
        inspectSaveBuffer
    };
    window.openSaveInspector = openSaveInspector;
    window.openAdminWorkspace = openAdminWorkspace;
}
