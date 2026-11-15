#!/usr/bin/env bash
set -euo pipefail

# ---------------------------------------------------------------------------
# 1. Privilege & Ownership Check (OnRootMismatch behavior for Docker)
# ---------------------------------------------------------------------------
PUID="${PUID:-1000}"
PGID="${PGID:-1000}"

if [ "$(id -u)" = "0" ]; then
    if [ "$(id -g steam)" != "$PGID" ]; then
        groupmod -o -g "$PGID" steam
    fi
    if [ "$(id -u steam)" != "$PUID" ]; then
        usermod -o -u "$PUID" steam
    fi

    mkdir -p /config
    if [ "$(stat -c '%u:%g' /config)" != "${PUID}:${PGID}" ]; then
        echo "==> Adjusting ownership of /config to ${PUID}:${PGID} (OnRootMismatch)..."
        chown -R "${PUID}:${PGID}" /config
    fi

    exec gosu steam "$0" "$@"
fi

echo "============================================================================"
echo "  FICSIT Dedicated Server Runtime (UID=$(id -u), GID=$(id -g))"
echo "============================================================================"

# ---------------------------------------------------------------------------
# 2. Environment Variable Normalization (New + Legacy wolveix compatibility)
# ---------------------------------------------------------------------------
export SERVER_NAME="${SERVER_NAME:-Satisfactory Server}"
export SERVER_GAME_PORT="${SERVER_GAME_PORT:-${SERVERGAMEPORT:-7777}}"
export SERVER_MESSAGING_PORT="${SERVER_MESSAGING_PORT:-${SERVERMESSAGINGPORT:-8888}}"
export MAX_PLAYERS="${MAX_PLAYERS:-${MAXPLAYERS:-4}}"
export MAX_TICK_RATE="${MAX_TICK_RATE:-${MAXTICKRATE:-30}}"
export AUTOSAVE_NUM="${AUTOSAVE_NUM:-${AUTOSAVENUM:-5}}"
export AUTOSAVE_INTERVAL="${AUTOSAVE_INTERVAL:-300}"
export MAX_OBJECTS="${MAX_OBJECTS:-${MAXOBJECTS:-2162688}}"
export TIMEOUT="${TIMEOUT:-30}"
export SERVER_STREAMING="${SERVER_STREAMING:-${SERVERSTREAMING:-true}}"
export AUTO_PAUSE="${AUTO_PAUSE:-true}"
export DISABLE_SEASONAL_EVENTS="${DISABLE_SEASONAL_EVENTS:-${DISABLESEASONALEVENTS:-false}}"

if [ -z "${STEAM_BRANCH:-}" ]; then
    if [ "${STEAMBETA:-false}" = "true" ]; then
        export STEAM_BRANCH="experimental"
    else
        export STEAM_BRANCH="public"
    fi
fi

if [ -z "${STEAM_UPDATE_ON_START:-}" ]; then
    if [ "${SKIPUPDATE:-false}" = "true" ]; then
        export STEAM_UPDATE_ON_START="false"
    else
        export STEAM_UPDATE_ON_START="true"
    fi
fi

export STEAM_VALIDATE="${STEAM_VALIDATE:-false}"
export BACKUP_KEEP_COUNT="${BACKUP_KEEP_COUNT:-5}"
export GAMEFILES_DIR="${GAMEFILES_DIR:-/config/gamefiles}"
export SAVES_DIR="${SAVES_DIR:-/config/saved/server}"
export BLUEPRINTS_DIR="${BLUEPRINTS_DIR:-/config/saved/blueprints}"
export BACKUPS_DIR="${BACKUPS_DIR:-/config/backups}"
export FICSIT_LOCAL_DIR="${FICSIT_LOCAL_DIR:-/config/ficsit/local}"
export FICSIT_CACHE_DIR="${FICSIT_CACHE_DIR:-/config/ficsit/cache}"
export TLS_CERT_DIR="${TLS_CERT_DIR:-/certs}"

# ---------------------------------------------------------------------------
# 3. Directory & Symlink Hygiene
# ---------------------------------------------------------------------------
mkdir -p \
    "$GAMEFILES_DIR" \
    "$SAVES_DIR" \
    "$BLUEPRINTS_DIR" \
    "$BACKUPS_DIR" \
    "/config/logs/steam" \
    "$FICSIT_LOCAL_DIR" \
    "$FICSIT_CACHE_DIR" \
    "$HOME/.config/Epic/FactoryGame/Saved" \
    "$HOME/.local/share" \
    "$HOME/.cache"

ln -sfn "/config/saved" "$HOME/.config/Epic/FactoryGame/Saved/SaveGames"
ln -sfn "$BLUEPRINTS_DIR" "$SAVES_DIR/blueprints" 2>/dev/null || true
ln -sfn "$FICSIT_LOCAL_DIR" "$HOME/.local/share/ficsit"
ln -sfn "$FICSIT_CACHE_DIR" "$HOME/.cache/ficsit"

# ---------------------------------------------------------------------------
# 4. Built-in TLS Certificate Sync (Replaces busybox initContainer)
# ---------------------------------------------------------------------------
sync_tls_certificates() {
    local cert_src=""
    local key_src=""

    if [ -f "${TLS_CERT_DIR}/cert_chain.pem" ] && [ -f "${TLS_CERT_DIR}/private_key.pem" ]; then
        cert_src="${TLS_CERT_DIR}/cert_chain.pem"
        key_src="${TLS_CERT_DIR}/private_key.pem"
    elif [ -f "${TLS_CERT_DIR}/tls.crt" ] && [ -f "${TLS_CERT_DIR}/tls.key" ]; then
        cert_src="${TLS_CERT_DIR}/tls.crt"
        key_src="${TLS_CERT_DIR}/tls.key"
    fi

    if [ -n "$cert_src" ] && [ -n "$key_src" ]; then
        local target_dir="${GAMEFILES_DIR}/FactoryGame/Certificates"
        mkdir -p "$target_dir"
        cp -L "$cert_src" "${target_dir}/cert_chain.pem"
        cp -L "$key_src" "${target_dir}/private_key.pem"
        chmod 600 "${target_dir}/cert_chain.pem" "${target_dir}/private_key.pem"
        echo "==> Synced TLS certificates from ${TLS_CERT_DIR} to ${target_dir}"
    fi
}

# ---------------------------------------------------------------------------
# 5. Smart Pre-Update Backup Snapshot with Rotation
# ---------------------------------------------------------------------------
create_pre_update_backup() {
    local label="${1:-manual}"
    shopt -s nullglob
    local sav_files=("$SAVES_DIR"/*.sav)
    shopt -u nullglob

    if [ ${#sav_files[@]} -eq 0 ]; then
        return 0
    fi

    local ts
    ts="$(date -u +%Y%m%d_%H%M%S)"
    local snap_dir="${BACKUPS_DIR}/pre-update-${label}-${ts}"
    mkdir -p "$snap_dir"
    cp -p "$SAVES_DIR"/*.sav "$snap_dir"/ 2>/dev/null || true
    echo "==> Created pre-update save snapshot: ${snap_dir} (${#sav_files[@]} save files)"

    # Rotate older pre-update-* snapshots beyond BACKUP_KEEP_COUNT
    if [[ "$BACKUP_KEEP_COUNT" =~ ^[0-9]+$ ]] && [ "$BACKUP_KEEP_COUNT" -gt 0 ]; then
        mapfile -t old_snaps < <(ls -1dt "${BACKUPS_DIR}"/pre-update-* 2>/dev/null | tail -n +"$((BACKUP_KEEP_COUNT + 1))")
        for old in "${old_snaps[@]}"; do
            [ -n "$old" ] && rm -rf "$old"
        done
    fi
}

# ---------------------------------------------------------------------------
# 6. Smart SteamCMD Update Pipeline
# ---------------------------------------------------------------------------
ACF_FILE="${GAMEFILES_DIR}/steamapps/appmanifest_1690800.acf"
LOCAL_BUILD_ID=""
if [ -f "$ACF_FILE" ]; then
    LOCAL_BUILD_ID="$(awk -F'"' '/"buildid"/ {print $4; exit}' "$ACF_FILE" || true)"
fi

OLD_CL="$(ficsit-ctl detect-changelist 2>/dev/null || echo "")"

NEED_STEAM_UPDATE="false"
if [ ! -f "${GAMEFILES_DIR}/FactoryServer.sh" ]; then
    echo "==> Fresh installation detected (FactoryServer.sh missing). Forcing SteamCMD install..."
    NEED_STEAM_UPDATE="true"
elif [ "${STEAM_UPDATE_ON_START,,}" = "true" ]; then
    echo "==> Checking remote Steam build ID for branch '${STEAM_BRANCH}'..."
    REMOTE_BUILD_ID="$(steamcmd +login anonymous +app_info_update 1 +app_info_print 1690800 +quit 2>/dev/null \
        | awk -v branch="$STEAM_BRANCH" '
            $0 ~ "\"" branch "\"" { in_branch=1 }
            in_branch && /"buildid"/ { gsub(/[^0-9]/, "", $2); print $2; exit }
        ' || true)"

    if [ "${STEAM_VALIDATE,,}" = "true" ]; then
        echo "==> STEAM_VALIDATE=true requested. Will run SteamCMD update + validate."
        NEED_STEAM_UPDATE="true"
    elif [ -z "$LOCAL_BUILD_ID" ] || [ -z "$REMOTE_BUILD_ID" ] || [ "$LOCAL_BUILD_ID" != "$REMOTE_BUILD_ID" ]; then
        echo "==> Update available (Local BuildID: ${LOCAL_BUILD_ID:-none}, Remote BuildID: ${REMOTE_BUILD_ID:-unknown})."
        NEED_STEAM_UPDATE="true"
    else
        echo "==> Satisfactory server is already up to date (BuildID: ${LOCAL_BUILD_ID}, CL-${OLD_CL:-unknown}). Skipping SteamCMD download."
    fi
else
    echo "==> STEAM_UPDATE_ON_START=false; skipping SteamCMD update check."
fi

if [ "$NEED_STEAM_UPDATE" = "true" ]; then
    create_pre_update_backup "CL-${OLD_CL:-init}"

    STEAM_ARGS=(
        "+force_install_dir" "$GAMEFILES_DIR"
        "+login" "anonymous"
        "+app_update" "1690800"
        "-beta" "$STEAM_BRANCH"
    )
    if [ -n "${STEAM_BETA_PASSWORD:-}" ]; then
        STEAM_ARGS+=("-betapassword" "$STEAM_BETA_PASSWORD")
    fi
    if [ "${STEAM_VALIDATE,,}" = "true" ]; then
        STEAM_ARGS+=("validate")
    fi
    STEAM_ARGS+=("+quit")

    echo "==> Running SteamCMD update..."
    if ! steamcmd "${STEAM_ARGS[@]}"; then
        echo "==> SteamCMD returned an error on first attempt (refreshing appcache and retrying once)..."
        rm -rf "$HOME/.local/share/Steam/appcache" 2>/dev/null || true
        steamcmd "${STEAM_ARGS[@]}"
    fi

    NEW_CL="$(ficsit-ctl detect-changelist 2>/dev/null || echo "")"
    if [ -n "$OLD_CL" ] && [ -n "$NEW_CL" ] && [ "$OLD_CL" != "$NEW_CL" ]; then
        ficsit-ctl notify-update "CL-${OLD_CL}" "CL-${NEW_CL}" || true
    fi
fi

# Re-sync TLS certificates after SteamCMD in case FactoryGame/Certificates was created/reset
sync_tls_certificates

# ---------------------------------------------------------------------------
# 7. Declarative Mod Reconciliation & Strict World-Protection Gate
# ---------------------------------------------------------------------------
ficsit-ctl reconcile-mods

# ---------------------------------------------------------------------------
# 8. Build Unreal Engine INI Arguments & Launch Server
# ---------------------------------------------------------------------------
[[ "${SERVER_STREAMING,,}" == "true" ]] && STREAMING_VAL="1" || STREAMING_VAL="0"
[[ "${AUTO_PAUSE,,}" == "true" ]] && AUTOPAUSE_VAL="True" || AUTOPAUSE_VAL="False"

INI_ARGS=(
    "-Port=${SERVER_GAME_PORT}"
    "-ReliablePort=${SERVER_MESSAGING_PORT}"
    "-ExternalReliablePort=${SERVER_MESSAGING_PORT}"
    "-ini:Engine:[/Script/FactoryGame.FGSaveSession]:mNumRotatingAutosaves=${AUTOSAVE_NUM}"
    "-ini:Engine:[/Script/FactoryGame.FGSaveSession]:mDefaultAutosaveInterval=${AUTOSAVE_INTERVAL}"
    "-ini:Engine:[/Script/Engine.GarbageCollectionSettings]:gc.MaxObjectsInEditor=${MAX_OBJECTS}"
    "-ini:Engine:[/Script/OnlineSubsystemUtils.IpNetDriver]:LanServerMaxTickRate=${MAX_TICK_RATE}"
    "-ini:Engine:[/Script/OnlineSubsystemUtils.IpNetDriver]:NetServerMaxTickRate=${MAX_TICK_RATE}"
    "-ini:Engine:[/Script/OnlineSubsystemUtils.IpNetDriver]:ConnectionTimeout=${TIMEOUT}"
    "-ini:Engine:[/Script/OnlineSubsystemUtils.IpNetDriver]:InitialConnectTimeout=${TIMEOUT}"
    "-ini:Engine:[ConsoleVariables]:wp.Runtime.EnableServerStreaming=${STREAMING_VAL}"
    "-ini:Game:[/Script/Engine.GameSession]:ConnectionTimeout=${TIMEOUT}"
    "-ini:Game:[/Script/Engine.GameSession]:InitialConnectTimeout=${TIMEOUT}"
    "-ini:Game:[/Script/Engine.GameSession]:MaxPlayers=${MAX_PLAYERS}"
    "-ini:GameUserSettings:[/Script/Engine.GameSession]:MaxPlayers=${MAX_PLAYERS}"
    "-ini:ServerSettings:[/Script/FactoryGame.FGServerSubsystem]:mAutoPause=${AUTOPAUSE_VAL}"
)

if [[ "${DISABLE_SEASONAL_EVENTS,,}" == "true" ]]; then
    INI_ARGS+=("-DisableSeasonalEvents")
fi

if [[ -n "${MULTIHOME:-}" ]]; then
    INI_ARGS+=("-multihome=${MULTIHOME}")
fi

# Append any custom newline-separated ENGINE_INI_OVERRIDES or GAME_INI_OVERRIDES
if [[ -n "${EXTRA_SERVER_ARGS:-}" ]]; then
    # shellcheck disable=SC2206
    EXTRA_SPLIT=($EXTRA_SERVER_ARGS)
    INI_ARGS+=("${EXTRA_SPLIT[@]}")
fi

cd "$GAMEFILES_DIR"
chmod +x FactoryServer.sh || true

# Ensure target game port is free before launching to prevent Unreal Engine from bumping the HTTPS port (e.g. 7778 -> 7779)
if command -v ss >/dev/null 2>&1; then
    for i in $(seq 1 15); do
        if ! ss -tuln | grep -qE "[: ]${SERVER_GAME_PORT}[[:space:]]"; then
            break
        fi
        echo "==> Port ${SERVER_GAME_PORT} is currently busy or lingering. Waiting for socket to clear ($i/15)..."
        sleep 1
    done
fi

echo "==> Launching Satisfactory Dedicated Server on UDP/TCP ${SERVER_GAME_PORT} (Messaging TCP ${SERVER_MESSAGING_PORT})..."
./FactoryServer.sh "${INI_ARGS[@]}" "$@" &
WRAPPER_PID=$!

shutdown_handler() {
    echo ""
    echo "==> Signal received: running pre-shutdown hook..."
    ficsit-ctl pre-stop || true

    echo "==> Forwarding SIGINT to Satisfactory server process..."
    pkill -INT -P "$WRAPPER_PID" 2>/dev/null || kill -INT "$WRAPPER_PID" 2>/dev/null || true
    wait "$WRAPPER_PID" 2>/dev/null || true
    echo "==> Satisfactory server stopped cleanly."
    exit 0
}

trap shutdown_handler SIGINT SIGTERM

wait "$WRAPPER_PID"
