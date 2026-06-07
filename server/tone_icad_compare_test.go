// Copyright (C) 2025 Thinline Dynamic Solutions
//
// Compare TLR Discover + auto-learn vs icad_tone_detection on exported prod audio.
// Skips when icad is not installed: pip install icad_tone_detection

package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

type icadBridgeResult struct {
	Path     string          `json:"path"`
	TwoTone  []map[string]any `json:"two_tone"`
	Long     []map[string]any `json:"long"`
	Tones    []Tone          `json:"tones"`
}

func icadBridgeScriptPath() string {
	return filepath.Join("scripts", "icad_tone_bridge.py")
}

func icadAvailable(t *testing.T) bool {
	t.Helper()
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not in PATH")
	}
	cmd := exec.Command("python3", "-c", "import icad_tone_detection")
	if err := cmd.Run(); err != nil {
		t.Skip("icad_tone_detection not installed (pip install icad_tone_detection)")
	}
	script := icadBridgeScriptPath()
	if _, err := os.Stat(script); err != nil {
		t.Skipf("missing %s", script)
	}
	return true
}

func runIcadBridge(t *testing.T, audioPath string) icadBridgeResult {
	t.Helper()
	cmd := exec.Command("python3", icadBridgeScriptPath(), "--json", audioPath)
	out, err := cmd.Output() // JSON on stdout only; warnings go to stderr
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			t.Fatalf("icad bridge %s: %v\nstderr: %s", audioPath, err, ee.Stderr)
		}
		t.Fatalf("icad bridge %s: %v", audioPath, err)
	}
	var result icadBridgeResult
	if err := json.Unmarshal(out, &result); err != nil {
		t.Fatalf("icad bridge json: %v\n%s", err, out)
	}
	return result
}

type toneLearnCompareRow struct {
	callID      string
	tlrCand     bool
	tlrMatch    bool
	tlrLabel    string
	tlrScore    float64
	icadCand    bool
	icadMatch   bool
	icadLabel   string
	icadScore   float64
	tlrOnly     bool
	icadOnly    bool
	bothMatch   bool
	bothMiss    bool
	disagree    bool
}

func compareLearnOnExport(
	t *testing.T,
	csvPath, audioDir string,
	maxToneCalls int,
	matchTolHz float64,
	systemId, talkgroupId uint64,
	official []trumbullOfficial,
) (rows []toneLearnCompareRow, tried, tlrMatched, icadMatched int) {
	t.Helper()
	f, err := os.Open(csvPath)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	csvRows, err := csv.NewReader(f).ReadAll()
	if err != nil {
		t.Fatal(err)
	}

	cfg := DefaultAutoLearnToneSetConfig()
	cfg.normalize()
	detector := NewToneDetector()

	for i, row := range csvRows {
		if i == 0 || len(row) < 7 {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(row[2]), "t") {
			continue
		}
		if tried >= maxToneCalls {
			break
		}
		tried++
		callID := row[0]
		audioFilename := strings.TrimSpace(row[6])
		path := filepath.Join(audioDir, callID+"_"+audioFilename)

		var rowOut toneLearnCompareRow
		rowOut.callID = callID

		audio, err := os.ReadFile(path)
		if err != nil {
			t.Logf("call %s: missing audio %v", callID, err)
			rows = append(rows, rowOut)
			continue
		}

		// TLR Discover → auto-learn
		tlrTones, err := detector.Discover(audio, toneHistoryAudioMime("", audioFilename))
		if err != nil {
			t.Logf("call %s: TLR discover err %v", callID, err)
		}
		tlrCands := extractToneLearnCandidates(tlrTones, cfg, systemId, talkgroupId)
		if len(tlrCands) > 0 {
			rowOut.tlrCand = true
			c := tlrCands[0]
			nearest, score := nearestTrumbull(c, official)
			rowOut.tlrScore = score
			rowOut.tlrLabel = nearest.label
			if score <= matchTolHz {
				rowOut.tlrMatch = true
				tlrMatched++
			}
		}

		// icad → same auto-learn extractor
		icad := runIcadBridge(t, path)
		icadCands := extractToneLearnCandidates(icad.Tones, cfg, systemId, talkgroupId)
		if len(icadCands) > 0 {
			rowOut.icadCand = true
			c := icadCands[0]
			nearest, score := nearestTrumbull(c, official)
			rowOut.icadScore = score
			rowOut.icadLabel = nearest.label
			if score <= matchTolHz {
				rowOut.icadMatch = true
				icadMatched++
			}
		}

		rowOut.tlrOnly = rowOut.tlrMatch && !rowOut.icadMatch
		rowOut.icadOnly = rowOut.icadMatch && !rowOut.tlrMatch
		rowOut.bothMatch = rowOut.tlrMatch && rowOut.icadMatch
		rowOut.bothMiss = rowOut.tlrCand && rowOut.icadCand && !rowOut.tlrMatch && !rowOut.icadMatch
		rowOut.disagree = rowOut.tlrCand && rowOut.icadCand &&
			(rowOut.tlrMatch != rowOut.icadMatch || rowOut.tlrLabel != rowOut.icadLabel)

		rows = append(rows, rowOut)
	}
	return rows, tried, tlrMatched, icadMatched
}

func TestIcadNewtonFalls44571060(t *testing.T) {
	if !icadAvailable(t) {
		return
	}
	const base = "/tmp/tlr-debug/tg-46036-3d"
	path := filepath.Join(base, "audio", "44571060_20260604_062449.851.mp3")
	if _, err := os.Stat(path); err != nil {
		t.Skipf("missing %s", path)
	}

	icad := runIcadBridge(t, path)
	if len(icad.TwoTone) != 1 {
		t.Fatalf("icad two_tone: want 1 hit, got %d", len(icad.TwoTone))
	}
	det := icad.TwoTone[0]["detected"].([]any)
	aHz, _ := det[0].(float64)
	bHz, _ := det[1].(float64)
	if diff := absFloat(aHz - 407.3); diff > 5 {
		t.Errorf("icad A: got %.1f want ~407", aHz)
	}
	if diff := absFloat(bHz - 1103.0); diff > 5 {
		t.Errorf("icad B: got %.1f want ~1103", bHz)
	}

	cfg := DefaultAutoLearnToneSetConfig()
	cfg.normalize()
	cands := extractToneLearnCandidates(icad.Tones, cfg, 44, 2762)
	if len(cands) != 1 {
		t.Fatalf("icad→learn: want 1 candidate, got %d (tones=%d)", len(cands), len(icad.Tones))
	}
	c := cands[0]
	if c.PatternType != toneLearnPatternABPair {
		t.Fatalf("want ab_pair, got %s", c.PatternType)
	}
	nearest, score := nearestTrumbull(c, trumbullOfficialSample())
	if nearest.label != "Newton Falls 43 DUTY" || score > 25 {
		t.Fatalf("learn match: %s score=%.1f A=%.1f B=%.1f", nearest.label, score, c.AFrequency, c.BFrequency)
	}

	audio, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	tlrTones, err := NewToneDetector().Discover(audio, "audio/mpeg")
	if err != nil {
		t.Fatal(err)
	}
	tlrCands := extractToneLearnCandidates(tlrTones, cfg, 44, 2762)
	if len(tlrCands) != 1 {
		t.Fatalf("TLR discover→learn: want 1 candidate, got %d", len(tlrCands))
	}
	t.Logf("icad: A=%.1f B=%.1f | TLR: A=%.1f B=%.1f | both Newton Falls (score %.1f)",
		c.AFrequency, c.BFrequency, tlrCands[0].AFrequency, tlrCands[0].BFrequency, score)
}

func TestCompareTrumbull46036_TLRvsIcadAutoLearn(t *testing.T) {
	if !icadAvailable(t) {
		return
	}
	const (
		base         = "/tmp/tlr-debug/tg-46036-3d"
		maxToneCalls = 20
		matchTolHz   = 25.0
	)
	csvPath := filepath.Join(base, "calls.csv")
	if _, err := os.Stat(csvPath); err != nil {
		t.Skipf("missing %s", csvPath)
	}

	rows, tried, tlrMatched, icadMatched := compareLearnOnExport(
		t, csvPath, filepath.Join(base, "audio"),
		maxToneCalls, matchTolHz, 44, 2762, trumbullOfficialSample(),
	)

	var tlrCand, icadCand, bothMatch, tlrOnly, icadOnly, disagree int
	for _, r := range rows {
		if r.tlrCand {
			tlrCand++
		}
		if r.icadCand {
			icadCand++
		}
		if r.bothMatch {
			bothMatch++
		}
		if r.tlrOnly {
			tlrOnly++
			t.Logf("TLR-only MATCH call %s (%s score %.1f)", r.callID, r.tlrLabel, r.tlrScore)
		}
		if r.icadOnly {
			icadOnly++
			t.Logf("icad-only MATCH call %s (%s score %.1f)", r.callID, r.icadLabel, r.icadScore)
		}
		if r.disagree {
			disagree++
			t.Logf("disagree call %s: TLR %s match=%v score=%.1f | icad %s match=%v score=%.1f",
				r.callID, r.tlrLabel, r.tlrMatch, r.tlrScore, r.icadLabel, r.icadMatch, r.icadScore)
		}
		if r.bothMiss {
			t.Logf("both MISS call %s: TLR %s %.1f | icad %s %.1f",
				r.callID, r.tlrLabel, r.tlrScore, r.icadLabel, r.icadScore)
		}
	}

	t.Logf("SUMMARY tried=%d tlrCandidates=%d icadCandidates=%d tlrMatched=%d icadMatched=%d bothMatch=%d tlrOnly=%d icadOnly=%d disagree=%d",
		tried, tlrCand, icadCand, tlrMatched, icadMatched, bothMatch, tlrOnly, icadOnly, disagree)
	if tried == 0 {
		t.Fatal("no sample calls")
	}
}

func TestCompareLFD_TLRvsIcadAutoLearn(t *testing.T) {
	if !icadAvailable(t) {
		return
	}
	const (
		exportDir    = "/tmp/tlr-debug/tlr-lfd-export"
		maxCalls     = 40
		matchTolHz   = 25.0
		systemId     = 44
		talkgroupId  = 2764
	)
	audioDir := filepath.Join(exportDir, "audio")
	if _, err := os.Stat(audioDir); err != nil {
		t.Skipf("missing %s", audioDir)
	}
	csvPath := loadLFDExportCSV(t, exportDir)

	f, err := os.Open(csvPath)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	csvRows, err := csv.NewReader(f).ReadAll()
	if err != nil {
		t.Fatal(err)
	}

	cfg := DefaultAutoLearnToneSetConfig()
	cfg.normalize()
	detector := NewToneDetector()
	official := lfdOfficialTonePairs()

	var tried, tlrMatched, icadMatched, tlrCand, icadCand int
	for i, row := range csvRows {
		if i == 0 || len(row) < 2 {
			continue
		}
		if tried >= maxCalls {
			break
		}
		tried++
		callID := strings.TrimSpace(row[0])
		audioFilename := lfdExportAudioFilename(row)
		path := filepath.Join(audioDir, callID+"_"+audioFilename)

		audio, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		tlrTones, _ := detector.Discover(audio, toneHistoryAudioMime("", audioFilename))
		tlrAB := discoverABPair(tlrTones)
		if tlrAB.ok {
			tlrCand++
			_, score := nearestOfficialPair(tlrAB.aHz, tlrAB.bHz, official)
			if score <= matchTolHz {
				tlrMatched++
			}
		}

		icad := runIcadBridge(t, path)
		icadAB := icadABFromBridge(icad)
		if icadAB.ok {
			icadCand++
			nearest, score := nearestOfficialPair(icadAB.aHz, icadAB.bHz, official)
			if score <= matchTolHz {
				icadMatched++
				t.Logf("icad MATCH call %s %s A=%.1f B=%.1f score=%.1f", callID, nearest.label, icadAB.aHz, icadAB.bHz, score)
			}
		}

		if tlrAB.ok && icadAB.ok {
			t.Logf("call %s TLR A=%.1f B=%.1f | icad A=%.1f B=%.1f",
				callID, tlrAB.aHz, tlrAB.bHz, icadAB.aHz, icadAB.bHz)
		}
	}

	t.Logf("LFD sample n=%d tlrAB=%d icadAB=%d tlrMatched=%d icadMatched=%d (tol %.0f Hz)",
		tried, tlrCand, icadCand, tlrMatched, icadMatched, matchTolHz)
}

type abPairSummary struct {
	ok  bool
	aHz float64
	bHz float64
}

func discoverABPair(tones []Tone) abPairSummary {
	var a, b *Tone
	for i := range tones {
		switch tones[i].ToneType {
		case "A":
			if a == nil {
				a = &tones[i]
			}
		case "B":
			if b == nil {
				b = &tones[i]
			}
		}
	}
	if a == nil || b == nil {
		// fallback: first two tones by time
		if len(tones) >= 2 {
			return abPairSummary{ok: true, aHz: tones[0].Frequency, bHz: tones[1].Frequency}
		}
		return abPairSummary{}
	}
	return abPairSummary{ok: true, aHz: a.Frequency, bHz: b.Frequency}
}

func icadABFromBridge(icad icadBridgeResult) abPairSummary {
	if len(icad.TwoTone) == 0 {
		return abPairSummary{}
	}
	det, ok := icad.TwoTone[0]["detected"].([]any)
	if !ok || len(det) < 2 {
		return abPairSummary{}
	}
	aHz, _ := det[0].(float64)
	bHz, _ := det[1].(float64)
	return abPairSummary{ok: true, aHz: aHz, bHz: bHz}
}

func absFloat(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

// lfdExportAudioFilename reads audio filename from LFD export CSV (audioFilename or last column).
func lfdExportAudioFilename(row []string) string {
	if len(row) >= 7 {
		if fn := strings.TrimSpace(row[6]); fn != "" && strings.HasSuffix(strings.ToLower(fn), ".mp3") {
			return fn
		}
	}
	return strings.TrimSpace(row[len(row)-1])
}

func TestIcadBridgeSmoke(t *testing.T) {
	if !icadAvailable(t) {
		return
	}
	// Ensure script runs without crashing on missing file
	cmd := exec.Command("python3", icadBridgeScriptPath(), "--json", "/nonexistent.mp3")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("expected error for missing file")
	}
	_ = out
	fmt.Fprintf(os.Stderr, "icad bridge smoke: missing file correctly failed\n")
}
