// Copyright (C) 2025 Thinline Dynamic Solutions
//
// Compare Discover A+B pairs on exported LFD (TG 46038) audio against official tone table.

package main

import (
	"encoding/csv"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type officialABPair struct {
	label string
	aHz   float64
	bHz   float64
}

func lfdOfficialTonePairs() []officialABPair {
	return []officialABPair{
		{label: "Lordstown 36 OFF DUTY E636", aHz: 1217.4, bHz: 1320.2},
		{label: "Lordstown 36 DUTY", aHz: 1251.4, bHz: 1122.5},
		{label: "Warren W344 DUTY", aHz: 688.3, bHz: 1251.4},
		{label: "Warren W355 PLECTRON", aHz: 726.8, bHz: 1285.8},
		{label: "Warren W366 OFF Duty", aHz: 767.4, bHz: 1321.2},
		{label: "Warren W388 PAGER/SRN", aHz: 855.5, bHz: 1395.0},
	}
}

func nearestOfficialPair(aHz, bHz float64, official []officialABPair) (officialABPair, float64) {
	best := official[0]
	bestScore := math.Max(math.Abs(aHz-best.aHz), math.Abs(bHz-best.bHz))
	for _, o := range official[1:] {
		score := math.Max(math.Abs(aHz-o.aHz), math.Abs(bHz-o.bHz))
		if score < bestScore {
			best = o
			bestScore = score
		}
	}
	return best, bestScore
}

func loadLFDExportCSV(t *testing.T, exportDir string) string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(exportDir, "calls_*.csv"))
	if err != nil || len(matches) == 0 {
		t.Skipf("no calls_*.csv in %s", exportDir)
	}
	best := matches[0]
	bestRows := 0
	for _, path := range matches {
		f, err := os.Open(path)
		if err != nil {
			continue
		}
		rows, err := csv.NewReader(f).ReadAll()
		f.Close()
		if err != nil {
			continue
		}
		dataRows := len(rows) - 1
		if dataRows > bestRows {
			bestRows = dataRows
			best = path
		}
	}
	return best
}

func TestDiscoverLFDVsOfficialTones(t *testing.T) {
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

	const systemId, talkgroupId uint64 = 44, 2764
	const matchTolHz = 25.0

	type hit struct {
		label string
		calls int
		sumA  float64
		sumB  float64
	}
	byOfficial := make(map[string]*hit)
	var scanned, withCand, matched int

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
		scanned++
		tones, err := detector.Discover(audio, toneHistoryAudioMime("", audioFilename))
		if err != nil || len(tones) == 0 {
			continue
		}
		cands := extractToneLearnCandidates(tones, cfg, systemId, talkgroupId)
		if len(cands) == 0 {
			continue
		}
		withCand++
		c := cands[0]
		nearest, score := nearestOfficialPair(c.AFrequency, c.BFrequency, official)
		if score > matchTolHz {
			t.Logf("call %s: detected A=%.1f B=%.1f — no official match within %.0f Hz (best %s score=%.1f)",
				callID, c.AFrequency, c.BFrequency, matchTolHz, nearest.label, score)
			continue
		}
		matched++
		h := byOfficial[nearest.label]
		if h == nil {
			h = &hit{label: nearest.label}
			byOfficial[nearest.label] = h
		}
		h.calls++
		h.sumA += c.AFrequency
		h.sumB += c.BFrequency
	}

	t.Logf("scanned=%d withCandidates=%d matchedOfficial=%d (within %.0f Hz)", scanned, withCand, matched, matchTolHz)
	for _, o := range official {
		h := byOfficial[o.label]
		if h == nil {
			t.Logf("  %s: 0 calls (official A=%.1f B=%.1f)", o.label, o.aHz, o.bHz)
			continue
		}
		avgA := h.sumA / float64(h.calls)
		avgB := h.sumB / float64(h.calls)
		t.Logf("  %s: %d calls — detected avg A=%.1f (off %.1f) B=%.1f (off %.1f)",
			o.label, h.calls, avgA, avgA-o.aHz, avgB, avgB-o.bHz)
	}
	if scanned == 0 {
		t.Fatal("no audio files scanned")
	}
}
