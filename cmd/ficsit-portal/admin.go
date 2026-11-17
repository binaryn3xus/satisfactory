package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/binaryn3xus/satisfactory/internal/saves"
)

func (s *PortalServer) adminToken() string {
	if s.cfg.AdminPassword == "" {
		return ""
	}
	h := sha256.Sum256([]byte("ficsit-admin-salt:" + s.cfg.AdminPassword))
	return fmt.Sprintf("%x", h)
}

func (s *PortalServer) isAdminAuthenticated(r *http.Request) bool {
	if s.cfg.AdminPassword == "" {
		return true // Open admin mode when no password configured
	}

	// Check session cookie
	cookie, err := r.Cookie("ficsit_admin_session")
	if err == nil && cookie.Value == s.adminToken() {
		return true
	}

	// Check Authorization header
	authHeader := r.Header.Get("Authorization")
	if authHeader != "" {
		token := strings.TrimPrefix(authHeader, "Bearer ")
		if token == s.cfg.AdminPassword || token == s.adminToken() {
			return true
		}
	}

	return false
}

func (s *PortalServer) handleAdminLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		pwd := r.FormValue("password")
		if pwd == s.cfg.AdminPassword {
			http.SetCookie(w, &http.Cookie{
				Name:     "ficsit_admin_session",
				Value:    s.adminToken(),
				Path:     "/",
				HttpOnly: true,
				MaxAge:   86400 * 7,
				SameSite: http.SameSiteLaxMode,
			})
			http.Redirect(w, r, "/admin", http.StatusSeeOther)
			return
		}

		s.renderAdminLoginPage(w, "Invalid administrator password. Access denied.")
		return
	}

	if s.isAdminAuthenticated(r) {
		http.Redirect(w, r, "/admin", http.StatusSeeOther)
		return
	}

	s.renderAdminLoginPage(w, "")
}

func (s *PortalServer) handleAdminLogout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     "ficsit_admin_session",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
	})
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *PortalServer) renderAdminLoginPage(w http.ResponseWriter, errorMsg string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	errBanner := ""
	if errorMsg != "" {
		errBanner = fmt.Sprintf(`<div class="login-error">⚠️ %s</div>`, html.EscapeString(errorMsg))
	}

	fmt.Fprintf(w, `<!DOCTYPE html>
<html lang="en">
<head>
	<meta charset="UTF-8">
	<meta name="viewport" content="width=device-width, initial-scale=1.0">
	<title>Admin Login - %s Portal</title>
	<link rel="icon" type="image/svg+xml" href="/favicon.svg">
	<style>
		:root {
			--bg-main: #0b0f16;
			--bg-surface: #131a24;
			--accent-primary: #fa8029;
			--border: rgba(255, 255, 255, 0.1);
			--text-main: #e2e8f0;
			--text-muted: #94a3b8;
		}
		* { box-sizing: border-box; }
		body {
			background-color: var(--bg-main);
			color: var(--text-main);
			font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
			display: flex;
			align-items: center;
			justify-content: center;
			min-height: 100vh;
			margin: 0;
			padding: 1rem;
		}
		.login-card {
			background: var(--bg-surface);
			border: 1px solid var(--border);
			border-radius: 12px;
			padding: 2.5rem 2rem;
			width: 100%%;
			max-width: 420px;
			box-shadow: 0 10px 25px rgba(0,0,0,0.5);
			text-align: center;
		}
		.logo { font-size: 3rem; margin-bottom: 0.5rem; }
		h1 { margin: 0 0 0.5rem 0; font-size: 1.4rem; color: #fff; }
		p { color: var(--text-muted); font-size: 0.88rem; margin: 0 0 1.5rem 0; }
		.login-error {
			background: rgba(239, 68, 68, 0.15);
			border: 1px solid rgba(239, 68, 68, 0.4);
			color: #f87171;
			padding: 0.75rem;
			border-radius: 6px;
			font-size: 0.85rem;
			margin-bottom: 1.25rem;
			text-align: left;
		}
		input[type="password"] {
			width: 100%%;
			padding: 0.75rem 1rem;
			background: #0f141c;
			border: 1px solid var(--border);
			border-radius: 6px;
			color: #fff;
			font-size: 1rem;
			margin-bottom: 1.25rem;
		}
		input[type="password"]:focus {
			border-color: var(--accent-primary);
			outline: none;
		}
		.btn-submit {
			width: 100%%;
			padding: 0.75rem;
			background: var(--accent-primary);
			border: none;
			border-radius: 6px;
			color: #fff;
			font-size: 1rem;
			font-weight: 600;
			cursor: pointer;
			transition: background 0.15s;
		}
		.btn-submit:hover { background: #e06c19; }
		.back-link {
			display: inline-block;
			margin-top: 1.5rem;
			color: var(--text-muted);
			text-decoration: none;
			font-size: 0.85rem;
		}
		.back-link:hover { color: #fff; }
	</style>
</head>
<body>
	<div class="login-card">
		<div class="logo">⚙️</div>
		<h1>FICSIT Admin Access</h1>
		<p>Enter the administrator password to unlock save repairs and privileged world operations.</p>
		%s
		<form method="POST" action="/admin/login">
			<input type="password" name="password" placeholder="Admin Password" autofocus required autocomplete="current-password">
			<button type="submit" class="btn-submit">🔒 Unlock Admin Center</button>
		</form>
		<a href="/" class="back-link">⬅ Return to Public Portal</a>
	</div>
</body>
</html>`, html.EscapeString(s.cfg.ServerName), errBanner)
}

func (s *PortalServer) handleAdminUploadSave(w http.ResponseWriter, r *http.Request) {
	setCORSHeaders(w)
	if !s.isAdminAuthenticated(r) {
		http.Error(w, `{"error":"Unauthorized"}`, http.StatusUnauthorized)
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"Method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	filename := r.URL.Query().Get("filename")
	if filename == "" {
		http.Error(w, `{"error":"Missing filename parameter"}`, http.StatusBadRequest)
		return
	}

	filename = filepath.Base(filename)
	if !strings.HasSuffix(strings.ToLower(filename), ".sav") {
		http.Error(w, `{"error":"Invalid filename. Must end in .sav"}`, http.StatusBadRequest)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 500*1024*1024))
	if err != nil || len(body) < 100 {
		http.Error(w, `{"error":"Failed to read save body"}`, http.StatusBadRequest)
		return
	}

	targetPath := filepath.Join(s.cfg.SavesDir, filename)
	baseName := strings.TrimSuffix(filename, filepath.Ext(filename))
	// Satisfactory in-game date format: DDMMYY-HHmmss (e.g. 161126-154236)
	backupName := fmt.Sprintf("%s-%s-backup.sav", baseName, time.Now().Format("020106-150405"))
	backupPath := filepath.Join(s.cfg.SavesDir, backupName)

	// Create backup of existing file if present (saved as loadable .sav instead of unreadable .bak)
	var backupCreated string
	if _, err := os.Stat(targetPath); err == nil {
		origBytes, err := os.ReadFile(targetPath)
		if err == nil {
			if err := os.WriteFile(backupPath, origBytes, 0644); err == nil {
				backupCreated = backupName
			}
		}
	}

	// Write updated file
	if err := os.WriteFile(targetPath, body, 0644); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": fmt.Sprintf("Failed to write save file: %v", err),
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success":  true,
		"filename": filename,
		"message":  fmt.Sprintf("Successfully applied %s to server!", filename),
		"backup":   backupCreated,
		"size":     len(body),
	})
}

func (s *PortalServer) handleAdmin(w http.ResponseWriter, r *http.Request, sendBody bool) {
	if !s.isAdminAuthenticated(r) {
		s.handleAdminLogin(w, r)
		return
	}

	saveList, _ := saves.ListSaves(s.cfg.SavesDir)
	latestName := ""
	if len(saveList) > 0 {
		latestName = saveList[0].Name
	}

	var saveOptions strings.Builder
	for _, sv := range saveList {
		selected := ""
		if sv.Name == latestName {
			selected = "selected"
		}
		fmt.Fprintf(&saveOptions, `<option value="/%s" data-name="%s" %s>%s (%s, %s)</option>`,
			html.EscapeString(sv.Name),
			html.EscapeString(sv.Name),
			selected,
			html.EscapeString(sv.Name),
			html.EscapeString(sv.SessionName),
			html.EscapeString(sv.FormattedPlaytime),
		)
	}

	logoutBtn := ""
	if s.cfg.AdminPassword != "" {
		logoutBtn = `<a href="/admin/logout" class="btn btn-secondary" style="font-size:0.82rem; padding:0.35rem 0.75rem; text-decoration:none;">Logout</a>`
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, `<!DOCTYPE html>
<html lang="en">
<head>
	<meta charset="UTF-8">
	<meta name="viewport" content="width=device-width, initial-scale=1.0">
	<title>Admin Control Center - %s</title>
	<link rel="icon" type="image/svg+xml" href="/favicon.svg">
	<script src="/inspector-v4.js"></script>
	<style>
		:root {
			--bg-main: #0b0f16;
			--bg-surface: #131a24;
			--bg-card: #0f141c;
			--border: rgba(255, 255, 255, 0.1);
			--text-main: #e2e8f0;
			--text-muted: #94a3b8;
			--accent-primary: #fa8029;
			--accent-hover: #e06c19;
			--accent-blue: #38bdf8;
		}
		* { box-sizing: border-box; }
		body {
			background-color: var(--bg-main);
			color: var(--text-main);
			font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
			margin: 0;
			padding: 0;
			min-height: 100vh;
		}
		.admin-header {
			background: #111722;
			border-bottom: 1px solid var(--border);
			padding: 0.85rem 1.5rem;
			display: flex;
			justify-content: space-between;
			align-items: center;
			flex-wrap: wrap;
			gap: 1rem;
		}
		.admin-title {
			display: flex;
			align-items: center;
			gap: 0.6rem;
			font-size: 1.15rem;
			font-weight: 700;
			color: #fff;
		}
		.admin-nav {
			display: flex;
			align-items: center;
			gap: 0.75rem;
		}
		.btn {
			display: inline-flex;
			align-items: center;
			gap: 0.35rem;
			padding: 0.45rem 0.85rem;
			border-radius: 6px;
			text-decoration: none;
			font-weight: 500;
			font-size: 0.85rem;
			cursor: pointer;
			border: none;
			transition: background 0.15s;
		}
		.btn-primary { background: var(--accent-primary); color: #fff; }
		.btn-primary:hover { background: var(--accent-hover); }
		.btn-secondary { background: #374151; color: #fff; }
		.btn-secondary:hover { background: #4b5563; }
		
		.admin-container {
			max-width: 1400px;
			margin: 1.5rem auto;
			padding: 0 1.25rem;
		}
		.save-picker-bar {
			background: var(--bg-surface);
			border: 1px solid var(--border);
			border-radius: 8px;
			padding: 1rem 1.25rem;
			margin-bottom: 1.5rem;
			display: flex;
			justify-content: space-between;
			align-items: center;
			flex-wrap: wrap;
			gap: 1rem;
		}
		.save-select {
			padding: 0.5rem 0.75rem;
			background: var(--bg-card);
			color: #fff;
			border: 1px solid var(--border);
			border-radius: 6px;
			font-size: 0.9rem;
			min-width: 320px;
		}
		#admin-workspace {
			min-height: 400px;
		}
	</style>
</head>
<body>
	<header class="admin-header">
		<div class="admin-title">
			<span>🛠️ FICSIT Admin Control Center</span>
			<span style="font-size:0.78rem; background:rgba(250,128,41,0.2); color:var(--accent-primary); border:1px solid rgba(250,128,41,0.4); padding:2px 8px; border-radius:12px;">Privileged</span>
		</div>
		<div class="admin-nav">
			<span id="admin-changes-badge" style="display:none; background:var(--accent-primary); color:#fff; font-size:0.75rem; font-weight:700; padding:3px 9px; border-radius:12px;"></span>
			<button class="btn btn-primary" onclick="FicsitAdmin.saveToServer()" id="admin-save-btn">💾 Save to Server</button>
			<label class="btn btn-secondary" style="cursor:pointer; margin:0;" title="Import and upload a local .sav file directly to server">
				⬆️ Import .sav
				<input type="file" accept=".sav" style="display:none;" id="admin-import-input" onchange="if(this.files.length){FicsitAdmin.importSave(this.files[0]); this.value='';}">
			</label>
			<button class="btn btn-secondary" onclick="FicsitAdmin.downloadSave()">⬇️ Export .sav</button>
			<a href="/" class="btn btn-secondary">⬅ View Public Portal</a>
			%s
		</div>
	</header>

	<div class="admin-container">
		<div class="save-picker-bar">
			<div style="display:flex; align-items:center; gap:0.75rem; flex-wrap:wrap;">
				<strong>Active Save File:</strong>
				<select id="admin-save-select" class="save-select" onchange="loadSelectedSave()">
					%s
				</select>
				<button class="btn btn-secondary" onclick="loadSelectedSave()">🔄 Reload Save</button>
			</div>
			<div style="font-size:0.85rem; color:var(--text-muted);">
				Visual Inventory Manager &bull; Save Doctor &bull; Direct Server Commit
			</div>
		</div>

		<div id="admin-workspace">
			<div style="text-align:center; padding:4rem; color:var(--text-muted);">
				<div class="inspector-spinner"></div>
				<p style="margin-top:1rem;">Initializing FICSIT Save Inspector &amp; Admin Suite...</p>
			</div>
		</div>
	</div>

	<script>
		function loadSelectedSave() {
			const sel = document.getElementById('admin-save-select');
			if (!sel) return;
			const opt = sel.options[sel.selectedIndex];
			const url = opt.value;
			const name = opt.getAttribute('data-name');
			if (window.FicsitAdmin && window.FicsitAdmin.init) {
				window.FicsitAdmin.init(url, name, 'admin-workspace');
			}
		}

		window.addEventListener('DOMContentLoaded', () => {
			loadSelectedSave();
		});
	</script>
</body>
</html>`,
		html.EscapeString(s.cfg.ServerName),
		logoutBtn,
		saveOptions.String(),
	)
}
