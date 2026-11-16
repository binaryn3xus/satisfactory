// =============================================================================
// FICSIT Admin Control Center - Purpose-Built Management Workstation
// =============================================================================

export function friendlyItemName(path) {
    if (!path) return 'Empty';
    const name = path.split('.').pop().replace(/^Desc_/, '').replace(/_C$/, '');
    const overrides = {
        'Cement': 'Concrete',
        'ResourceSinkCoupon': 'FICSIT Coupon',
        'WAT1': 'Mercer Sphere',
        'WAT2': 'Somersloop',
        'CrystalShard': 'Power Shard',
        'LiquidOil': 'Crude Oil',
        'SpaceElevatorPart_1': 'Smart Plating',
        'SpaceElevatorPart_2': 'Versatile Framework',
        'SpaceElevatorPart_3': 'Automated Wiring',
        'SpaceElevatorPart_4': 'Modular Engine',
        'SpaceElevatorPart_5': 'Adaptive Control Unit',
        'CircuitBoardHighSpeed': 'High-Speed Circuit',
        'IronPlateReinforced': 'Reinforced Plate',
        'HighSpeedWire': 'Quickwire'
    };
    if (overrides[name]) return overrides[name];
    return name.replace(/([a-z])([A-Z])/g, '$1 $2');
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

export function renderAdminWorkspaceHTML({ data, activePioneerName, unsavedChanges, noticeMsg, showRawEditor, selectedRawObj, rawCategory, rawQuery, rawFilteredObjects }) {
    const { header, pioneers, rawPlayerStates, totalClones, worldInfo } = data;
    const corruptPlayers = rawPlayerStates.filter(p => p.hasCorruptRespawn);

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
                            ? `<span style="color:#f87171;">Found ${corruptPlayers.length} player(s) (${corruptPlayers.map(p => p.name).join(', ')}) with 63-slot crashing respawn tables!</span>` 
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
        </div>

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
                            <span style="font-size:0.75rem; background:rgba(59,130,246,0.15); color:#60a5fa; border:1px solid rgba(59,130,246,0.3); padding:2px 8px; border-radius:10px;">
                                ${activePioneer?.primaryState?.hasPawn ? 'Active In-World' : 'Offline'}
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
                <div style="background:#0f141c; border:1px dashed var(--border); border-radius:6px; padding:0.85rem; display:flex; justify-content:space-between; align-items:center; flex-wrap:wrap; gap:0.75rem;">
                    <div style="display:flex; align-items:center; gap:0.6rem; flex-wrap:wrap; flex:1;">
                        <strong style="font-size:0.85rem; color:#fff; white-space:nowrap;">🎁 Care Package Injector:</strong>
                        <select id="admin-care-select" onchange="FicsitAdmin.changeCarePreset(this.value)"
                                style="padding:0.45rem 0.6rem; background:#131a24; color:#fff; border:1px solid var(--border); border-radius:6px; font-size:0.82rem; min-width:220px;">
                            <option value="/Game/FactoryGame/Resource/Parts/ResourceSinkCoupon/Desc_ResourceSinkCoupon.Desc_ResourceSinkCoupon_C" data-qty="50">🎫 FICSIT Coupons (50x)</option>
                            <option value="/Game/FactoryGame/Resource/Environment/CrashSites/Desc_HardDrive.Desc_HardDrive_C" data-qty="1">💾 Hard Drive (1x)</option>
                            <option value="/Game/FactoryGame/Prototype/WAT/Desc_WAT1.Desc_WAT1_C" data-qty="5">🔮 Mercer Sphere (5x)</option>
                            <option value="/Game/FactoryGame/Prototype/WAT/Desc_WAT2.Desc_WAT2_C" data-qty="5">🌀 Somersloop (5x)</option>
                            <option value="/Game/FactoryGame/Resource/Parts/Cement/Desc_Cement.Desc_Cement_C" data-qty="500">🧱 Concrete (500x)</option>
                            <option value="/Game/FactoryGame/Resource/Parts/IronPlateReinforced/Desc_IronPlateReinforced.Desc_IronPlateReinforced_C" data-qty="100">⚙️ Reinforced Plate (100x)</option>
                            <option value="/Game/FactoryGame/Resource/Parts/ModularFrame/Desc_ModularFrame.Desc_ModularFrame_C" data-qty="50">🔩 Modular Frame (50x)</option>
                            <option value="/Game/FactoryGame/Resource/Parts/SAMFluctuator/Desc_SAMFluctuator.Desc_SAMFluctuator_C" data-qty="50">🔋 SAM Fluctuator (50x)</option>
                        </select>
                        <input type="number" id="admin-care-qty" value="50" min="1" max="5000" 
                               style="width:80px; padding:0.45rem; background:#131a24; color:#fff; border:1px solid var(--border); border-radius:6px; font-size:0.82rem;">
                    </div>
                    <button class="btn btn-primary" onclick="FicsitAdmin.grantCarePackage('${activePioneer?.name}')" style="font-size:0.82rem; padding:0.5rem 1rem;">
                        🎁 Add to Inventory
                    </button>
                </div>

                <!-- Visual Inventory Occupancy Bar -->
                <div>
                    <div style="display:flex; justify-content:space-between; align-items:center; margin-bottom:0.35rem; font-size:0.82rem;">
                        <span style="color:var(--text-muted);">
                            <strong>Backpack Occupancy:</strong> ${occupiedCount} of ${totalSlots} slots occupied (${totalSlots - occupiedCount} free)
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
            </div>
        </div>

        <!-- 3. World & Progression Controls -->
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
