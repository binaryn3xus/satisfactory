# Satisfactory Dedicated Server, Mod Manager & Portal

A container-native **Satisfactory 1.0+ Dedicated Server** suite written in Go, featuring declarative **`ficsit-cli` Mod Management**, a **Strict World-Protection Gate**, a lightweight **Save / Mod / Blueprint Web Portal** sidecar, first-class **Docker Compose** support, and a reusable **OCI Helm Chart**.

---

## 📦 Published Artifacts

All artifacts are built and published from this single repository to GitHub Container Registry (`ghcr.io`):

| Artifact | Registry URI | Description |
| :--- | :--- | :--- |
| **Server Image** | `ghcr.io/binaryn3xus/satisfactory-server` | SteamCMD + `ficsit-cli` + `ficsit-ctl` server lifecycle controller |
| **Portal Image** | `ghcr.io/binaryn3xus/satisfactory-portal` | Save downloader, SCIM links, live `.smmprofile` export, Blueprints `.zip` streamer, pioneer tracker & Prometheus `/metrics` |
| **Helm Chart** | `oci://ghcr.io/binaryn3xus/charts/satisfactory` | Flexible Kubernetes Helm chart supporting Gateway API (`HTTPRoute`), `Ingress`, `cert-manager`, and NFS backups |

---

## ✨ Key Features

- **Declarative Mod Management (`ficsit-cli`):**
  Specify mods via `MODS_LIST` (e.g. `NoFuel, SmartFoundations@^2.0.0`) or `/config/mods.json`. Supports `latest` (mapped to `>=0.0.0`) and full SemVer ranges, with `always` (auto-upgrade to newest compatible release) or `locked` update policies.
- **Strict World-Protection Gate (`MODS_STRICT_MODE=true`):**
  Before `FactoryServer.sh` is ever executed, `ficsit-ctl` verifies that `ficsit-cli apply` succeeded, `SML.uplugin` is present, and every declared mod is in the resolved lockfile and on disk. If a new Satisfactory patch drops before a mod updates, startup halts cleanly so Unreal Engine never loads a modded save without its mods.
- **Smart SteamCMD Updates & Pre-Update Save Snapshots:**
  Compares the local `appmanifest_1690800.acf` build ID against Steam's remote build ID on startup—skipping redundant SteamCMD downloads when already up to date, and automatically snapshotting `.sav` files to `/config/backups/pre-update-CL-<old>` (with retention rotation) before applying a game update.
- **Built-in TLS Certificate Sync & Pre-Shutdown Save Hook:**
  Automatically syncs mounted TLS certificates from `/certs` into `FactoryGame/Certificates` (no Kubernetes `initContainer` needed) and calls Satisfactory's 1.0 HTTPS API (`SaveGame`) before shutting down when `SERVER_API_TOKEN` is configured.
- **Save, Mod & Blueprint Web Portal (`satisfactory-portal`):**
  - **Live Status & Pioneer Tracker:** Combines the Satisfactory 1.0 HTTPS API (`QueryServerState`) with a real-time `FactoryGame.log` parser to show **Who's Online** and **Recently Seen** on both Vanilla and Modded servers (and automatically enriches with ping/power telemetry if `FicsitRemoteMonitoring` is active).
  - **Binary `.sav` Header Inspector:** Reads uncompressed `.sav` headers to display Session Name, Total Playtime (`76h 06m`), and Build Changelist alongside one-click **SCIM Interactive Map** links.
  - **One-Click `.smmprofile` Export:** Dynamically builds a live `.smmprofile` from the server's lockfile (plus optional `MODS_CLIENT_ONLY` QoL mods) for one-click import into Desktop Satisfactory Mod Manager.
  - **Blueprints Browser & `.zip` Streamer:** Parses `.sbp` + `.sbpcfg` descriptions and streams single or full-server blueprint `.zip` bundles.
  - **Prometheus `/metrics`:** Exposes tick rate, connected players, tech tier, mod count, save size, playtime, and blueprint count.

---

## 🐳 Quickstart: Docker Compose

```yaml
services:
  satisfactory-server:
    image: ghcr.io/binaryn3xus/satisfactory-server:latest
    container_name: satisfactory-server
    restart: unless-stopped
    stop_grace_period: 90s
    ports:
      - "7777:7777/udp"
      - "7777:7777/tcp"
      - "8888:8888/tcp"
    environment:
      SERVER_NAME: "UNSC-Section-III"
      MAX_PLAYERS: "8"
      MAX_TICK_RATE: "30"
      STEAM_UPDATE_ON_START: "true"
      MODS_ENABLED: "true"
      MODS_STRICT_MODE: "true"
      MODS_PROFILE: "UNSC-Section-III"
      MODS_UPDATE_POLICY: "always"
      MODS_LIST: |
        NoFuel
        SmartFoundations@^2.0.0
    volumes:
      - ./data:/config

  satisfactory-portal:
    image: ghcr.io/binaryn3xus/satisfactory-portal:latest
    container_name: satisfactory-portal
    restart: unless-stopped
    ports:
      - "8080:8080/tcp"
    environment:
      SERVER_NAME: "UNSC-Section-III"
      SERVER_API_URL: "https://satisfactory-server:7777"
      SMM_PROFILE_NAME: "UNSC-Section-III"
    volumes:
      - ./data:/config:ro
```

---

## ☸️ Kubernetes Helm Chart (`charts/satisfactory`)

### Example Flux `HelmRelease` (Modded Server)

```yaml
apiVersion: helm.toolkit.fluxcd.io/v2
kind: HelmRelease
metadata:
  name: satisfactory-modded
spec:
  interval: 30m
  chartRef:
    kind: OCIRepository
    name: satisfactory
  values:
    nodeSelector:
      sku: gaming
    tolerations:
      - key: sku
        operator: Equal
        value: gaming
        effect: NoSchedule

    server:
      config:
        serverName: "UNSC-Section-III"
        gamePort: 7778
        messagingPort: 8889
        maxPlayers: 8
        maxTickRate: 30

    mods:
      enabled: true
      strictMode: true
      profileName: "UNSC-Section-III"
      updatePolicy: "always"
      items:
        - ref: NoFuel
          version: latest

    portal:
      enabled: true
      route:
        enabled: true
        parentRefs:
          - name: envoy-internal
            namespace: network
            sectionName: https
        hostnames:
          - satisfactory-modded-saves.unscfleet.com

    service:
      game:
        type: LoadBalancer
        annotations:
          lbipam.cilium.io/ips: "10.0.30.41"
          external-dns.alpha.kubernetes.io/hostname: "satisfactory-modded.unscfleet.com"

    tls:
      certManager:
        enabled: true
        commonName: satisfactory-modded.unscfleet.com
        dnsNames:
          - satisfactory-modded.unscfleet.com
        issuerRef:
          name: unscfleet-ca-issuer
          kind: ClusterIssuer

    backup:
      enabled: true
      schedule: "0 */6 * * *"
      nfs:
        server: "10.0.30.4"
        path: "/volume1/Media"
        targetSubDir: "Gaming/Satisfactory-Modded"
```

---

## 🛠️ Local Development

This repository uses [`mise`](https://mise.jdx.dev) and [`just`](https://github.com/casey/just):

```bash
mise install
just test                  # Run Go unit tests with race detector
just build                 # Compile bin/ficsit-ctl and bin/ficsit-portal
just chart-lint            # Lint charts/satisfactory
just docker-build-server   # Build satisfactory-server image
just docker-build-portal   # Build satisfactory-portal image
```
