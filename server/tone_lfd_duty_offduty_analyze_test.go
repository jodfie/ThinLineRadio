// Quick analysis: Lordstown DUTY vs OFF DUTY tones on LFD export (TG 46038).

package main

import (
	"encoding/csv"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const lfdAnalyzeTolHz = 30.0

type tonePairTarget struct {
	name string
	aHz  float64
	bHz  float64
}

func toneNear(freq, target, tol float64) bool {
	return math.Abs(freq-target) <= tol
}

func callHasABPair(tones []Tone, aHz, bHz, tol float64) (bool, []Tone, []Tone) {
	var nearA, nearB []Tone
	for _, t := range tones {
		if toneNear(t.Frequency, aHz, tol) {
			nearA = append(nearA, t)
		}
		if toneNear(t.Frequency, bHz, tol) {
			nearB = append(nearB, t)
		}
	}
	return len(nearA) > 0 && len(nearB) > 0, nearA, nearB
}

func formatToneList(tones []Tone) string {
	if len(tones) == 0 {
		return "(none)"
	}
	parts := make([]string, 0, len(tones))
	for _, t := range tones {
		parts = append(parts, fmt.Sprintf("%.1fHz start=%.3fs end=%.3fs dur=%.3fs", t.Frequency, t.StartTime, t.EndTime, t.Duration))
	}
	return strings.Join(parts, "; ")
}

func TestAnalyzeLFDDutyOffDutyDiscover(t *testing.T) {
	exportDir := "/tmp/tlr-debug/tlr-lfd-export"
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
	rows, err := csv.NewReader(f).ReadAll()
	if err != nil {
		t.Fatal(err)
	}

	cfg := DefaultAutoLearnToneSetConfig()
	cfg.normalize()
	detector := NewToneDetector()
	official := lfdOfficialTonePairs()
	const matchTolHz = 25.0
	const systemId, talkgroupId uint64 = 44, 2764

	duty := tonePairTarget{"Lordstown DUTY", 1251.4, 1122.5}
	off := tonePairTarget{"Lordstown OFF DUTY", 1217.4, 1320.2}
	w344 := tonePairTarget{"Warren W344", 688.3, 1251.4}

	var scanned, withAudio, withTones int
	var onlyDuty, onlyOff, bothDutyOff int
	var hasDuty, hasOff, hasW344 int

	type bothRecord struct {
		callID string
		dutyA  []Tone
		dutyB  []Tone
		offA   []Tone
		offB   []Tone
		all    []Tone
	}
	var bothCalls []bothRecord

	var officialDutyMatched []string
	type dutyPlusOff struct {
		callID string
		candA  float64
		candB  float64
		offA   []Tone
		offB   []Tone
	}
	var officialDutyAlsoOff []dutyPlusOff

	for i, row := range rows {
		if i == 0 || len(row) < 7 {
			continue
		}
		callID := row[0]
		audioFilename := strings.TrimSpace(row[6])
		path := filepath.Join(audioDir, callID+"_"+audioFilename)
		audio, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		withAudio++
		tones, err := detector.Discover(audio, toneHistoryAudioMime("", audioFilename))
		if err != nil {
			t.Logf("call %s: Discover error: %v", callID, err)
			continue
		}
		scanned++
		if len(tones) == 0 {
			continue
		}
		withTones++

		hasD, dA, dB := callHasABPair(tones, duty.aHz, duty.bHz, lfdAnalyzeTolHz)
		hasO, oA, oB := callHasABPair(tones, off.aHz, off.bHz, lfdAnalyzeTolHz)
		hasW, _, _ := callHasABPair(tones, w344.aHz, w344.bHz, lfdAnalyzeTolHz)
		if hasD {
			hasDuty++
		}
		if hasO {
			hasOff++
		}
		if hasW {
			hasW344++
		}

		switch {
		case hasD && hasO:
			bothDutyOff++
			bothCalls = append(bothCalls, bothRecord{callID, dA, dB, oA, oB, tones})
		case hasD && !hasO:
			onlyDuty++
		case hasO && !hasD:
			onlyOff++
		}

		cands := extractToneLearnCandidates(tones, cfg, systemId, talkgroupId)
		if len(cands) == 0 {
			continue
		}
		c := cands[0]
		nearest, score := nearestOfficialPair(c.AFrequency, c.BFrequency, official)
		if score > matchTolHz || nearest.label != "Lordstown 36 DUTY" {
			continue
		}
		officialDutyMatched = append(officialDutyMatched, callID)
		if hasO {
			officialDutyAlsoOff = append(officialDutyAlsoOff, dutyPlusOff{callID, c.AFrequency, c.BFrequency, oA, oB})
		}
	}

	t.Log("=== LFD export Discover analysis (TG 46038) ===")
	t.Logf("CSV: %s", csvPath)
	t.Logf("Calls with audio files: %d", withAudio)
	t.Logf("Discover attempted: %d", scanned)
	t.Logf("Calls with >=1 detected tone: %d", withTones)
	t.Log("")
	t.Logf("Pair presence (±%.0f Hz on any Discover tone):", lfdAnalyzeTolHz)
	t.Logf("  Lordstown DUTY (A~1251, B~1122): %d calls", hasDuty)
	t.Logf("  Lordstown OFF DUTY (A~1217, B~1320): %d calls", hasOff)
	t.Logf("  Warren W344 (A~688, B~1251): %d calls", hasW344)
	t.Log("")
	t.Log("Lordstown DUTY vs OFF (mutually exclusive buckets):")
	t.Logf("  ONLY DUTY (duty present, off absent): %d", onlyDuty)
	t.Logf("  ONLY OFF DUTY (off present, duty absent): %d", onlyOff)
	t.Logf("  BOTH DUTY+OFF in same clip: %d", bothDutyOff)
	t.Log("")
	if len(bothCalls) > 0 {
		t.Log("Calls with BOTH DUTY and OFF-DUTY tones in Discover:")
		for _, rec := range bothCalls {
			t.Logf("  call %s:", rec.callID)
			t.Logf("    all tones: %s", formatAllTones(rec.all))
			t.Logf("    duty A-near: %s", formatToneList(rec.dutyA))
			t.Logf("    duty B-near: %s", formatToneList(rec.dutyB))
			t.Logf("    off A-near: %s", formatToneList(rec.offA))
			t.Logf("    off B-near: %s", formatToneList(rec.offB))
		}
	} else {
		t.Log("Calls with BOTH DUTY and OFF-DUTY tones: (none)")
	}
	t.Log("")
	t.Logf("Official-compare Lordstown 36 DUTY matches (extractToneLearnCandidates + nearest within %.0f Hz): %d", matchTolHz, len(officialDutyMatched))
	if len(officialDutyAlsoOff) > 0 {
		t.Logf("Of those, also have OFF-DUTY tones in Discover (±%.0f Hz): %d", lfdAnalyzeTolHz, len(officialDutyAlsoOff))
		for _, rec := range officialDutyAlsoOff {
			t.Logf("  call %s: learn cand A=%.1f B=%.1f; off A-near: %s; off B-near: %s",
				rec.callID, rec.candA, rec.candB, formatToneList(rec.offA), formatToneList(rec.offB))
		}
	} else {
		t.Log("Of official DUTY-matched calls, none also show OFF-DUTY A+B in Discover output.")
	}

	if withAudio == 0 {
		t.Fatal("no audio files found")
	}
}

func formatAllTones(tones []Tone) string {
	if len(tones) == 0 {
		return "(none)"
	}
	parts := make([]string, len(tones))
	for i, t := range tones {
		parts[i] = fmt.Sprintf("%.1fHz [%.3f-%.3f] dur=%.3fs", t.Frequency, t.StartTime, t.EndTime, t.Duration)
	}
	return strings.Join(parts, " | ")
}
