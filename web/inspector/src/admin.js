// =============================================================================
// FICSIT Admin Control Center - Purpose-Built Management Workstation
// =============================================================================

import { friendlyItemName, getAllAvailableItems, scanSaveForItemDescriptors } from './items.js';
import { ALTERNATE_RECIPES, MAM_TREES, isAlternateUnlocked, isMAMNodeUnlocked } from './recipes.js';
export { friendlyItemName, getAllAvailableItems, scanSaveForItemDescriptors };

export function formatSatisfactoryDate(date = new Date()) {
    const pad = (n) => String(n).padStart(2, '0');
    const dd = pad(date.getDate());
    const mm = pad(date.getMonth() + 1);
    const yy = String(date.getFullYear()).slice(-2);
    const HH = pad(date.getHours());
    const MM = pad(date.getMinutes());
    const ss = pad(date.getSeconds());
    return `${dd}${mm}${yy}-${HH}${MM}${ss}`;
}

export function generatePortalSaveName(save, currentFilename) {
    let serverOrSession = save?.header?.sessionName || '';
    if (!serverOrSession && currentFilename) {
        const base = currentFilename.replace(/\.sav$/i, '');
        const match = base.match(/^([^_]+)/);
        serverOrSession = match ? match[1] : base;
    }
    if (!serverOrSession) {
        serverOrSession = 'Satisfactory-Server';
    }
    serverOrSession = serverOrSession.replace(/[<>:"/\\|?*]/g, '').trim().replace(/\s+/g, '-');
    const dateStr = formatSatisfactoryDate(new Date());
    return `${serverOrSession}-${dateStr}-portalsave.sav`;
}

export function findPioneerInventory(targetPioneer, save) {
    if (!targetPioneer || !save) return { ps: null, invObj: null, stacks: null, isRespawnTable: false };
    const psInst = targetPioneer?.primaryState?.instanceName;
    let ps = null;
    let invObj = null;

    for (const lvl in save.levels) {
        for (const obj of save.levels[lvl].objects) {
            if (obj.instanceName === psInst) {
                ps = obj;
                break;
            }
        }
        if (ps) break;
    }

    const pawnPath = ps?.properties?.mOwnedPawn?.value?.pathName;
    if (pawnPath) {
        for (const lvl in save.levels) {
            for (const obj of save.levels[lvl].objects) {
                if (obj.instanceName === pawnPath + '.inventory') {
                    invObj = obj;
                    break;
                }
            }
            if (invObj) break;
        }
    }

    if (!invObj) {
        for (const lvl in save.levels) {
            for (const obj of save.levels[lvl].objects) {
                if (obj.typePath?.includes('Char_Player') && obj.properties?.mCachedPlayerName?.value === targetPioneer.name) {
                    const candidate = obj.instanceName + '.inventory';
                    for (const obj2 of save.levels[lvl].objects) {
                        if (obj2.instanceName === candidate) {
                            invObj = obj2;
                            break;
                        }
                    }
                    if (invObj) break;
                }
            }
            if (invObj) break;
        }
    }

    let stacks = null;
    let isRespawnTable = false;
    if (invObj && invObj.properties?.mInventoryStacks?.values) {
        stacks = invObj.properties.mInventoryStacks.values;
    } else if (ps?.properties?.mInventoryToRespawnWith?.value?.properties?.mInventoryStacks?.values) {
        stacks = ps.properties.mInventoryToRespawnWith.value.properties.mInventoryStacks.values;
        isRespawnTable = true;
    }

    return { ps, invObj, stacks, isRespawnTable };
}

export function createInventoryItemStruct(itemPath) {
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

export function createRespawnInventoryStruct(slotCount = 63) {
    const emptyStacks = [];
    for (let i = 0; i < slotCount; i++) {
        emptyStacks.push({
            type: 'InventoryStack',
            properties: {
                Item: createInventoryItemStruct(''),
                NumItems: {
                    type: 'IntProperty',
                    name: 'NumItems',
                    propertyTagType: { name: 'IntProperty', children: [] },
                    value: 0
                }
            }
        });
    }

    const createEquipmentHolder = (count) => {
        const eqStacks = [];
        for (let i = 0; i < count; i++) {
            eqStacks.push({
                type: 'InventoryStack',
                properties: {
                    Item: createInventoryItemStruct(''),
                    NumItems: {
                        type: 'IntProperty',
                        name: 'NumItems',
                        propertyTagType: { name: 'IntProperty', children: [] },
                        value: 0
                    }
                }
            });
        }
        return {
            type: 'StructProperty',
            properties: {
                mArrayOfInventoryStacks: {
                    type: 'ArrayProperty',
                    name: 'mArrayOfInventoryStacks',
                    propertyTagType: {
                        name: 'ArrayProperty',
                        children: [{
                            name: 'StructProperty',
                            children: [{
                                name: 'InventoryStack',
                                children: [{ name: '/Script/FactoryGame', children: [] }]
                            }]
                        }]
                    },
                    values: eqStacks
                }
            }
        };
    };

    return {
        type: 'StructProperty',
        name: 'mInventoryToRespawnWith',
        propertyTagType: {
            name: 'StructProperty',
            children: [{
                name: 'InventoryToRespawnWith',
                children: [{ name: '/Script/FactoryGame', children: [] }]
            }]
        },
        value: {
            type: 'InventoryToRespawnWith',
            properties: {
                mInventoryStacks: {
                    type: 'ArrayProperty',
                    name: 'mInventoryStacks',
                    propertyTagType: {
                        name: 'ArrayProperty',
                        children: [{
                            name: 'StructProperty',
                            children: [{
                                name: 'InventoryStack',
                                children: [{ name: '/Script/FactoryGame', children: [] }]
                            }]
                        }]
                    },
                    values: emptyStacks
                },
                mEquipmentPairs: {
                    type: 'MapProperty',
                    name: 'mEquipmentPairs',
                    propertyTagType: {
                        name: 'MapProperty',
                        children: [
                            {
                                name: 'EnumProperty',
                                children: [
                                    { name: 'EEquipmentSlot', children: [{ name: '/Script/FactoryGame', children: [] }] },
                                    { name: 'ByteProperty', children: [] }
                                ]
                            },
                            {
                                name: 'StructProperty',
                                children: [
                                    { name: 'ArrayOfInventoryStacksHolder', children: [{ name: '/Script/FactoryGame', children: [] }] }
                                ]
                            }
                        ]
                    },
                    numElementsToRemove: 0,
                    values: [
                        ['EEquipmentSlot::ES_ARMS', createEquipmentHolder(4)],
                        ['EEquipmentSlot::ES_BACK', createEquipmentHolder(1)],
                        ['EEquipmentSlot::ES_LEGS', createEquipmentHolder(1)],
                        ['EEquipmentSlot::ES_HEAD', createEquipmentHolder(1)],
                        ['EEquipmentSlot::ES_BODY', createEquipmentHolder(1)]
                    ]
                }
            }
        }
    };
}

export function scanSaveForCrates(save) {
    if (!save || !save.levels) return [];
    const invMap = new Map();
    for (const lvl in save.levels) {
        for (const obj of save.levels[lvl].objects) {
            if (obj.typePath && obj.typePath.includes('FGInventoryComponent')) {
                invMap.set(obj.instanceName, obj);
            }
        }
    }

    const crates = [];
    for (const lvl in save.levels) {
        for (const obj of save.levels[lvl].objects) {
            if (obj.typePath && (obj.typePath.includes('BP_Crate') || (obj.typePath.includes('Crate') && !obj.typePath.includes('Crater')))) {
                const invCompName = `${obj.instanceName}.inventory`;
                const invObj = invMap.get(invCompName);
                const rawStacks = invObj?.properties?.mInventoryStacks?.values || [];
                const occupiedStacks = rawStacks.filter(s => s.properties?.Item?.value?.itemReference?.pathName && s.properties?.NumItems?.value > 0);
                
                const items = occupiedStacks.map(s => {
                    const path = s.properties.Item.value.itemReference.pathName;
                    const qty = s.properties.NumItems.value || 0;
                    return {
                        path,
                        name: friendlyItemName(path),
                        qty
                    };
                });

                const crateTypeVal = obj.properties?.mCrateType?.value?.value || '';
                const isDeathCrate = crateTypeVal.includes('DeathCrate');
                const isDismantleCrate = crateTypeVal.includes('DismantleCrate');

                crates.push({
                    instanceName: obj.instanceName,
                    levelName: lvl,
                    typePath: obj.typePath,
                    isDeathCrate,
                    isDismantleCrate,
                    crateType: isDeathCrate ? 'Death Crate' : (isDismantleCrate ? 'Dismantle Overflow' : 'Loot Crate'),
                    position: obj.transform?.translation || { x: 0, y: 0, z: 0 },
                    items,
                    totalItemsCount: items.reduce((acc, i) => acc + i.qty, 0),
                    stacksCount: items.length,
                    isEmpty: items.length === 0,
                    rawActor: obj,
                    rawInventory: invObj
                });
            }
        }
    }

    return crates;
}

export function renderAdminWorkspaceHTML({ 
    data, 
    activePioneerName, 
    unsavedChanges, 
    noticeMsg, 
    showRawEditor, 
    selectedRawObj, 
    rawCategory, 
    rawQuery, 
    rawFilteredObjects,
    careCategory = 'all',
    careSearchQuery = '',
    careSelectedItemPath = '',
    careQty = 50,
    researchTab = 'alternates',
    altCategory = 'all',
    altSearchQuery = '',
    expandedMAMTree = null,
    showCrateRadar = false
}) {
    const { header, pioneers, rawPlayerStates, totalClones, worldInfo } = data;
    const corruptPlayers = rawPlayerStates.filter(p => p.hasCorruptRespawn);

    // Crate Radar calculations
    const crates = scanSaveForCrates(data.rawSave);
    const deathCrates = crates.filter(c => c.isDeathCrate);
    const dismantleCrates = crates.filter(c => c.isDismantleCrate);
    const emptyCrates = crates.filter(c => c.isEmpty);

    // Research & MAM calculations
    const unlockedAltsCount = ALTERNATE_RECIPES.filter(a => isAlternateUnlocked(data.rawSave, a)).length;
    const totalAltsCount = ALTERNATE_RECIPES.length;
    const altsPct = Math.round((unlockedAltsCount / totalAltsCount) * 100);

    let totalMAMNodes = 0;
    let unlockedMAMNodes = 0;
    for (const tree of MAM_TREES) {
        totalMAMNodes += tree.nodes.length;
        unlockedMAMNodes += tree.nodes.filter(n => isMAMNodeUnlocked(data.rawSave, n)).length;
    }
    const mamPct = Math.round((unlockedMAMNodes / totalMAMNodes) * 100);

    const altCategories = Array.from(new Set(ALTERNATE_RECIPES.map(a => a.category))).sort();

    const filteredAlts = ALTERNATE_RECIPES.filter(a => {
        if (altCategory !== 'all' && a.category !== altCategory) return false;
        if (altSearchQuery) {
            const q = altSearchQuery.toLowerCase().trim();
            return a.name.toLowerCase().includes(q) || a.category.toLowerCase().includes(q);
        }
        return true;
    });

    const filteredAltsHTML = filteredAlts.map(alt => {
        const isUnlocked = isAlternateUnlocked(data.rawSave, alt);
        return `
        <div class="admin-recipe-card ${isUnlocked ? 'unlocked' : ''}">
            <div style="display:flex; flex-direction:column; gap:0.2rem; overflow:hidden;">
                <strong style="font-size:0.88rem; color:#fff; white-space:nowrap; overflow:hidden; text-overflow:ellipsis;">
                    ${isUnlocked ? '✅ ' : '🔒 '}${alt.name}
                </strong>
                <span style="font-size:0.72rem; color:var(--text-muted);">${alt.category}</span>
            </div>
            <button class="btn ${isUnlocked ? 'btn-secondary' : 'btn-primary'}" 
                    onclick="FicsitAdmin.toggleAlternate('${alt.id}')"
                    style="font-size:0.75rem; padding:0.3rem 0.7rem; white-space:nowrap; ${isUnlocked ? 'background:#065f46; color:#6ee7b7; border-color:#059669;' : ''}">
                ${isUnlocked ? 'Unlocked' : 'Unlock'}
            </button>
        </div>`;
    }).join('');

    const mamTreesHTML = MAM_TREES.map(tree => {
        const completedCount = tree.nodes.filter(n => isMAMNodeUnlocked(data.rawSave, n)).length;
        const treeTotal = tree.nodes.length;
        const isComplete = completedCount === treeTotal;
        const pct = Math.round((completedCount / treeTotal) * 100);
        const isExpanded = expandedMAMTree === tree.id;

        return `
        <div style="background:#0f141c; border:1px solid ${isComplete ? '#059669' : 'var(--border)'}; border-radius:8px; padding:1rem; display:flex; flex-direction:column; gap:0.75rem;">
            <div style="display:flex; justify-content:space-between; align-items:center;">
                <strong style="color:#fff; font-size:0.95rem;">${isComplete ? '🌟 ' : '🌲 '}${tree.name}</strong>
                <span style="font-size:0.78rem; font-family:monospace; color:${isComplete ? '#4ade80' : '#60a5fa'}; font-weight:700;">
                    ${completedCount} / ${treeTotal} (${pct}%)
                </span>
            </div>

            <div style="height:6px; background:#1e293b; border-radius:3px; overflow:hidden;">
                <div style="width:${pct}%; height:100%; background:${isComplete ? '#10b981' : '#3b82f6'}; transition:width 0.3s;"></div>
            </div>

            <div style="display:flex; justify-content:space-between; align-items:center; gap:0.5rem; margin-top:0.25rem;">
                <button class="btn btn-secondary" onclick="FicsitAdmin.toggleMAMTreeExpand('${tree.id}')" style="font-size:0.75rem; padding:0.3rem 0.6rem;">
                    ${isExpanded ? '▲ Hide Nodes' : `▼ View Nodes (${treeTotal})`}
                </button>
                ${!isComplete ? `
                <button class="btn btn-primary" onclick="FicsitAdmin.completeMAMTree('${tree.id}')" style="font-size:0.75rem; padding:0.3rem 0.65rem;">
                    ⚡ Complete Tree
                </button>` : `<span style="font-size:0.75rem; color:#4ade80; font-weight:600;">✓ All Researched</span>`}
            </div>

            ${isExpanded ? `
            <div style="display:flex; flex-direction:column; gap:0.35rem; margin-top:0.5rem; padding-top:0.5rem; border-top:1px dashed var(--border); max-height:220px; overflow-y:auto;">
                ${tree.nodes.map(n => {
                    const nodeDone = isMAMNodeUnlocked(data.rawSave, n);
                    return `
                    <div style="display:flex; justify-content:space-between; align-items:center; padding:0.3rem 0.5rem; background:#131a24; border-radius:4px; font-size:0.78rem;">
                        <span style="color:${nodeDone ? '#a7f3d0' : '#cbd5e1'};">${nodeDone ? '✓' : '○'} ${n.name}</span>
                        <button class="btn ${nodeDone ? 'btn-secondary' : 'btn-primary'}" 
                                onclick="FicsitAdmin.toggleMAMNode('${n.schematicPath}')" 
                                style="font-size:0.7rem; padding:0.15rem 0.5rem; ${nodeDone ? 'background:#065f46; color:#6ee7b7;' : ''}">
                            ${nodeDone ? 'Done' : 'Unlock'}
                        </button>
                    </div>`;
                }).join('')}
            </div>` : ''}
        </div>`;
    }).join('');

    const activePioneer = pioneers.find(p => p.name === activePioneerName) || pioneers[0];
    const { stacks, isRespawnTable } = findPioneerInventory(activePioneer, data.rawSave);

    let occupiedCount = 0;
    let totalSlots = 0;
    let slotCards = '';

    if (stacks) {
        totalSlots = stacks.length;
        occupiedCount = stacks.filter(s => s.properties?.Item?.value?.itemReference?.pathName && s.properties?.NumItems?.value > 0).length;

        slotCards = stacks.map((slot, index) => {
            const itemPath = slot.properties?.Item?.value?.itemReference?.pathName;
            const qty = slot.properties?.NumItems?.value || 0;
            const isOccupied = itemPath && qty > 0;
            const itemName = friendlyItemName(itemPath);

            if (!isOccupied) {
                return `
                <div class="admin-inv-slot empty" onclick="FicsitAdmin.promptAddToSlot('${activePioneer?.name}', ${index})" title="Empty Slot #${index + 1} - Click to drop care package">
                    <span style="font-size:0.75rem; color:var(--text-muted); font-weight:500;">+ Empty</span>
                    <span style="font-size:0.65rem; color:#475569; position:absolute; bottom:4px; right:6px;">#${index + 1}</span>
                </div>`;
            }

            return `
            <div class="admin-inv-slot occupied" title="${itemName} (${itemPath})">
                <button class="admin-inv-slot-del" onclick="FicsitAdmin.clearSlot('${activePioneer?.name}', ${index})" title="Empty this slot">&times;</button>
                <div class="admin-inv-slot-name">${itemName}</div>
                <div style="display:flex; justify-content:space-between; align-items:flex-end; width:100%;">
                    <span style="font-size:0.65rem; color:#475569;">#${index + 1}</span>
                    <span class="admin-inv-slot-qty" onclick="FicsitAdmin.editSlotQty('${activePioneer?.name}', ${index}, ${qty})" title="Click to edit quantity">x${qty.toLocaleString()}</span>
                </div>
            </div>`;
        }).join('');
    }

    const noticeBanner = noticeMsg ? `
        <div style="margin-bottom:1.25rem; padding:0.85rem 1.25rem; border-radius:8px; background:rgba(34,197,94,0.15); border:1px solid rgba(34,197,94,0.4); color:#4ade80; display:flex; justify-content:space-between; align-items:center;">
            <div><strong>Success:</strong> ${noticeMsg}</div>
            <button style="background:transparent; border:none; color:#4ade80; cursor:pointer; font-size:1.2rem;" onclick="FicsitAdmin.dismissNotice()">&times;</button>
        </div>` : '';

    const percentFull = totalSlots > 0 ? Math.round((occupiedCount / totalSlots) * 100) : 0;

    const availableItems = getAllAvailableItems(data.rawSave);
    const categories = Array.from(new Set(availableItems.map(i => i.category))).sort();

    const effectiveCategory = careCategory || 'all';
    const effectiveSearch = (careSearchQuery || '').toLowerCase().trim();

    const filteredCareItems = availableItems.filter(item => {
        if (effectiveCategory !== 'all' && item.category !== effectiveCategory) return false;
        if (effectiveSearch) {
            const matchesName = item.name.toLowerCase().includes(effectiveSearch);
            const matchesPath = item.path.toLowerCase().includes(effectiveSearch);
            const matchesCat = item.category.toLowerCase().includes(effectiveSearch);
            if (!matchesName && !matchesPath && !matchesCat) return false;
        }
        return true;
    });

    const careGrouped = {};
    for (const item of filteredCareItems) {
        if (!careGrouped[item.category]) careGrouped[item.category] = [];
        careGrouped[item.category].push(item);
    }

    let careOptionsHTML = '';
    const selectedItemPath = careSelectedItemPath || (filteredCareItems[0] ? filteredCareItems[0].path : '');
    for (const [catName, catItems] of Object.entries(careGrouped)) {
        careOptionsHTML += `<optgroup label="${catName}">`;
        for (const item of catItems) {
            const isSel = item.path === selectedItemPath;
            careOptionsHTML += `<option value="${item.path}" data-qty="${item.defaultQty}" ${isSel ? 'selected' : ''}>${item.icon} ${item.name} (${item.defaultQty}x)</option>`;
        }
        careOptionsHTML += `</optgroup>`;
    }
    if (filteredCareItems.length === 0) {
        careOptionsHTML = `<option disabled value="">No items match "${careSearchQuery}"</option>`;
    }

    return `
    <div style="display:flex; flex-direction:column; gap:1.25rem;">
        ${noticeBanner}

        <!-- 1. Top Health Doctor Alert Cards -->
        <div style="display:grid; grid-template-columns:repeat(auto-fit, minmax(360px, 1fr)); gap:1rem;">
            <!-- Corrupt Respawns -->
            <div style="background:#131a24; border:1px solid ${corruptPlayers.length > 0 ? '#ef4444' : 'var(--border)'}; border-radius:8px; padding:1.2rem; display:flex; justify-content:space-between; align-items:center; flex-wrap:wrap; gap:0.75rem;">
                <div>
                    <div style="display:flex; align-items:center; gap:0.5rem;">
                        <span style="font-size:1.25rem;">${corruptPlayers.length > 0 ? '⚠️' : '✅'}</span>
                        <strong style="color:${corruptPlayers.length > 0 ? '#ef4444' : '#4ade80'}; font-size:0.95rem;">Respawn Crash Watchdog</strong>
                    </div>
                    <div style="font-size:0.82rem; color:var(--text-muted); margin-top:0.35rem;">
                        ${corruptPlayers.length > 0 
                            ? `<span style="color:#f87171;">Found ${corruptPlayers.length} player(s) (${corruptPlayers.map(p => p.name).join(', ')}) with corrupted respawn tables!</span>` 
                            : 'All player respawn tables are healthy. No connection-crash bugs detected.'}
                    </div>
                </div>
                ${corruptPlayers.length > 0 ? `
                    <button class="btn btn-primary" onclick="FicsitAdmin.fixCorruptRespawns()" style="background:#ef4444; border-color:#dc2626; padding:0.5rem 1rem;">
                        🩺 Fix Corrupt Respawns
                    </button>` : ''}
            </div>

            <!-- Ghost Clones -->
            <div style="background:#131a24; border:1px solid ${totalClones > 0 ? '#c084fc' : 'var(--border)'}; border-radius:8px; padding:1.2rem; display:flex; justify-content:space-between; align-items:center; flex-wrap:wrap; gap:0.75rem;">
                <div>
                    <div style="display:flex; align-items:center; gap:0.5rem;">
                        <span style="font-size:1.25rem;">${totalClones > 0 ? '👻' : '✅'}</span>
                        <strong style="color:${totalClones > 0 ? '#c084fc' : '#4ade80'}; font-size:0.95rem;">Ghost Clone Watchdog</strong>
                    </div>
                    <div style="font-size:0.82rem; color:var(--text-muted); margin-top:0.35rem;">
                        ${totalClones > 0 
                            ? `<span style="color:#e9d5ff;">Found ${totalClones} dormant pioneer clone bodies sitting in world from earlier sessions.</span>` 
                            : 'Player roster is clean. Zero dormant ghost clones in world.'}
                    </div>
                </div>
                ${totalClones > 0 ? `
                    <button class="btn btn-primary" onclick="FicsitAdmin.purgeGhostClones()" style="background:#9333ea; border-color:#7e22ce; padding:0.5rem 1rem;">
                        🧹 Purge ${totalClones} Clone(s)
                    </button>` : ''}
            </div>

            <!-- Crate & Loot Radar -->
            <div style="background:#131a24; border:1px solid ${crates.length > 0 ? (deathCrates.length > 0 ? '#ef4444' : '#eab308') : 'var(--border)'}; border-radius:8px; padding:1.2rem; display:flex; justify-content:space-between; align-items:center; flex-wrap:wrap; gap:0.75rem;">
                <div>
                    <div style="display:flex; align-items:center; gap:0.5rem;">
                        <span style="font-size:1.25rem;">${crates.length > 0 ? (deathCrates.length > 0 ? '💀' : '📦') : '✅'}</span>
                        <strong style="color:${crates.length > 0 ? (deathCrates.length > 0 ? '#ef4444' : '#facc15') : '#4ade80'}; font-size:0.95rem;">
                            Crate &amp; Loot Radar
                        </strong>
                    </div>
                    <div style="font-size:0.82rem; color:var(--text-muted); margin-top:0.35rem;">
                        ${crates.length > 0 
                            ? `<span style="color:#fde047;">Found ${crates.length} dropped crate(s) in world (${deathCrates.length} death crate(s), ${dismantleCrates.length} dismantle overflow).</span>` 
                            : 'World is clean. Zero lost death crates or dismantle boxes in world.'}
                    </div>
                </div>
                ${crates.length > 0 ? `
                    <div style="display:flex; gap:0.5rem; align-items:center;">
                        <button class="btn btn-secondary" onclick="FicsitAdmin.toggleCrateRadar()" style="font-size:0.82rem;">
                            ${showCrateRadar ? '▲ Hide Radar' : `📡 View Radar (${crates.length})`}
                        </button>
                        <button class="btn btn-primary" onclick="FicsitAdmin.teleportAllCratesToHub()" style="font-size:0.82rem;">
                            🏠 Teleport All to HUB
                        </button>
                    </div>` : ''}
            </div>
        </div>

        <!-- Crate Radar Interactive Panel -->
        ${crates.length > 0 && showCrateRadar ? `
        <div style="background:#131a24; border:1px solid ${deathCrates.length > 0 ? 'rgba(239,68,68,0.5)' : 'var(--border)'}; border-radius:8px; padding:1.25rem; display:flex; flex-direction:column; gap:1rem;">
            <div style="display:flex; justify-content:space-between; align-items:center; flex-wrap:wrap; gap:0.75rem; border-bottom:1px solid var(--border); padding-bottom:0.75rem;">
                <div style="display:flex; align-items:center; gap:0.5rem;">
                    <span style="font-size:1.1rem;">📡</span>
                    <strong style="color:#fff; font-size:1rem;">Active World Crates (${crates.length})</strong>
                    <span style="font-size:0.75rem; color:var(--text-muted);">
                        Total Items: ${crates.reduce((acc, c) => acc + c.totalItemsCount, 0).toLocaleString()}
                    </span>
                </div>
                <div style="display:flex; gap:0.5rem; flex-wrap:wrap; align-items:center;">
                    <button class="btn btn-primary" onclick="FicsitAdmin.teleportAllCratesToHub()" style="font-size:0.82rem;">
                        🏠 Teleport All Crates to HUB
                    </button>
                    <button class="btn btn-secondary" onclick="FicsitAdmin.claimAllCrates('${activePioneer?.name}')" style="font-size:0.82rem;">
                        🎒 Claim All into ${activePioneer?.name}'s Backpack
                    </button>
                    ${emptyCrates.length > 0 ? `
                    <button class="btn btn-secondary" onclick="FicsitAdmin.purgeEmptyCrates()" style="font-size:0.82rem; color:#f87171;">
                        🧹 Purge ${emptyCrates.length} Empty Crate(s)
                    </button>` : ''}
                </div>
            </div>

            <div style="display:grid; grid-template-columns:repeat(auto-fill, minmax(340px, 1fr)); gap:0.85rem;">
                ${crates.map(c => `
                <div style="background:#0f141c; border:1px solid ${c.isDeathCrate ? 'rgba(239,68,68,0.4)' : 'var(--border)'}; border-radius:6px; padding:0.85rem; display:flex; flex-direction:column; gap:0.65rem;">
                    <div style="display:flex; justify-content:space-between; align-items:flex-start;">
                        <div>
                            <div style="display:flex; align-items:center; gap:0.4rem;">
                                <span style="font-size:0.8rem; background:${c.isDeathCrate ? 'rgba(239,68,68,0.2)' : 'rgba(234,179,8,0.2)'}; color:${c.isDeathCrate ? '#f87171' : '#facc15'}; border:1px solid ${c.isDeathCrate ? 'rgba(239,68,68,0.4)' : 'rgba(234,179,8,0.4)'}; padding:1px 6px; border-radius:4px; font-weight:700;">
                                    ${c.isDeathCrate ? '💀 Death Crate' : '📦 Dismantle Overflow'}
                                </span>
                                <span style="font-size:0.75rem; color:var(--text-muted); font-family:monospace;">
                                    ${c.stacksCount} stack(s) &bull; ${c.totalItemsCount.toLocaleString()} item(s)
                                </span>
                            </div>
                            <div style="font-size:0.72rem; color:var(--text-muted); margin-top:0.35rem; font-family:monospace;">
                                Pos: (${Math.round(c.position.x)}, ${Math.round(c.position.y)}, ${Math.round(c.position.z)})
                            </div>
                        </div>
                        <button class="btn btn-secondary" onclick="FicsitAdmin.deleteCrate('${c.instanceName}')" style="color:#ef4444; border-color:rgba(239,68,68,0.4); padding:2px 7px; font-size:0.75rem;" title="Delete this crate from the world">&times; Delete</button>
                    </div>

                    <!-- Items Preview -->
                    <div style="background:#131a24; border-radius:4px; padding:0.5rem; max-height:110px; overflow-y:auto; display:flex; flex-direction:column; gap:0.25rem;">
                        ${c.items.length === 0 ? '<span style="font-size:0.75rem; color:var(--text-muted);">Empty crate (0 items)</span>' : ''}
                        ${c.items.map(item => `
                        <div style="display:flex; justify-content:space-between; align-items:center; font-size:0.76rem;">
                            <span style="color:#e2e8f0;">${item.name}</span>
                            <span style="color:#a7f3d0; font-family:monospace; font-weight:700;">x${item.qty.toLocaleString()}</span>
                        </div>`).join('')}
                    </div>

                    <!-- Crate Action Buttons -->
                    <div style="display:flex; gap:0.4rem; justify-content:flex-end;">
                        <button class="btn btn-secondary" onclick="FicsitAdmin.teleportCrate('${c.instanceName}')" style="font-size:0.75rem; padding:0.3rem 0.65rem;">
                            🏠 Teleport to HUB
                        </button>
                        <button class="btn btn-primary" onclick="FicsitAdmin.claimCrate('${c.instanceName}', '${activePioneer?.name}')" style="font-size:0.75rem; padding:0.3rem 0.65rem;">
                            🎒 Claim into Backpack
                        </button>
                    </div>
                </div>`).join('')}
            </div>
        </div>` : ''}

        <!-- 2. Pioneer Operations & Visual Inventory Workspace -->
        <div style="display:grid; grid-template-columns:280px 1fr; gap:1.25rem; align-items:start;" class="admin-pioneer-workspace">
            <!-- Left: Pioneer List -->
            <div style="background:#131a24; border:1px solid var(--border); border-radius:8px; padding:1rem; display:flex; flex-direction:column; gap:0.5rem;">
                <div style="font-size:0.85rem; font-weight:700; color:var(--text-muted); text-transform:uppercase; letter-spacing:0.05em; margin-bottom:0.35rem;">
                    Pioneers Roster (${pioneers.length})
                </div>
                ${pioneers.map(p => {
                    const isSelected = p.name === activePioneer?.name;
                    const statusIcon = p.primaryState?.hasPawn ? '🟢' : '⚪';
                    const slotCount = p.primaryState?.inventorySlots || 0;
                    return `
                    <div onclick="FicsitAdmin.selectPioneer('${p.name}')" 
                         style="background:${isSelected ? '#1e293b' : '#0f141c'}; border:1px solid ${isSelected ? 'var(--accent-primary)' : 'var(--border)'}; border-radius:6px; padding:0.75rem; cursor:pointer; transition:all 0.15s;">
                        <div style="display:flex; justify-content:space-between; align-items:center;">
                            <strong style="color:#fff; font-size:0.92rem;">${statusIcon} ${p.name}</strong>
                            <span style="font-size:0.75rem; color:#a7f3d0; font-family:monospace; font-weight:600;">${slotCount} slots</span>
                        </div>
                        <div style="display:flex; justify-content:space-between; align-items:center; margin-top:0.35rem; font-size:0.75rem; color:var(--text-muted);">
                            <span>${p.platform}</span>
                            ${p.cloneCount > 0 ? `<span style="color:#c084fc; font-weight:600;">👻 ${p.cloneCount} clone(s)</span>` : '<span style="color:#4ade80;">Active</span>'}
                        </div>
                    </div>`;
                }).join('')}
            </div>

            <!-- Right: Interactive Pioneer Workspace -->
            <div style="background:#131a24; border:1px solid var(--border); border-radius:8px; padding:1.25rem; display:flex; flex-direction:column; gap:1rem;">
                <!-- Pioneer Toolbar -->
                <div style="display:flex; justify-content:space-between; align-items:center; flex-wrap:wrap; gap:0.75rem; border-bottom:1px solid var(--border); padding-bottom:1rem;">
                    <div>
                        <h3 style="margin:0; font-size:1.15rem; color:#fff; display:flex; align-items:center; gap:0.5rem;">
                            <span>👷 ${activePioneer?.name}</span>
                            <span style="font-size:0.75rem; background:${activePioneer?.primaryState?.hasPawn ? 'rgba(34,197,94,0.15)' : (isRespawnTable ? 'rgba(59,130,246,0.15)' : 'rgba(148,163,184,0.15)')}; color:${activePioneer?.primaryState?.hasPawn ? '#4ade80' : (isRespawnTable ? '#60a5fa' : '#94a3b8')}; border:1px solid ${activePioneer?.primaryState?.hasPawn ? 'rgba(34,197,94,0.3)' : (isRespawnTable ? 'rgba(59,130,246,0.3)' : 'rgba(148,163,184,0.3)')}; padding:2px 8px; border-radius:10px;">
                                ${activePioneer?.primaryState?.hasPawn ? 'Active In-World' : (isRespawnTable ? 'Offline (Respawn Table)' : 'Offline')}
                            </span>
                        </h3>
                        <div style="font-size:0.8rem; color:var(--text-muted); margin-top:0.25rem;">
                            Position: ${activePioneer?.primaryState?.locationStr || 'Unknown'} &bull; Platform: ${activePioneer?.platform}
                        </div>
                    </div>

                    <div style="display:flex; gap:0.5rem; flex-wrap:wrap; align-items:center;">
                        <button class="btn btn-secondary" onclick="FicsitAdmin.teleportPioneer('${activePioneer?.name}')" style="font-size:0.82rem;">
                            🏠 Teleport to HUB
                        </button>
                        <div style="display:inline-flex; align-items:center; gap:0.35rem;">
                            <input type="number" id="admin-pioneer-slots-input" value="${totalSlots || activePioneer?.primaryState?.inventorySlots || 68}" min="10" max="500" 
                                   style="width:70px; padding:0.4rem; background:#0f141c; color:#fff; border:1px solid var(--border); border-radius:6px; font-size:0.82rem;">
                            <button class="btn btn-secondary" onclick="FicsitAdmin.applySlots('${activePioneer?.name}')" style="font-size:0.82rem;">Set Capacity</button>
                        </div>
                    </div>
                </div>

                <!-- Care Package Injector Bar -->
                <div style="background:#0f141c; border:1px dashed var(--border); border-radius:6px; padding:0.85rem; display:flex; flex-direction:column; gap:0.65rem;">
                    <div style="display:flex; justify-content:space-between; align-items:center; flex-wrap:wrap; gap:0.5rem;">
                        <div style="display:flex; align-items:center; gap:0.5rem;">
                            <strong style="font-size:0.88rem; color:#fff; display:flex; align-items:center; gap:0.35rem;">
                                🎁 Care Package Injector
                            </strong>
                            <span style="font-size:0.75rem; color:var(--text-muted); background:rgba(255,255,255,0.06); padding:0.15rem 0.5rem; border-radius:999px;" id="admin-care-count">
                                ${filteredCareItems.length} of ${availableItems.length} items cataloged
                            </span>
                        </div>
                        <div style="font-size:0.75rem; color:var(--text-muted);">
                            Search &amp; select from all Satisfactory 1.0 items or discovered mod items
                        </div>
                    </div>

                    <div style="display:flex; align-items:center; gap:0.6rem; flex-wrap:wrap;">
                        <select id="admin-care-cat" onchange="FicsitAdmin.changeCareCategory(this.value)"
                                style="padding:0.45rem 0.6rem; background:#131a24; color:#fff; border:1px solid var(--border); border-radius:6px; font-size:0.82rem; min-width:170px;">
                            <option value="all" ${effectiveCategory === 'all' ? 'selected' : ''}>📂 All Categories (${availableItems.length})</option>
                            ${categories.map(cat => `<option value="${cat}" ${effectiveCategory === cat ? 'selected' : ''}>${cat}</option>`).join('')}
                        </select>

                        <div style="position:relative; flex:1; min-width:160px;">
                            <input type="text" id="admin-care-search" placeholder="🔍 Search item name..." value="${careSearchQuery || ''}"
                                   oninput="FicsitAdmin.filterCareItems(this.value)"
                                   style="width:100%; box-sizing:border-box; padding:0.45rem 0.6rem; background:#131a24; color:#fff; border:1px solid var(--border); border-radius:6px; font-size:0.82rem;">
                        </div>

                        <select id="admin-care-select" onchange="FicsitAdmin.changeCarePreset(this.value)"
                                style="padding:0.45rem 0.6rem; background:#131a24; color:#fff; border:1px solid var(--border); border-radius:6px; font-size:0.82rem; flex:1.5; min-width:240px;">
                            ${careOptionsHTML}
                        </select>

                        <div style="display:inline-flex; align-items:center; gap:0.3rem;">
                            <label for="admin-care-qty" style="font-size:0.75rem; color:var(--text-muted);">Qty:</label>
                            <input type="number" id="admin-care-qty" value="${careQty || 50}" min="1" max="50000" 
                                   style="width:75px; padding:0.45rem; background:#131a24; color:#fff; border:1px solid var(--border); border-radius:6px; font-size:0.82rem;">
                        </div>

                        <button class="btn btn-primary" onclick="FicsitAdmin.grantCarePackage('${activePioneer?.name}')" style="font-size:0.82rem; padding:0.45rem 0.9rem; white-space:nowrap;">
                            🎁 Add to Backpack
                        </button>
                    </div>
                </div>

                <!-- Visual Inventory Section -->
                ${!stacks ? `
                <div style="background:#0f141c; border:1px dashed #475569; border-radius:8px; padding:2.5rem 1.5rem; text-align:center; display:flex; flex-direction:column; align-items:center; gap:0.85rem;">
                    <div style="font-size:2.4rem;">💤</div>
                    <div>
                        <strong style="font-size:1.05rem; color:#fff; display:block; margin-bottom:0.35rem;">Pioneer is Offline (No Physical Body in World)</strong>
                        <div style="font-size:0.82rem; color:var(--text-muted); max-width:480px; line-height:1.45;">
                            ${activePioneer?.name} is currently disconnected and has no active pawn or saved respawn backpack in this save. Provision a respawn backpack to grant items or adjust capacity while they are offline.
                        </div>
                    </div>
                    <button class="btn btn-primary" onclick="FicsitAdmin.provisionBackpack('${activePioneer?.name}', ${activePioneer?.primaryState?.inventorySlots || 63})" style="padding:0.55rem 1.25rem; font-size:0.88rem; margin-top:0.35rem;">
                        🎒 Initialize Respawn Backpack (${activePioneer?.primaryState?.inventorySlots || 63} Slots)
                    </button>
                </div>
                ` : `
                <!-- Visual Inventory Occupancy Bar -->
                <div>
                    <div style="display:flex; justify-content:space-between; align-items:center; margin-bottom:0.35rem; font-size:0.82rem;">
                        <span style="color:var(--text-muted);">
                            <strong>${isRespawnTable ? '📦 Offline Respawn Inventory' : '🎒 Backpack Occupancy'}:</strong> ${occupiedCount} of ${totalSlots} slots occupied (${totalSlots - occupiedCount} free)
                        </span>
                        <span style="font-weight:600; color:${percentFull >= 95 ? '#ef4444' : '#a7f3d0'};">${percentFull}% Full</span>
                    </div>
                    <div style="height:6px; background:#0f141c; border-radius:3px; overflow:hidden;">
                        <div style="width:${percentFull}%; height:100%; background:${percentFull >= 95 ? '#ef4444' : 'var(--accent-primary)'}; transition:width 0.3s;"></div>
                    </div>
                </div>

                <!-- Visual Inventory Grid -->
                <div>
                    <div style="font-size:0.8rem; color:var(--text-muted); margin-bottom:0.5rem; display:flex; justify-content:space-between;">
                        <span>Click quantity to edit &bull; Hover slot to empty</span>
                        <span>Click empty slot to drop selected care package</span>
                    </div>
                    <div class="admin-inv-grid">
                        ${slotCards}
                    </div>
                </div>
                `}
            </div>
        </div>

        <!-- 3. MAM & Alternate Recipe Research Station -->
        <div style="background:#131a24; border:1px solid var(--border); border-radius:8px; padding:1.25rem; display:flex; flex-direction:column; gap:1rem;">
            <!-- Header -->
            <div style="display:flex; justify-content:space-between; align-items:center; flex-wrap:wrap; gap:0.75rem; border-bottom:1px solid var(--border); padding-bottom:1rem;">
                <div>
                    <h3 style="margin:0; font-size:1.15rem; color:#fff; display:flex; align-items:center; gap:0.5rem;">
                        <span>🔬 MAM &amp; Alternate Recipe Research Station</span>
                    </h3>
                    <div style="display:flex; align-items:center; gap:0.75rem; margin-top:0.4rem; font-size:0.82rem;">
                        <span style="background:rgba(250,128,41,0.15); color:var(--accent-primary); border:1px solid rgba(250,128,41,0.3); padding:2px 8px; border-radius:12px; font-weight:600;">
                            ⚡ ${unlockedAltsCount} / ${totalAltsCount} Alternates (${altsPct}%)
                        </span>
                        <span style="background:rgba(59,130,246,0.15); color:#60a5fa; border:1px solid rgba(59,130,246,0.3); padding:2px 8px; border-radius:12px; font-weight:600;">
                            🌲 ${unlockedMAMNodes} / ${totalMAMNodes} MAM Nodes (${mamPct}%)
                        </span>
                    </div>
                </div>

                <!-- Batch Actions -->
                <div style="display:flex; gap:0.5rem; flex-wrap:wrap; align-items:center;">
                    <button class="btn btn-primary" onclick="FicsitAdmin.unlockAllAlternates()" style="font-size:0.82rem;">
                        ⚡ Unlock All Alternates
                    </button>
                    <button class="btn btn-secondary" onclick="FicsitAdmin.completeAllMAMTrees()" style="font-size:0.82rem;">
                        🌲 Complete All MAM Trees
                    </button>
                    <button class="btn btn-secondary" onclick="FicsitAdmin.lockAllAlternates()" style="font-size:0.82rem; color:#f87171;">
                        🔒 Lock All Alternates
                    </button>
                </div>
            </div>

            <!-- Sub Tabs: Alternates vs MAM Trees -->
            <div style="display:flex; gap:0.5rem; align-items:center;">
                <button class="admin-tab-btn ${researchTab === 'alternates' ? 'active' : ''}" onclick="FicsitAdmin.setResearchTab('alternates')">
                    📜 Alternate Recipes (${totalAltsCount})
                </button>
                <button class="admin-tab-btn ${researchTab === 'mam' ? 'active' : ''}" onclick="FicsitAdmin.setResearchTab('mam')">
                    🌲 MAM Research Trees (${MAM_TREES.length})
                </button>
            </div>

            ${researchTab === 'alternates' ? `
                <!-- Alternate Recipes Toolbar: Search & Category Filter -->
                <div style="display:flex; gap:0.75rem; align-items:center; flex-wrap:wrap;">
                    <select id="admin-alt-cat" onchange="FicsitAdmin.filterAltCategory(this.value)"
                            style="padding:0.45rem 0.6rem; background:#0f141c; color:#fff; border:1px solid var(--border); border-radius:6px; font-size:0.82rem; min-width:180px;">
                        <option value="all" ${altCategory === 'all' ? 'selected' : ''}>📂 All Categories (${totalAltsCount})</option>
                        ${altCategories.map(cat => `<option value="${cat}" ${altCategory === cat ? 'selected' : ''}>${cat}</option>`).join('')}
                    </select>

                    <div style="position:relative; flex:1; min-width:200px;">
                        <input type="text" id="admin-alt-search" placeholder="🔍 Search alternate recipes (e.g. Cast Screw, Heavy Oil Residue)..." 
                               value="${altSearchQuery || ''}"
                               oninput="FicsitAdmin.filterAltSearch(this.value)"
                               style="width:100%; box-sizing:border-box; padding:0.45rem 0.6rem; background:#0f141c; color:#fff; border:1px solid var(--border); border-radius:6px; font-size:0.82rem;">
                    </div>
                </div>

                <!-- Alternate Recipes Grid -->
                <div class="admin-recipe-grid">
                    ${filteredAltsHTML}
                </div>
            ` : `
                <!-- MAM Trees View -->
                <div style="display:grid; grid-template-columns:repeat(auto-fill, minmax(320px, 1fr)); gap:1rem;">
                    ${mamTreesHTML}
                </div>
            `}
        </div>

        <!-- 4. World & Progression Controls -->
        <div style="background:#131a24; border:1px solid var(--border); border-radius:8px; padding:1.25rem;">
            <h3 style="margin:0 0 0.75rem 0; font-size:1rem; color:var(--accent-primary);">🗺️ World &amp; Project Assembly Controls</h3>
            <div style="display:grid; grid-template-columns:repeat(auto-fit, minmax(280px, 1fr)); gap:1rem; align-items:end;">
                <div>
                    <label style="font-size:0.8rem; color:var(--text-muted); display:block; margin-bottom:0.35rem;">
                        Space Elevator Progression Phase:
                    </label>
                    <div style="display:flex; gap:0.5rem;">
                        <select id="admin-phase-select" style="flex:1; padding:0.5rem; background:#0f141c; color:#fff; border:1px solid var(--border); border-radius:6px; font-size:0.85rem;">
                            <option value="1">Phase 1: Launch Project Assembly</option>
                            <option value="2">Phase 2: Platform Expansion</option>
                            <option value="3" ${worldInfo.targetGamePhase?.includes('3') ? 'selected' : ''}>Phase 3: Automated Production</option>
                            <option value="4" ${worldInfo.targetGamePhase?.includes('4') ? 'selected' : ''}>Phase 4: Propulsion Systems</option>
                            <option value="5" ${worldInfo.targetGamePhase?.includes('5') ? 'selected' : ''}>Phase 5: Final Delivery</option>
                        </select>
                        <button class="btn btn-secondary" onclick="FicsitAdmin.applyGamePhase()">Apply Phase</button>
                    </div>
                </div>

                <div>
                    <label style="font-size:0.8rem; color:var(--text-muted); display:block; margin-bottom:0.35rem;">
                        World Session Name:
                    </label>
                    <div style="display:flex; gap:0.5rem;">
                        <input type="text" id="admin-session-name-input" value="${header.sessionName}" 
                               style="flex:1; padding:0.5rem; background:#0f141c; color:#fff; border:1px solid var(--border); border-radius:6px; font-size:0.85rem;">
                        <button class="btn btn-secondary" onclick="FicsitAdmin.applySessionName()">Rename</button>
                    </div>
                </div>
            </div>
        </div>

        <!-- 4. Advanced World Object & Property Browser (Collapsible) -->
        <div style="background:#131a24; border:1px solid var(--border); border-radius:8px; padding:1.25rem;">
            <div style="display:flex; justify-content:space-between; align-items:center; cursor:pointer;" onclick="FicsitAdmin.toggleRawDrawer()">
                <div>
                    <h3 style="margin:0; font-size:1rem; color:var(--text-main);">🔍 Advanced World Object &amp; Property Inspector</h3>
                    <div style="font-size:0.8rem; color:var(--text-muted); margin-top:0.2rem;">
                        Browse production machines, power grids, logistics, and subsystems by friendly categories.
                    </div>
                </div>
                <button class="btn btn-secondary" style="font-size:0.8rem;">
                    ${showRawEditor ? '▲ Hide Advanced Inspector' : '▼ Open Advanced Inspector'}
                </button>
            </div>

            ${showRawEditor ? `
                <div style="margin-top:1.25rem; border-top:1px solid var(--border); padding-top:1rem;">
                    <div style="display:grid; grid-template-columns:repeat(auto-fit, minmax(260px, 1fr)); gap:0.75rem; margin-bottom:0.85rem;">
                        <div>
                            <label style="font-size:0.8rem; color:var(--text-muted); display:block; margin-bottom:0.35rem;">Category:</label>
                            <select onchange="FicsitAdmin.changeRawCategory(this.value)" 
                                    style="width:100%; padding:0.5rem; background:#0f141c; color:#fff; border:1px solid var(--border); border-radius:6px; font-size:0.85rem;">
                                <option value="subsystems" ${rawCategory === 'subsystems' ? 'selected' : ''}>⚙️ Subsystems &amp; Managers</option>
                                <option value="production" ${rawCategory === 'production' ? 'selected' : ''}>🏭 Production &amp; Machines</option>
                                <option value="power" ${rawCategory === 'power' ? 'selected' : ''}>⚡ Power &amp; Energy</option>
                                <option value="storage" ${rawCategory === 'storage' ? 'selected' : ''}>📦 Storage &amp; Containers</option>
                                <option value="pioneers" ${rawCategory === 'pioneers' ? 'selected' : ''}>👷 Pioneers</option>
                                <option value="logistics" ${rawCategory === 'logistics' ? 'selected' : ''}>🚂 Logistics &amp; Vehicles</option>
                                <option value="modded" ${rawCategory === 'modded' ? 'selected' : ''}>🧩 Modded Actors</option>
                                <option value="all" ${rawCategory === 'all' ? 'selected' : ''}>🌐 All World Objects</option>
                            </select>
                        </div>
                        <div>
                            <label style="font-size:0.8rem; color:var(--text-muted); display:block; margin-bottom:0.35rem;">Filter:</label>
                            <input type="text" id="admin-raw-search-input" value="${rawQuery}" placeholder="Filter objects..."
                                   oninput="FicsitAdmin.changeRawQuery(this.value)"
                                   style="width:100%; padding:0.5rem; background:#0f141c; color:#fff; border:1px solid var(--border); border-radius:6px; font-size:0.85rem; box-sizing:border-box;">
                        </div>
                    </div>

                    <div style="margin-bottom:0.75rem;">
                        <label style="font-size:0.8rem; color:var(--text-muted); display:block; margin-bottom:0.35rem;">
                            Select Object (${rawFilteredObjects?.length || 0} found):
                        </label>
                        <select onchange="FicsitAdmin.selectRawObject(this.value)" 
                                style="width:100%; padding:0.55rem; background:#0f141c; color:#fff; border:1px solid var(--border); border-radius:6px; font-size:0.85rem;">
                            ${(rawFilteredObjects || []).map(o => `<option value="${o.instanceName}" ${selectedRawObj?.instanceName === o.instanceName ? 'selected' : ''}>${o.friendlyName || o.instanceName}</option>`).join('')}
                        </select>
                    </div>

                    ${selectedRawObj ? `
                        <div style="margin-top:1rem;">
                            <div style="display:flex; justify-content:space-between; align-items:center; margin-bottom:0.4rem; flex-wrap:wrap; gap:0.5rem;">
                                <span style="font-size:0.85rem; font-weight:600; color:var(--accent-primary);">${selectedRawObj.friendlyName || selectedRawObj.instanceName}</span>
                                <span style="font-size:0.75rem; color:var(--text-muted); font-family:monospace;">${selectedRawObj.instanceName.split('.').pop()}</span>
                            </div>
                            <textarea id="admin-raw-json" rows="12" 
                                      style="width:100%; font-family:monospace; font-size:0.8rem; background:#0b0f16; color:#a7f3d0; border:1px solid var(--border); border-radius:6px; padding:0.75rem; box-sizing:border-box;">${JSON.stringify(selectedRawObj.properties || {}, null, 2)}</textarea>
                            <div style="margin-top:0.5rem; display:flex; justify-content:space-between; align-items:center;">
                                <span style="font-size:0.75rem; color:var(--text-muted);">Edit JSON values directly, then click Apply.</span>
                                <button class="btn btn-primary" onclick="FicsitAdmin.saveRawProperties()">💾 Apply Changes to Object</button>
                            </div>
                        </div>
                    ` : ''}
                </div>
            ` : ''}
        </div>
    </div>`;
}
