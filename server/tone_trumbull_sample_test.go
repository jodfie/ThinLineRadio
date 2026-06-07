// Copyright (C) 2025 Thinline Dynamic Solutions
//
// Quick sample: first N prod tone calls on TG 46036 vs official Trumbull table.

package main

import (
	"encoding/csv"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type trumbullOfficial struct {
	label  string
	aHz    float64
	bHz    float64
	longHz float64
}

func trumbullOfficialSample() []trumbullOfficial {
	return []trumbullOfficial{
		{label: "Brookfield 18/51", longHz: 358.6},
		{label: "Champion 21 DUTY", aHz: 389, bHz: 1063.2},
		{label: "Fowler 23 PAGER", longHz: 1285.8},
		{label: "Girard City ON", aHz: 349, bHz: 330.5},
		{label: "Howland 30 DUTY", aHz: 433.7, bHz: 1006.9},
		{label: "Hubbard 28 DUTY", aHz: 1122.5, bHz: 832.5},
		{label: "Hubbard 28 OFF FIRE", longHz: 1122.5},
		{label: "Mecca 38", aHz: 496.8, bHz: 1395},
		{label: "Newton Falls 43 DUTY", aHz: 399.8, bHz: 1092.4},
		{label: "Vienna 46", longHz: 399.8},
		{label: "Lordstown 36 DUTY", aHz: 1251.4, bHz: 1122.5},
		{label: "Warren W344 DUTY", aHz: 688.3, bHz: 1251.4},
	}
}

func abMatchScore(aHz, bHz, oA, oB float64) float64 {
	direct := math.Max(math.Abs(aHz-oA), math.Abs(bHz-oB))
	swap := math.Max(math.Abs(aHz-oB), math.Abs(bHz-oA))
	return math.Min(direct, swap)
}

func nearestTrumbull(cand toneLearnCandidate, official []trumbullOfficial) (trumbullOfficial, float64) {
	best := official[0]
	bestScore := 1e9
	for _, o := range official {
		var score float64
		switch cand.PatternType {
		case toneLearnPatternABPair:
			if o.aHz == 0 || o.bHz == 0 {
				continue
			}
			score = abMatchScore(cand.AFrequency, cand.BFrequency, o.aHz, o.bHz)
		case toneLearnPatternLong:
			if o.longHz == 0 {
				continue
			}
			score = math.Abs(cand.LongFrequency - o.longHz)
		default:
			continue
		}
		if score < bestScore {
			best = o
			bestScore = score
		}
	}
	return best, bestScore
}

func TestDiscoverTrumbull46036Sample(t *testing.T) {
	const (
		base         = "/tmp/tlr-debug/tg-46036-3d"
		maxToneCalls = 20
		matchTolHz   = 25.0
		systemId     = 44
		talkgroupId  = 2762
	)

	csvPath := filepath.Join(base, "calls.csv")
	audioDir := filepath.Join(base, "audio")
	if _, err := os.Stat(csvPath); err != nil {
		t.Skipf("missing %s", csvPath)
	}

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
	official := trumbullOfficialSample()

	var tried, withCand, matched int
	for i, row := range rows {
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
		audio, err := os.ReadFile(path)
		if err != nil {
			t.Logf("call %s: missing audio", callID)
			continue
		}
		tones, err := detector.Discover(audio, toneHistoryAudioMime("", audioFilename))
		if err != nil {
			t.Logf("call %s: discover err %v", callID, err)
			continue
		}
		cands := extractToneLearnCandidates(tones, cfg, systemId, talkgroupId)
		if len(cands) == 0 {
			t.Logf("call %s: %d tones, no learn candidate", callID, len(tones))
			continue
		}
		withCand++
		c := cands[0]
		nearest, score := nearestTrumbull(c, official)
		switch c.PatternType {
		case toneLearnPatternABPair:
			if score <= matchTolHz {
				matched++
				t.Logf("call %s: MATCH %s A=%.1f B=%.1f (score %.1f)", callID, nearest.label, c.AFrequency, c.BFrequency, score)
			} else {
				t.Logf("call %s: MISS A=%.1f B=%.1f best=%s score=%.1f", callID, c.AFrequency, c.BFrequency, nearest.label, score)
			}
		case toneLearnPatternLong:
			if score <= matchTolHz {
				matched++
				t.Logf("call %s: MATCH %s long=%.1f Hz (score %.1f)", callID, nearest.label, c.LongFrequency, score)
			} else {
				t.Logf("call %s: MISS long=%.1f best=%s score=%.1f", callID, c.LongFrequency, nearest.label, score)
			}
		}
	}

	t.Logf("sample: prod-tone-calls=%d withCandidates=%d matchedOfficial=%d (within %.0f Hz)", tried, withCand, matched, matchTolHz)
	if tried == 0 {
		t.Fatal("no sample calls")
	}
}
