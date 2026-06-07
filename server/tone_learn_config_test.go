// Copyright (C) 2025 Thinline Dynamic Solutions

package main

import (
	"math"
	"testing"
)

func TestMigrateLegacyAutoLearnToneDurations(t *testing.T) {
	cfg := AutoLearnToneSetConfig{
		AToneMinDuration:     0.5,
		AToneMaxDuration:     0.9,
		BToneMinDuration:     1.5,
		BToneMaxDuration:     2.5,
		LongToneMinDuration:  6,
		CallsRequired:        3,
		FrequencyToleranceHz: 10,
	}
	if !migrateLegacyAutoLearnToneDurations(&cfg) {
		t.Fatal("expected migration")
	}
	if cfg.AToneMaxDuration != 1.2 || cfg.BToneMaxDuration != 3.3 {
		t.Fatalf("got A max %.1f B max %.1f", cfg.AToneMaxDuration, cfg.BToneMaxDuration)
	}
	if migrateLegacyAutoLearnToneDurations(&cfg) {
		t.Fatal("expected no second migration")
	}

	cfg2 := AutoLearnToneSetConfig{
		AToneMinDuration: 0.5, AToneMaxDuration: 1.2,
		BToneMinDuration: 1.5, BToneMaxDuration: 4.0,
	}
	if !migrateLegacyAutoLearnToneDurations(&cfg2) {
		t.Fatal("expected 4.0 B max migration")
	}
	if cfg2.BToneMaxDuration != 3.3 {
		t.Fatalf("4.0 migration: got B max %.1f", cfg2.BToneMaxDuration)
	}
}

func TestExtractToneLearnCandidates_LFDPaging(t *testing.T) {
	tones := []Tone{
		{Frequency: 1257.29, Duration: 1.024, StartTime: 1.824, EndTime: 2.848},
		{Frequency: 1232.39, Duration: 1.024, StartTime: 1.888, EndTime: 2.912},
		{Frequency: 1124.56, Duration: 3.104, StartTime: 2.816, EndTime: 5.920},
	}
	tight := tightAutoLearnToneSetConfig()
	if c := extractToneLearnCandidates(tones, tight, 44, 2764); len(c) != 0 {
		t.Fatalf("tight config should reject LFD paging, got %d", len(c))
	}
	def := DefaultAutoLearnToneSetConfig()
	def.normalize()
	c := extractToneLearnCandidates(tones, def, 44, 2764)
	if len(c) != 1 {
		t.Fatalf("default config: want 1 candidate, got %d", len(c))
	}
	if c[0].AFrequency != 1257.29 || c[0].BDuration != 3.104 {
		t.Fatalf("unexpected pair: A=%.1f/%.2fs B=%.1f/%.2fs", c[0].AFrequency, c[0].ADuration, c[0].BFrequency, c[0].BDuration)
	}
}

func TestExtractToneLearnCandidates_StackedPageEarliestB(t *testing.T) {
	tones := []Tone{
		{Frequency: 688.0, Duration: 1.12, StartTime: 1.95, EndTime: 3.07, Magnitude: 0.039},
		{Frequency: 1247.8, Duration: 3.04, StartTime: 3.04, EndTime: 6.08, Magnitude: 0.024},
		{Frequency: 1311.5, Duration: 3.07, StartTime: 8.03, EndTime: 11.10, Magnitude: 0.030},
	}
	cfg := DefaultAutoLearnToneSetConfig()
	cfg.normalize()
	c := extractToneLearnCandidates(tones, cfg, 44, 2764)
	if len(c) != 1 {
		t.Fatalf("want 1 candidate, got %d", len(c))
	}
	if c[0].BFrequency != 1247.8 {
		t.Fatalf("want earliest B 1247.8 Hz, got %.1f", c[0].BFrequency)
	}
}

func TestExtractToneLearnCandidates_NewtonFallsHarmonic(t *testing.T) {
	tones := []Tone{
		{Frequency: 407.3, Duration: 1.15, StartTime: 1.50, EndTime: 2.65, Magnitude: 0.27},
		{Frequency: 1101.9, Duration: 2.94, StartTime: 2.50, EndTime: 5.44, Magnitude: 0.13},
		{Frequency: 1223.0, Duration: 1.12, StartTime: 1.50, EndTime: 2.62, Magnitude: 0.04},
	}
	cfg := DefaultAutoLearnToneSetConfig()
	cfg.normalize()
	c := extractToneLearnCandidates(tones, cfg, 44, 2762)
	if len(c) != 1 {
		t.Fatalf("want 1 candidate, got %d", len(c))
	}
	if math.Abs(c[0].AFrequency-407.3) > 5 || math.Abs(c[0].BFrequency-1101.9) > 5 {
		t.Fatalf("want ~407/1102 Newton Falls, got A=%.1f B=%.1f", c[0].AFrequency, c[0].BFrequency)
	}
}
