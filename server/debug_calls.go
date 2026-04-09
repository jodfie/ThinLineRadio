package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// CallsDebugHandler serves an admin-only HTML page at /calls showing
// recent calls with full metadata, audio playback, and duplicate flags.
func (controller *Controller) CallsDebugHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	limit := 200
	noLimit := r.URL.Query().Get("limit") == "all"
	if !noLimit {
		if v := r.URL.Query().Get("limit"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 100000 {
				limit = n
			}
		}
	}
	systemFilter := r.URL.Query().Get("system")
	tgFilter := r.URL.Query().Get("tg")
	dupFilter := r.URL.Query().Get("dup") // "only" or "hide"

	where := []string{}
	if systemFilter != "" {
		where = append(where, fmt.Sprintf(`"systemRef" = %s`, systemFilter))
	}
	if tgFilter != "" {
		where = append(where, fmt.Sprintf(`"talkgroupRef" = %s`, tgFilter))
	}
	if dupFilter == "only" {
		where = append(where, `"isDuplicate" = true`)
	} else if dupFilter == "hide" {
		where = append(where, `"isDuplicate" = false`)
	}

	whereClause := ""
	if len(where) > 0 {
		whereClause = "WHERE " + strings.Join(where, " AND ")
	}

	limitClause := fmt.Sprintf("LIMIT %d", limit)
	if noLimit {
		limitClause = ""
	}

	query := fmt.Sprintf(`
		SELECT "callId", "systemRef", "talkgroupRef", "timestamp", "audioDuration",
		       octet_length("audio"), "isDuplicate", "audioHash", "verifiedDuplicate"
		FROM "calls"
		%s
		ORDER BY "callId" DESC
		%s`, whereClause, limitClause)

	rows, err := controller.Database.Sql.QueryContext(ctx, query)
	if err != nil {
		http.Error(w, "DB error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type row struct {
		ID                uint64
		SystemRef         int
		TalkgroupRef      int
		Timestamp         int64
		Duration          float64
		Bytes             int
		IsDuplicate       bool
		AudioHash         string
		VerifiedDuplicate *bool // nil = unreviewed
	}
	var calls []row
	for rows.Next() {
		var c row
		if err := rows.Scan(&c.ID, &c.SystemRef, &c.TalkgroupRef, &c.Timestamp,
			&c.Duration, &c.Bytes, &c.IsDuplicate, &c.AudioHash, &c.VerifiedDuplicate); err == nil {
			calls = append(calls, c)
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>Calls Debug</title>
<style>
*{box-sizing:border-box;margin:0;padding:0}
body{background:#0f1117;color:#e2e8f0;font-family:'SF Mono',monospace;font-size:13px}
header{background:#1a1d2e;border-bottom:1px solid #2d3148;padding:16px 24px;display:flex;align-items:center;gap:16px;flex-wrap:wrap}
header h1{font-size:16px;font-weight:600;color:#a78bfa;letter-spacing:.05em}
.filters{display:flex;gap:8px;flex-wrap:wrap;margin-left:auto}
.filters input,.filters select{background:#0f1117;border:1px solid #2d3148;color:#e2e8f0;padding:4px 8px;border-radius:4px;font-family:inherit;font-size:12px}
.filters button{background:#7c3aed;border:none;color:#fff;padding:4px 12px;border-radius:4px;cursor:pointer;font-size:12px}
.stats{padding:8px 24px;background:#13162a;border-bottom:1px solid #2d3148;font-size:11px;color:#64748b}
.stats span{color:#a78bfa}
table{width:100%%;border-collapse:collapse}
th{background:#1a1d2e;padding:8px 12px;text-align:left;font-weight:500;color:#94a3b8;font-size:11px;letter-spacing:.08em;text-transform:uppercase;position:sticky;top:0;z-index:1}
td{padding:7px 12px;border-bottom:1px solid #1a1d2e;vertical-align:middle}
tr:hover td{background:#13162a}
tr.dup td{background:#1f1010}
tr.dup:hover td{background:#2a1515}
tr.verified-dup td{background:#1a1500}
tr.verified-ok td{background:#001a0a}
.badge{display:inline-block;padding:2px 7px;border-radius:3px;font-size:10px;font-weight:600;letter-spacing:.05em}
.badge.dup{background:#7f1d1d;color:#fca5a5}
.badge.ok{background:#14532d;color:#86efac}
.ts{color:#64748b;font-size:11px}
.dur{color:#93c5fd}
.sz{color:#94a3b8;font-size:11px}
.sysref{color:#c4b5fd}
.tgref{color:#67e8f9}
audio{height:28px;width:200px}
.ref-link{color:#a78bfa;text-decoration:none;font-size:11px}
.ref-link:hover{text-decoration:underline}
.vbtn{display:inline-flex;border:1px solid #2d3148;border-radius:4px;overflow:hidden;margin-top:4px}
.vbtn button{background:#0f1117;color:#64748b;border:none;padding:3px 8px;font-size:11px;cursor:pointer;transition:background .15s,color .15s}
.vbtn button:hover{background:#1a1d2e;color:#e2e8f0}
.vbtn button.active-dup{background:#7f1d1d;color:#fca5a5;font-weight:700}
.vbtn button.active-ok{background:#14532d;color:#86efac;font-weight:700}
.vbtn button.active-none{background:#1e293b;color:#94a3b8;font-weight:700}
.saving{opacity:.5;pointer-events:none}
</style>
</head>
<body>
<header>
  <h1>📡 Calls Debug</h1>
  <form class="filters" method="get">
    <input name="system" placeholder="sysRef" value="%s" style="width:70px">
    <input name="tg" placeholder="tgRef" value="%s" style="width:90px">
    <select name="dup">
      <option value="">All calls</option>
      <option value="hide" %s>Hide duplicates</option>
      <option value="only" %s>Duplicates only</option>
    </select>
    <select name="limit">
      <option value="50" %s>50</option>
      <option value="200" %s>200</option>
      <option value="500" %s>500</option>
      <option value="1000" %s>1000</option>
      <option value="5000" %s>5000</option>
      <option value="all" %s>No limit</option>
    </select>
    <button type="submit">Filter</button>
  </form>
</header>
<div class="stats">Showing <span>%d</span> calls — newest first</div>
<table>
<thead><tr>
  <th>ID</th><th>Time</th><th>Sys</th><th>TG</th>
  <th>Dur</th><th>Size</th><th>System flag</th><th>Your verdict</th><th>Audio</th>
</tr></thead>
<tbody>
`,
		systemFilter, tgFilter,
		boolAttr(dupFilter == "hide", "selected"),
		boolAttr(dupFilter == "only", "selected"),
		boolAttr(limit == 50 && !noLimit, "selected"),
		boolAttr(limit == 200 && !noLimit, "selected"),
		boolAttr(limit == 500 && !noLimit, "selected"),
		boolAttr(limit == 1000 && !noLimit, "selected"),
		boolAttr(limit == 5000 && !noLimit, "selected"),
		boolAttr(noLimit, "selected"),
		len(calls))

	for _, c := range calls {
		ts := time.UnixMilli(c.Timestamp)

		rowClass := ""
		if c.VerifiedDuplicate != nil {
			if *c.VerifiedDuplicate {
				rowClass = `class="verified-dup"`
			} else {
				rowClass = `class="verified-ok"`
			}
		} else if c.IsDuplicate {
			rowClass = `class="dup"`
		}

		sysBadge := `<span class="badge ok">OK</span>`
		if c.IsDuplicate {
			sysBadge = `<span class="badge dup">DUPLICATE</span>`
		}

		dupActive, okActive, noneActive := "", "", "active-none"
		if c.VerifiedDuplicate != nil {
			if *c.VerifiedDuplicate {
				dupActive, noneActive = "active-dup", ""
			} else {
				okActive, noneActive = "active-ok", ""
			}
		}

		fmt.Fprintf(w, `<tr %s id="%d">
  <td><a class="ref-link" href="#%d">%d</a></td>
  <td><span class="ts">%s</span><br><span class="ts" style="color:#475569">%d</span></td>
  <td class="sysref">%d</td>
  <td class="tgref">%d</td>
  <td class="dur">%.2fs</td>
  <td class="sz">%s</td>
  <td>%s</td>
  <td>
    <div class="vbtn" id="vb%d">
      <button class="%s" onclick="verify(%d,'duplicate',this)">✓ Dup</button>
      <button class="%s" onclick="verify(%d,'not_duplicate',this)">✗ Not Dup</button>
      <button class="%s" onclick="verify(%d,'unreviewed',this)">?</button>
    </div>
  </td>
  <td><audio controls preload="none" src="/calls/audio/%d" onplay="stopOthers(this)"></audio></td>
</tr>`,
			rowClass, c.ID, c.ID, c.ID,
			ts.In(time.Local).Format("15:04:05"),
			c.Timestamp,
			c.SystemRef, c.TalkgroupRef,
			c.Duration,
			formatBytes(c.Bytes),
			sysBadge+hashBadge(c.AudioHash),
			c.ID,
			dupActive, c.ID,
			okActive, c.ID,
			noneActive, c.ID,
			c.ID)
	}

	fmt.Fprintf(w, `</tbody></table>
<script>
var currentAudio = null;
function stopOthers(el) {
  if (currentAudio && currentAudio !== el) {
    currentAudio.pause();
    currentAudio.currentTime = 0;
  }
  currentAudio = el;
}

function verify(callId, status, btn) {
  var container = document.getElementById('vb' + callId);
  container.classList.add('saving');
  fetch('/calls/verify', {
    method: 'POST',
    headers: {'Content-Type': 'application/json'},
    body: JSON.stringify({callId: callId, status: status})
  }).then(function(r) {
    if (!r.ok) throw new Error('server error');
    var btns = container.querySelectorAll('button');
    btns[0].className = '';
    btns[1].className = '';
    btns[2].className = '';
    if (status === 'duplicate')     { btns[0].className = 'active-dup'; }
    else if (status === 'not_duplicate') { btns[1].className = 'active-ok'; }
    else                            { btns[2].className = 'active-none'; }
    var row = container.closest('tr');
    row.className = status === 'duplicate' ? 'verified-dup'
                  : status === 'not_duplicate' ? 'verified-ok' : '';
  }).catch(function(e) {
    alert('Save failed: ' + e.message);
  }).finally(function() {
    container.classList.remove('saving');
  });
}
</script>
</body></html>`)
}

// CallsAudioHandler serves raw audio for a single call at /calls/audio/{id}.
func (controller *Controller) CallsAudioHandler(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/calls/audio/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		http.NotFound(w, r)
		return
	}
	id, err := strconv.ParseUint(parts[0], 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var audio []byte
	var mime string
	query := fmt.Sprintf(`SELECT "audio", "audioMime" FROM "calls" WHERE "callId" = %d`, id)
	if err := controller.Database.Sql.QueryRowContext(ctx, query).Scan(&audio, &mime); err != nil {
		http.NotFound(w, r)
		return
	}

	if mime == "" {
		mime = "audio/mp4"
	}
	w.Header().Set("Content-Type", mime)
	w.Header().Set("Content-Length", strconv.Itoa(len(audio)))
	w.Header().Set("Accept-Ranges", "bytes")
	w.Write(audio)
}

func hashBadge(h string) string {
	if h == "" {
		return ` <span style="font-size:10px;color:#475569">no hash</span>`
	}
	return fmt.Sprintf(` <span style="font-size:10px;color:#334155;font-family:monospace" title="%s">%s…</span>`, h, h[:8])
}

func boolAttr(cond bool, attr string) string {
	if cond {
		return attr
	}
	return ""
}

// CallsVerifyHandler handles POST /calls/verify to save human review decisions.
// Body: {"callId": 123, "status": "duplicate"|"not_duplicate"|"unreviewed"}
func (controller *Controller) CallsVerifyHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var body struct {
		CallID uint64 `json:"callId"`
		Status string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	var val string
	switch body.Status {
	case "duplicate":
		val = "true"
	case "not_duplicate":
		val = "false"
	case "unreviewed":
		val = "NULL"
	default:
		http.Error(w, "invalid status", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	query := fmt.Sprintf(`UPDATE "calls" SET "verifiedDuplicate" = %s WHERE "callId" = %d`, val, body.CallID)
	if _, err := controller.Database.Sql.ExecContext(ctx, query); err != nil {
		http.Error(w, "db error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"ok": "true"})
}

func formatBytes(b int) string {
	if b < 1024 {
		return fmt.Sprintf("%dB", b)
	}
	if b < 1024*1024 {
		return fmt.Sprintf("%.1fKB", float64(b)/1024)
	}
	return fmt.Sprintf("%.1fMB", float64(b)/1024/1024)
}
