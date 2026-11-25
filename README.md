# Satisfactory Dedicated Server, Mod Manager & Portal

A container-native **Satisfactory Dedicated Server** suite written in Go, featuring declarative **`ficsit-cli` Mod Management**, a **Strict World-Protection Gate**, a lightweight **Save / Mod / Blueprint Web Portal** sidecar, first-class **Docker Compose** support, and a production-grade **OCI Helm Chart**.

---

## 📑 Table of Contents

- [📦 Published Artifacts](#-published-artifacts)
- [✨ Key Features](#-key-features)
- [☸️ Kubernetes Helm Chart (`charts/satisfactory`)](#️-kubernetes-helm-chart-chartssatisfactory)
  - [Standard Helm Installation](#standard-helm-installation)
  - [ConfigMap Generation & Environment Expansion](#configmap-generation--environment-expansion)
  - [GitOps with Flux & Controlled Renovate Upgrades](#gitops-with-flux--controlled-renovate-upgrades)
  - [Complete `values.yaml` Parameters](#complete-valuesyaml-parameters)
  - [Gateway API (`HTTPRoute`) & Ingress](#gateway-api-httproute--ingress)
  - [Automated TLS Certificates (`cert-manager`)](#automated-tls-certificates-cert-manager)
  - [Off-Site NFS Backup CronJob](#off-site-nfs-backup-cronjob)
- [🐳 Quickstart: Docker Compose](#-quickstart-docker-compose)
  - [Quick Start Commands](#quick-start-commands)
  - [Standalone `docker-compose.yml` Snippet](#standalone-docker-composeyml-snippet)
- [⚙️ Environment Variables Reference](#️-environment-variables-reference)
  - [Server Core Configuration](#server-core-configuration)
  - [SteamCMD & Engine Updates](#steamcmd--engine-updates)
  - [Declarative Mod Management (`ficsit-cli`)](#declarative-mod-management-ficsit-cli)
  - [Save, Mod & Blueprint Web Portal](#save-mod--blueprint-web-portal)
  - [TLS Certificates & Notifications](#tls-certificates--notifications)
  - [Environment Variable Aliases](#environment-variable-aliases)
- [🌐 Web Portal & Prometheus Telemetry](#-web-portal--prometheus-telemetry)
  - [Portal Routes & Endpoints](#portal-routes--endpoints)
  - [Exported Prometheus Metrics](#exported-prometheus-metrics)
- [🛠️ Local Development](#️-local-development)

---

## 📦 Published Artifacts

All images and the Helm chart are built from this repository and published to GitHub Container Registry (`ghcr.io`):

| Artifact | Registry URI | Description |
| :--- | :--- | :--- |
| **Server Image** | `ghcr.io/binaryn3xus/satisfactory-server` | SteamCMD + `ficsit-cli` + `ficsit-ctl` dedicated server lifecycle controller |
| **Portal Image** | `ghcr.io/binaryn3xus/satisfactory-portal` | Save downloader, SCIM links, live `.smmprofile` export, Blueprints `.zip` streamer, pioneer tracker & Prometheus `/metrics` |
| **Helm Chart** | `oci://ghcr.io/binaryn3xus/charts/satisfactory` | Production Kubernetes Helm chart supporting Gateway API (`HTTPRoute`), standard `Ingress`, `cert-manager`, and NFS backups |

> [!NOTE]
> All container images and the OCI Helm chart use synchronized Semantic Versioning (e.g. `0.2.0`, `0.2`, `latest`).

---

## ✨ Key Features

- **Declarative Mod Management (`ficsit-cli`):**
  Specify mods via `MODS_LIST` (e.g. `InfiniteNudge, SmartFoundations@^2.0.0`) or `/config/mods.json`. Supports `latest` (mapped to `>=0.0.0`) and full SemVer ranges, with `always` (auto-upgrade to newest compatible release) or `locked` update policies.
- **Strict World-Protection Gate (`MODS_STRICT_MODE=true`):**
  Before `FactoryServer.sh` is ever executed, `ficsit-ctl` verifies that `ficsit-cli apply` succeeded, `SML.uplugin` is present, and every declared mod is in the resolved lockfile and on disk. If a new Satisfactory patch drops before a mod updates, startup halts cleanly so Unreal Engine never loads a modded save without its mods.
- **Smart SteamCMD Updates & Pre-Update Save Snapshots:**
  Compares the local `appmanifest_1690800.acf` build ID against Steam's remote build ID on startup—skipping redundant SteamCMD downloads when already up to date, and automatically snapshotting `.sav` files to `/config/backups/pre-update-CL-<old>` (with retention rotation) before applying a game update.
- **Unauthenticated Local Telemetry (`ALLOW_INSECURE_LOCAL_ACCESS=true`):**
  Enables `FG.DedicatedServer.AllowInsecureLocalAccess=1` on the dedicated server by default. This allows the co-located portal sidecar to query `QueryServerState` directly over `127.0.0.1` to display server tick rate (TPS), connected players, and current tech tier without requiring an exposed admin API token.
- **Built-in TLS Certificate Sync & Pre-Shutdown Save Hook:**
  Automatically syncs mounted TLS certificates from `/certs` into `FactoryGame/Certificates` (no Kubernetes `initContainer` needed) and calls Satisfactory's HTTPS API (`SaveGame`) before shutting down when `SERVER_API_TOKEN` is configured.
- **Save, Mod & Blueprint Web Portal (`satisfactory-portal`):**
  - **Live Status & Pioneer Tracker:** Combines the Satisfactory HTTPS API with a real-time `FactoryGame.log` parser to show **Who's Online** and **Recently Seen** on both Vanilla and Modded servers (and automatically enriches with ping/power telemetry if `FicsitRemoteMonitoring` is active).
  - **Admin Control Center (`/admin`):** Password-protected (`PORTAL_ADMIN_PASSWORD` / `portal.adminPassword`) web console featuring live dedicated server `.sav` upload, server `LoadGame` execution, interactive Death Crate & Loot Radar (teleport, claim, purge), and MAM alternate recipe research station.
  - **Binary `.sav` Header & Web Inspector:** Reads `.sav` headers for session name, playtime, and changelist with one-click **SCIM Interactive Map** links and built-in client-side save inspection.
  - **One-Click `.smmprofile` Export:** Dynamically builds a live `.smmprofile` from the server's lockfile (plus optional `MODS_CLIENT_ONLY` QoL mods) for one-click import into Desktop Satisfactory Mod Manager.
  - **Blueprints Browser & `.zip` Streamer:** Parses `.sbp` + `.sbpcfg` descriptions and streams single or full-server blueprint `.zip` bundles.
  - **Prometheus `/metrics`:** Exposes tick rate, connected players, tech tier, mod count, save size, playtime, and blueprint count.

---


## ☸️ Kubernetes Helm Chart (`charts/satisfactory`)

### Standard Helm Installation

The chart is packaged and published as an OCI artifact to GitHub Container Registry (`ghcr.io`):

```bash
# 1. Inspect chart metadata
helm show chart oci://ghcr.io/binaryn3xus/charts/satisfactory --version 0.2.1

# 2. Deploy using a custom values.yaml
helm upgrade --install satisfactory oci://ghcr.io/binaryn3xus/charts/satisfactory \
  --version 0.2.1 \
  --namespace satisfactory --create-namespace \
  -f values.yaml
```

#### Example `values.yaml`:
```yaml
server:
  config:
    serverName: "FICSIT-Automated-Facility"
    gamePort: 7777
    messagingPort: 8888
    maxPlayers: 8
    maxTickRate: 30

mods:
  enabled: true
  strictMode: true
  profileName: "default"
  items:
    - ref: InfiniteNudge
      version: latest
    - ref: SmartFoundations
      version: "^2.0.0"

portal:
  enabled: true
  ingress:
    enabled: true
    hosts:
      - host: satisfactory-saves.example.com
        paths:
          - path: /
            pathType: Prefix

persistence:
  enabled: true
  size: 50Gi

service:
  game:
    type: LoadBalancer
```

---

### ConfigMap Generation & Environment Expansion

The chart automatically generates a centralized ConfigMap named `<release>-env` and mounts it to both the `server` and `portal` containers via `envFrom`.

#### Automatic List-to-String Expansion for Mods
Rather than requiring you to manually format newline-delimited environment strings, the chart takes structured YAML in `mods.items` and automatically expands it into the `MODS_LIST` format required by `ficsit-cli`:

```yaml
# In your values.yaml:
mods:
  items:
    - ref: InfiniteNudge
      version: latest
    - ref: SmartFoundations
      version: "^2.0.0"
```

Expands in the rendered Kubernetes `ConfigMap` to:

```yaml
# Rendered ConfigMap (<release>-env):
apiVersion: v1
kind: ConfigMap
metadata:
  name: satisfactory-env
data:
  SERVER_NAME: "FICSIT-Automated-Facility"
  SERVER_GAME_PORT: "7777"
  SERVER_MESSAGING_PORT: "8888"
  MAX_PLAYERS: "8"
  MAX_TICK_RATE: "30"
  AUTOSAVE_NUM: "5"
  AUTOSAVE_INTERVAL: "300"
  AUTO_PAUSE: "true"
  ALLOW_INSECURE_LOCAL_ACCESS: "true"
  STEAM_BRANCH: "public"
  STEAM_UPDATE_ON_START: "true"
  MODS_ENABLED: "true"
  MODS_STRICT_MODE: "true"
  MODS_PROFILE: "default"
  MODS_UPDATE_POLICY: "always"
  MODS_LIST: |
    InfiniteNudge@latest
    SmartFoundations@^2.0.0
```

#### Attaching Custom ConfigMaps or Secrets
If you maintain additional configuration in external ConfigMaps or Secrets, you can inject them directly into the dedicated server container using `server.extraEnvFrom` and `server.extraEnv`:

```yaml
server:
  extraEnv:
    - name: CUSTOM_SERVER_FLAG
      value: "true"
  extraEnvFrom:
    - configMapRef:
        name: custom-engine-settings
    - secretRef:
        name: custom-server-credentials
```

---

### GitOps with Flux & Controlled Renovate Upgrades

For GitOps workflows, deploy the chart using Flux's `OCIRepository` alongside a `HelmRelease`:

<details>
<summary><strong>Click to view Flux CD & Renovate GitOps manifests</strong></summary>

#### 1. Define the OCI Repository (`ocirepository.yaml`):
```yaml
apiVersion: source.toolkit.fluxcd.io/v1
kind: OCIRepository
metadata:
  name: satisfactory
  namespace: default
spec:
  interval: 15m
  layerSelector:
    mediaType: application/vnd.cncf.helm.chart.content.v1.tar+gzip
    operation: copy
  ref:
    tag: 0.2.1
  url: oci://ghcr.io/binaryn3xus/charts/satisfactory
```

#### 2. Define the HelmRelease (`helmrelease.yaml`):
```yaml
apiVersion: helm.toolkit.fluxcd.io/v2
kind: HelmRelease
metadata:
  name: satisfactory
  namespace: default
spec:
  interval: 30m
  chartRef:
    kind: OCIRepository
    name: satisfactory
  values:
    server:
      config:
        serverName: "FICSIT-Automated-Facility"
        gamePort: 7777
        messagingPort: 8888
        maxPlayers: 8
        maxTickRate: 30
    mods:
      enabled: true
      strictMode: true
      profileName: "default"
      items:
        - ref: InfiniteNudge
          version: latest
    portal:
      enabled: true
      route:
        enabled: true
        parentRefs:
          - name: envoy-external
            namespace: network
            sectionName: https
        hostnames:
          - satisfactory-saves.example.com
```

> [!TIP]
> **Controlled Updates with Renovate**: By omitting `server.image.tag` and `portal.image.tag`, container image tags default to `.Chart.AppVersion`. When you cut a new release (e.g. `0.2.0 ➔ 0.3.0`), Renovate discovers the new chart tag in GHCR and opens a single PR against `ocirepository.yaml`. Merging that PR upgrades the chart and matching container images together in atomic lockstep.

</details>

---

### Complete `values.yaml` Parameters

<details>
<summary><strong>Click to expand full Helm Values reference table</strong></summary>

#### Scheduling & Security
| Parameter | Default | Description |
| :--- | :--- | :--- |
| `nameOverride` | `""` | Override chart release name |
| `fullnameOverride` | `""` | Override full release name |
| `terminationGracePeriodSeconds` | `120` | Grace period for game save and clean shutdown |
| `nodeSelector` | `{}` | Node labels for pod assignment (e.g. `sku: gaming`) |
| `tolerations` | `[]` | Node taints to tolerate |
| `affinity` | `{}` | Pod affinity / anti-affinity rules |
| `podSecurityContext.fsGroup` | `1000` | GID owning mounted volumes |
| `podSecurityContext.fsGroupChangePolicy` | `OnRootMismatch` | Efficient permission management |

#### Dedicated Server (`server.*`)
| Parameter | Default | Description |
| :--- | :--- | :--- |
| `server.image.repository` | `ghcr.io/binaryn3xus/satisfactory-server` | Dedicated server container image |
| `server.image.tag` | `""` | Defaults to `.Chart.AppVersion` |
| `server.image.pullPolicy` | `IfNotPresent` | Image pull policy |
| `server.config.serverName` | `Satisfactory Server` | In-game server name |
| `server.config.gamePort` | `7777` | Game and API port (UDP/TCP) |
| `server.config.messagingPort` | `8888` | Messaging port (TCP) |
| `server.config.maxPlayers` | `4` | Maximum player connections |
| `server.config.maxTickRate` | `30` | Server tick rate |
| `server.config.autoSaveNum` | `5` | Number of rotating autosaves |
| `server.config.autoSaveInterval` | `300` | Autosave interval in seconds |
| `server.config.autoPause` | `true` | Pause world when empty |
| `server.config.serverStreaming` | `true` | Enable server world streaming |
| `server.config.allowInsecureLocalAccess` | `true` | Enable unauthenticated 127.0.0.1 telemetry |
| `server.config.steam.branch` | `public` | Steam branch (`public` or `experimental`) |
| `server.config.steam.updateOnStart` | `true` | Smart update check on boot |
| `server.config.steam.validate` | `false` | Force SteamCMD cache validation |
| `server.resources.requests.cpu` | `2` | CPU request |
| `server.resources.requests.memory` | `4Gi` | Memory request |
| `server.resources.limits.cpu` | `4` | CPU limit |
| `server.resources.limits.memory` | `8Gi` | Memory limit |

#### Declarative Mods (`mods.*`)
| Parameter | Default | Description |
| :--- | :--- | :--- |
| `mods.enabled` | `false` | Enable `ficsit-cli` mod management |
| `mods.strictMode` | `true` | Prevent boot on mod resolution failures |
| `mods.profileName` | `default` | Active `ficsit-cli` profile name |
| `mods.updatePolicy` | `always` | Mod resolution policy (`always` or `locked`) |
| `mods.items` | `[]` | List of server mods (`ref`, `version`) |
| `mods.clientOnlyItems` | `[]` | List of client-only mods for `.smmprofile` |

#### Web Portal Sidecar (`portal.*`)
| Parameter | Default | Description |
| :--- | :--- | :--- |
| `portal.enabled` | `true` | Enable portal sidecar |
| `portal.image.repository` | `ghcr.io/binaryn3xus/satisfactory-portal` | Portal container image |
| `portal.image.tag` | `""` | Defaults to `.Chart.AppVersion` |
| `portal.port` | `8081` | HTTP portal listen port (defaults to `8081` to avoid collision with FRM on `8080`) |
| `portal.readOnly` | `false` | Mount `/config` as read-only in portal container (set to `true` to block save writes) |
| `portal.frmApiUrl` | `""` | Optional FicsitRemoteMonitoring URL |
| `portal.adminPassword.value` | `""` | Optional plain admin password for `/admin` (when unset, `/admin` is in open-access mode) |
| `portal.adminPassword.existingSecret` | `""` | Kubernetes Secret name containing portal admin password |
| `portal.adminPassword.secretKey` | `ADMIN_PASSWORD` | Key inside `existingSecret` |
| `portal.extraEnv` | `[]` | Additional environment variables for the portal container |
| `portal.securityContext.readOnlyRootFilesystem` | `true` | Hardened read-only container root |
| `portal.route.enabled` | `false` | Enable Gateway API `HTTPRoute` |
| `portal.ingress.enabled` | `false` | Enable Kubernetes `Ingress` |
| `portal.serviceMonitor.enabled` | `false` | Prometheus Operator ServiceMonitor |

#### Services, TLS & Storage
| Parameter | Default | Description |
| :--- | :--- | :--- |
| `service.game.type` | `LoadBalancer` | Service type for game traffic (`LoadBalancer`, `NodePort`) |
| `tls.certManager.enabled` | `false` | Create `cert-manager` Certificate resource automatically |
| `tls.existingSecret` | `""` | Mount existing `kubernetes.io/tls` secret |
| `persistence.enabled` | `true` | Persistent storage for `/config` |
| `persistence.size` | `50Gi` | PVC volume size |
| `backup.enabled` | `false` | Scheduled off-site NFS backup CronJob |
| `backup.nfs.server` | `""` | Target NFS server IP/hostname |
| `backup.nfs.path` | `""` | NFS export path |
| `notifications.discordWebhookUrl` | `""` | Discord webhook URL |

</details>

---

### Gateway API (`HTTPRoute`) & Ingress

The portal sidecar supports modern Kubernetes Gateway API (`HTTPRoute`) and standard `Ingress`:

#### Gateway API (Envoy Gateway / Cilium / Istio):
```yaml
portal:
  route:
    enabled: true
    parentRefs:
      - name: envoy-external
        namespace: network
        sectionName: https
    hostnames:
      - satisfactory-saves.example.com
```

#### Standard Kubernetes Ingress (NGINX / Traefik):
```yaml
portal:
  ingress:
    enabled: true
    className: nginx
    annotations:
      cert-manager.io/cluster-issuer: letsencrypt-production
    hosts:
      - host: satisfactory-saves.example.com
        paths:
          - path: /
            pathType: Prefix
    tls:
      - secretName: satisfactory-portal-tls
        hosts:
          - satisfactory-saves.example.com
```

---

### Automated TLS Certificates (`cert-manager`)

To automate valid TLS certificates for Satisfactory's HTTPS game API without manual secret management:

```yaml
tls:
  certManager:
    enabled: true
    commonName: satisfactory.example.com
    dnsNames:
      - satisfactory.example.com
    issuerRef:
      name: letsencrypt-production
      kind: ClusterIssuer
```

The server container automatically detects the secret mounted at `/certs`, copies `tls.crt` and `tls.key` into `FactoryGame/Certificates/`, and sets the required `600` permissions on every boot.

---

### Off-Site NFS Backup CronJob

To maintain an independent off-site backup of your game saves, blueprints, and server configuration:

```yaml
backup:
  enabled: true
  schedule: "0 */6 * * *"
  timeZone: "America/New_York"
  nfs:
    server: "10.0.30.4"
    path: "/volume1/Media"
    targetSubDir: "Gaming/Satisfactory-Backups"
```

The backup CronJob runs `rsync` across `/config` to your NFS target, rotating snapshots and logging status cleanly.

---

## 🐳 Quickstart: Docker Compose

The repository includes a ready-to-use [`docker-compose.yml`](docker-compose.yml) and [`.env.example`](.env.example) for immediate local or single-node deployments.

### Quick Start Commands

```bash
# 1. Clone the repository
git clone https://github.com/binaryn3xus/satisfactory.git
cd satisfactory

# 2. (Optional) Create your custom environment configuration
cp .env.example .env

# 3. Launch the dedicated game server
docker compose up -d

# Or launch both the server AND the Web Portal sidecar:
docker compose --profile portal up -d

# 4. View live logs
docker compose logs -f

# 5. Stop the server cleanly (allows 120s for save-game hook to complete)
docker compose stop
```

> [!TIP]
> The web portal service is configured with `profiles: ["portal"]`. Running `docker compose up -d` launches the server by itself. Adding `--profile portal` spins up the co-located `satisfactory-portal` sidecar on port `8081`.

### Standalone `docker-compose.yml` Snippet

If you are incorporating this into an existing Docker stack without cloning the repository, use this standalone snippet:

```yaml
services:
  satisfactory-server:
    image: ghcr.io/binaryn3xus/satisfactory-server:0.2.1
    container_name: satisfactory-server
    restart: unless-stopped
    stop_grace_period: 120s
    ports:
      - "7777:7777/udp" # Game Port (UDP)
      - "7777:7777/tcp" # Game & HTTPS API (TCP)
      - "8888:8888/tcp" # Messaging Port (TCP)
    environment:
      SERVER_NAME: "Satisfactory-Server"
      MAX_PLAYERS: "8"
      MAX_TICK_RATE: "30"
      STEAM_UPDATE_ON_START: "true"
      ALLOW_INSECURE_LOCAL_ACCESS: "true"
      MODS_ENABLED: "true"
      MODS_STRICT_MODE: "true"
      MODS_PROFILE: "default"
      MODS_UPDATE_POLICY: "always"
      MODS_LIST: |
        InfiniteNudge
        SmartFoundations@^2.0.0
    volumes:
      - ./data:/config

  satisfactory-portal:
    image: ghcr.io/binaryn3xus/satisfactory-portal:0.2.1
    container_name: satisfactory-portal
    restart: unless-stopped
    ports:
      - "8081:8081/tcp"
    environment:
      PORTAL_PORT: "8081"
      SERVER_NAME: "Satisfactory-Server"
      SERVER_API_URL: "https://satisfactory-server:7777"
      MODS_PROFILE: "default"
    volumes:
      - ./data:/config:ro
```

---

## ⚙️ Environment Variables Reference

### Server Core Configuration

| Variable | Default | Description |
| :--- | :--- | :--- |
| `SERVER_NAME` | `Satisfactory Server` | Display name of the dedicated server in the server browser. |
| `SERVER_GAME_PORT` | `7777` | Primary game and HTTPS API port (UDP and TCP). |
| `SERVER_MESSAGING_PORT` | `8888` | Reliable messaging port (TCP). |
| `MAX_PLAYERS` | `4` | Maximum concurrent player connections. |
| `MAX_TICK_RATE` | `30` | Server tick rate / TPS limit (`LanServerMaxTickRate` & `NetServerMaxTickRate`). |
| `AUTOSAVE_NUM` | `5` | Number of rotating autosaves to retain (`mNumRotatingAutosaves`). |
| `AUTOSAVE_INTERVAL` | `300` | Autosave frequency in seconds (`mDefaultAutosaveInterval`). |
| `AUTO_PAUSE` | `true` | Pause simulation when no players are connected (`mAutoPause`). |
| `SERVER_STREAMING` | `true` | Enables World Partition runtime streaming (`wp.Runtime.EnableServerStreaming`). |
| `MAX_OBJECTS` | `2162688` | Unreal Engine UObject limit (`gc.MaxObjectsInEditor`). |
| `SERVER_TIMEOUT` | `30` | Network driver and game session disconnect timeout in seconds (alias: `TIMEOUT`). |
| `DISABLE_SEASONAL_EVENTS` | `false` | When `true`, passes `-DisableSeasonalEvents` (disables FICS*MAS). |
| `ALLOW_INSECURE_LOCAL_ACCESS` | `true` | Allows unauthenticated `127.0.0.1` access to `QueryServerState` for portal telemetry. |
| `MULTIHOME` | `""` | Binds server listener to a specific network interface IP (`-multihome`). |
| `EXTRA_SERVER_ARGS` | `""` | Additional arguments passed directly to `FactoryServer.sh`. |
| `PUID` / `PGID` | `1000` / `1000` | User and group IDs for file permissions under `/config`. |

### SteamCMD & Engine Updates

| Variable | Default | Description |
| :--- | :--- | :--- |
| `STEAM_BRANCH` | `public` | Steam branch to track (`public`, `experimental`, or private beta name). |
| `STEAM_UPDATE_ON_START` | `true` | Compares local vs remote Steam build ID on startup and downloads updates if needed. |
| `STEAM_VALIDATE` | `false` | When `true`, forces SteamCMD to validate game cache integrity on startup. |
| `STEAM_BETA_PASSWORD` | `""` | Password for protected/private Steam beta branches. |
| `BACKUP_KEEP_COUNT` | `5` | Maximum number of pre-update save snapshots to keep under `/config/backups`. |

### Declarative Mod Management (`ficsit-cli`)

| Variable | Default | Description |
| :--- | :--- | :--- |
| `MODS_ENABLED` | `false` | Enables declarative mod management via `ficsit-cli` during container boot. |
| `MODS_STRICT_MODE` | `true` | **World-Protection Gate**: aborts launch if any mod fails to resolve or is missing its plugin. |
| `MODS_PROFILE` | `default` | Name of the active `ficsit-cli` mod profile. |
| `MODS_UPDATE_POLICY` | `always` | `always` updates mods to the highest SemVer match; `locked` retains installed versions. |
| `MODS_LIST` | `""` | Multi-line or comma-separated mod list (e.g. `InfiniteNudge, SmartFoundations@^2.0.0`). |
| `MODS_CLIENT_ONLY` | `""` | Client-only QoL mods included in `.smmprofile` export, but skipped on the server. |
| `MODS_FILE` | `/config/mods.json` | Path to JSON-formatted declarative mod specifications file. |

### Save, Mod & Blueprint Web Portal

| Variable | Default | Description |
| :--- | :--- | :--- |
| `PORTAL_PORT` | `8081` | HTTP port the web portal listens on (alias: `PORT`). Default `8081` avoids collision with FRM on `8080`. |
| `PORTAL_TITLE` | `SERVER_NAME` | Optional web portal header title override. |
| `PORTAL_ADMIN_PASSWORD` | `""` | Optional password for `/admin` control center (alias: `ADMIN_PASSWORD`). When unset, `/admin` operates in open-access mode. |
| `SERVER_NAME` | `MODS_PROFILE` | Default server display name in portal header. |
| `SMM_PROFILE_NAME` | `MODS_PROFILE` | Profile name stamped inside exported `.smmprofile` download files (alias: `MODS_PROFILE`). |
| `SERVER_API_URL` | `https://127.0.0.1:7777` | HTTPS URL of Satisfactory Dedicated Server's API. |
| `SERVER_API_TOKEN` | `""` | Optional admin API bearer token (not required when `ALLOW_INSECURE_LOCAL_ACCESS=true`). |
| `FRM_API_URL` | `""` | Optional HTTP URL for Ficsit Remote Monitoring mod (default inside server: `http://127.0.0.1:8080`). |
| `MAX_PLAYERS` | `8` | Maximum players displayed in the web UI header and metrics. |
| `SAVES_DIR` | `/config/saved/server` | Directory containing `.sav` save files. |
| `BLUEPRINTS_DIR` | `/config/saved/blueprints` | Directory containing `.sbp` and `.sbpcfg` blueprint files. |
| `GAMEFILES_DIR` | `/config/gamefiles` | Root directory containing the installed Satisfactory game files. |
| `LOG_FILE` | `.../FactoryGame.log` | Path to `FactoryGame.log` for real-time pioneer join/leave tracking. |

### TLS Certificates & Notifications

| Variable | Default | Description |
| :--- | :--- | :--- |
| `TLS_CERT_DIR` | `/certs` | Directory checked on boot for `tls.crt`/`tls.key` or `cert_chain.pem`/`private_key.pem`. |
| `DISCORD_WEBHOOK_URL` | `""` | Discord Webhook URL for automated game and mod update notifications. |

### Environment Variable Aliases

For convenience and compatibility with community setups, the following common environment variable aliases are also supported:

| Alias | Primary Variable | Description |
| :--- | :--- | :--- |
| `SERVERGAMEPORT` | `SERVER_GAME_PORT` | Mapped automatically |
| `SERVERMESSAGINGPORT` | `SERVER_MESSAGING_PORT` | Mapped automatically |
| `MAXPLAYERS` | `MAX_PLAYERS` | Mapped automatically |
| `MAXTICKRATE` | `MAX_TICK_RATE` | Mapped automatically |
| `AUTOSAVENUM` | `AUTOSAVE_NUM` | Mapped automatically |
| `MAXOBJECTS` | `MAX_OBJECTS` | Mapped automatically |
| `SERVERSTREAMING` | `SERVER_STREAMING` | Accepts `true`/`false` |
| `DISABLESEASONALEVENTS` | `DISABLE_SEASONAL_EVENTS` | Accepts `true`/`false` |
| `STEAMBETA=true` | `STEAM_BRANCH=experimental` | Mapped automatically |
| `SKIPUPDATE=true` | `STEAM_UPDATE_ON_START=false` | Mapped automatically |

---

## 🌐 Web Portal & Prometheus Telemetry

### Portal Routes & Endpoints

| Route | Method | Description |
| :--- | :--- | :--- |
| `/` | `GET` | Web dashboard (online players, tech tier, playtime, save files, blueprints). |
| `/admin` | `GET` | Admin Control Center (save manager, MAM research, death crate loot radar). |
| `/admin/login` | `GET, POST` | Admin authentication login portal. |
| `/admin/logout` | `POST` | Log out and clear session cookie. |
| `/admin/api/upload-save` | `POST` | Upload and install a `.sav` file directly to the dedicated server. |
| `/admin/api/load-game` | `POST` | Trigger dedicated server HTTPS API to load a save file. |
| `/favicon.svg` | `GET` | FICSIT isometric chamfered chevron SVG icon. |
| `/favicon.ico` | `GET` | Favicon endpoint (served with `image/svg+xml`). |
| `/healthz` | `GET` | Portal liveness/readiness probe (`200 OK`). |
| `/metrics` | `GET` | Standard Prometheus metric scraper endpoint. |
| `/download?save=<name>` | `GET` | Direct download of a `.sav` game save file. |
| `/profile` | `GET` | Download `.smmprofile` for 1-click import into Desktop Satisfactory Mod Manager. |
| `/blueprints/download?name=<name>` | `GET` | Download individual blueprint zip (`.sbp` + `.sbpcfg`). |
| `/blueprints/download-all` | `GET` | Stream zip bundle of all blueprints on the server. |

### Exported Prometheus Metrics

The portal sidecar exports Prometheus metrics ready for scraping by Prometheus, Prometheus Operator, or Grafana Alloy:

```promql
# Server Performance & Health
satisfactory_server_tps              # Server tick rate (target: 30)
satisfactory_tech_tier                # Current factory milestone tier (0-9)
satisfactory_players_connected        # Active pioneers currently online
satisfactory_players_max              # Server player capacity
satisfactory_playtime_seconds         # Total playtime in current save session

# Content & Mod Counts
satisfactory_save_size_bytes          # Uncompressed size of the latest save file
satisfactory_blueprint_count          # Total number of uploaded blueprint designs
satisfactory_mod_count                # Number of active mods loaded on the server
```

---

## 🛠️ Local Development

This repository uses [`mise`](https://mise.jdx.dev) and [`just`](https://github.com/casey/just):

```bash
mise install
just test                  # Run Go unit tests with race detector
just build                 # Compile bin/ficsit-ctl and bin/ficsit-portal
just chart-lint            # Lint charts/satisfactory
just docker-build-server   # Build satisfactory-server container image
just docker-build-portal   # Build satisfactory-portal container image
```
