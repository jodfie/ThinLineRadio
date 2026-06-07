// Copyright (C) 2025 Thinline Dynamic Solutions
//
// Single-engine tone extraction: one STFT front-end + one grouping stage, then classify.
// Architecture mirrors icad_tone_detection (thegreatcodeholio, Apache 2.0): a single
// per-frame dominant-frequency stream is grouped into stable segments using a dynamic
// tolerance (percent-of-frequency capped to an absolute Hz), a hard force-split for large
// frame-to-frame steps, and silence (OFF) breaks. Tone "type" (long / two-tone A / B) is a
// label applied AFTER grouping based on duration and order, not a separate DSP path.
//
// One parameter set serves every Ohio paging style because the numbers do not conflict:
//   - intra-tone drift on analog/compressed MP3 stays < ~30 Hz
//   - the smallest real A->B transition in the tone book (Lordstown 1251->1122) is 129 Hz
//   - a force-split threshold between those two cleanly separates real tone pairs of any
//     frequency without splitting a single tone's own wander.
// Transient harmonics (e.g. a 407 Hz A-tone's 3rd harmonic near 1223 Hz during ring-up) are
// rejected by the minimum-duration gate: they never sustain long enough to form a segment.

package main

import (
	"fmt"
	"math"
	"sort"
)

// STFT engine parameters (shared by production Detect and auto-learn Discover).
const (
	stftWindowSize   = 2048 // ~128 ms @ 16 kHz; 7.8 Hz/bin before parabolic interpolation
	stftHop          = 256  // ~16 ms @ 16 kHz; fine onset/offset resolution
	stftForceSplitHz = 50.0 // > max intra-tone drift (~30 Hz), < smallest real A/B split (129 Hz)
	stftAbsCapHz     = 30.0 // absolute cap on dynamic grouping tolerance
	stftMatchPct     = 2.5  // percent-of-previous-frequency grouping tolerance (icad default)
)

type toneAnalysisGates struct {
	globalPeak   float64
	noiseFloorDB float64
	q20          float64
}

// computeToneAnalysisGates estimates the global peak, 20th-percentile level and noise floor
// from early paging audio so later dispatch voice does not hide quiet lead-in tones.
func (detector *ToneDetector) computeToneAnalysisGates(samples []float64, sampleRate int) toneAnalysisGates {
	toneRange := detector.FrequencyRange
	if toneRange.Max == 0 {
		toneRange.Max = 5000
	}
	windowSize := stftWindowSize
	hopSize := stftHop

	var framePeaks []float64
	for win := 0; ; win++ {
		start := win * hopSize
		end := start + windowSize
		if end > len(samples) {
			break
		}
		window := samples[start:end]
		windowed := make([]float64, len(window))
		for i := range window {
			hann := 0.5 * (1.0 - math.Cos(2.0*math.Pi*float64(i)/float64(len(window)-1)))
			windowed[i] = window[i] * hann
		}
		magnitudes := detector.dft(windowed, sampleRate)
		var framePeak float64
		for bin, mag := range magnitudes {
			freq := float64(bin) * float64(sampleRate) / float64(windowSize)
			if freq >= toneRange.Min && freq <= toneRange.Max && mag > framePeak {
				framePeak = mag
			}
		}
		framePeaks = append(framePeaks, framePeak)
	}

	gates := toneAnalysisGates{noiseFloorDB: -60}
	if len(framePeaks) == 0 {
		return gates
	}

	peakRefFrames := len(framePeaks)
	if tonePeakReferenceSeconds > 0 {
		peakRefFrames = int(tonePeakReferenceSeconds*float64(sampleRate)/float64(hopSize)) + 1
		if peakRefFrames > len(framePeaks) {
			peakRefFrames = len(framePeaks)
		}
	}
	for i, peak := range framePeaks {
		if i >= peakRefFrames {
			break
		}
		if peak > gates.globalPeak {
			gates.globalPeak = peak
		}
	}
	if gates.globalPeak < 1e-20 {
		return gates
	}

	relativeDB := make([]float64, len(framePeaks))
	for i, peak := range framePeaks {
		relativeDB[i] = 20.0 * math.Log10(math.Max(peak, 1e-20)/gates.globalPeak)
	}
	sortedDB := make([]float64, len(relativeDB))
	copy(sortedDB, relativeDB)
	sort.Float64s(sortedDB)
	q20Index := int(float64(len(sortedDB)) * 0.20)
	if q20Index >= len(sortedDB) {
		q20Index = len(sortedDB) - 1
	}
	gates.q20 = sortedDB[q20Index]

	var belowQ20 []float64
	for _, db := range relativeDB {
		if db <= gates.q20 {
			belowQ20 = append(belowQ20, db)
		}
	}
	if len(belowQ20) > 0 {
		sort.Float64s(belowQ20)
		gates.noiseFloorDB = belowQ20[len(belowQ20)/2]
	}
	return gates
}

// stftFrame is one analysis frame: left-edge sample index, dominant in-band frequency
// (0 = OFF / silent), and that peak's magnitude.
type stftFrame struct {
	startSample int
	freq        float64
	mag         float64
}

// frameDominantFrequency returns the dominant in-band frequency for one window with parabolic
// sub-bin interpolation, or (0,0) when the frame fails silence/SNR/magnitude gating.
func (detector *ToneDetector) frameDominantFrequency(window []float64, sampleRate int, gates toneAnalysisGates) (float64, float64) {
	toneRange := detector.FrequencyRange
	if toneRange.Max == 0 {
		toneRange.Max = 5000
	}
	windowSize := len(window)
	windowed := make([]float64, windowSize)
	for i := range window {
		hann := 0.5 * (1.0 - math.Cos(2.0*math.Pi*float64(i)/float64(windowSize-1)))
		windowed[i] = window[i] * hann
	}
	magnitudes := detector.dft(windowed, sampleRate)

	bestBin, bestMag := -1, 0.0
	for bin, mag := range magnitudes {
		freq := float64(bin) * float64(sampleRate) / float64(windowSize)
		if freq < toneRange.Min || freq > toneRange.Max {
			continue
		}
		if mag > bestMag {
			bestMag = mag
			bestBin = bin
		}
	}
	if bestBin < 0 || bestMag <= toneDetectMagnitudeThreshold || gates.globalPeak < 1e-20 {
		return 0, 0
	}

	// Silence / SNR gating against the global peak and estimated noise floor.
	relDB := 20.0 * math.Log10(math.Max(bestMag, 1e-20)/gates.globalPeak)
	if relDB < toneDetectSilenceBelowGlobal || relDB < gates.noiseFloorDB+toneDetectSNRAboveNoise {
		return 0, 0
	}

	bestFreq := float64(bestBin) * float64(sampleRate) / float64(windowSize)
	if mPrev, okP := magnitudes[bestBin-1]; okP {
		if mNext, okN := magnitudes[bestBin+1]; okN {
			delta := parabolicInterpolate(mPrev, bestMag, mNext)
			delta = math.Max(-0.5, math.Min(0.5, delta))
			bestFreq += delta * float64(sampleRate) / float64(windowSize)
		}
	}
	return bestFreq, bestMag
}

// analyzeSTFTTones is the single tone-extraction engine: per-frame dominant frequency, then
// one grouping pass (dynamic tolerance + force-split + OFF breaks), then a minimum-duration
// gate. It returns stable segments; classification into A/B/Long happens in the caller.
func (detector *ToneDetector) analyzeSTFTTones(samples []float64, sampleRate int, gates toneAnalysisGates) []mergedDetection {
	windowSize := stftWindowSize
	if len(samples) < windowSize {
		return nil
	}
	hop := stftHop

	var frames []stftFrame
	for start := 0; start+windowSize <= len(samples); start += hop {
		freq, mag := detector.frameDominantFrequency(samples[start:start+windowSize], sampleRate, gates)
		frames = append(frames, stftFrame{startSample: start, freq: freq, mag: mag})
	}
	if len(frames) == 0 {
		return nil
	}

	windowSec := float64(windowSize) / float64(sampleRate)
	var dets []mergedDetection

	var (
		groupFreqs []float64
		groupMag   float64
		groupStart int
		groupEnd   int
		inGroup    bool
	)

	flush := func() {
		if !inGroup || len(groupFreqs) == 0 {
			inGroup = false
			groupFreqs = nil
			return
		}
		startTime := float64(groupStart) / float64(sampleRate)
		endTime := float64(groupEnd)/float64(sampleRate) + windowSec
		if endTime-startTime >= toneDetectMinDurationSec {
			hist := make([]float64, len(groupFreqs))
			copy(hist, groupFreqs)
			dets = append(dets, mergedDetection{
				frequency:   medianFloat(groupFreqs),
				startTime:   startTime,
				endTime:     endTime,
				magnitude:   groupMag,
				count:       1,
				freqHistory: hist,
			})
		}
		inGroup = false
		groupFreqs = nil
		groupMag = 0
	}

	startNew := func(fr stftFrame) {
		inGroup = true
		groupFreqs = []float64{fr.freq}
		groupMag = fr.mag
		groupStart = fr.startSample
		groupEnd = fr.startSample
	}

	for i, fr := range frames {
		if fr.freq <= 0 { // OFF frame: always breaks the current ON group
			flush()
			continue
		}
		if !inGroup {
			startNew(fr)
			continue
		}
		prev := groupFreqs[len(groupFreqs)-1]
		step := math.Abs(fr.freq - prev)

		// Hard force-split on a large instantaneous step (a real A->B transition).
		if step > stftForceSplitHz {
			flush()
			startNew(fr)
			continue
		}
		// Otherwise group when within the dynamic tolerance (percent of previous, abs-capped).
		thr := math.Min(prev*stftMatchPct/100.0, stftAbsCapHz)
		if step <= thr {
			groupFreqs = append(groupFreqs, fr.freq)
			groupEnd = fr.startSample
			if fr.mag > groupMag {
				groupMag = fr.mag
			}
		} else {
			flush()
			startNew(fr)
		}
		_ = i
	}
	flush()

	fmt.Printf("stft tone detection: %d stable segments\n", len(dets))
	return dets
}

func medianFloat(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	c := make([]float64, len(vals))
	copy(c, vals)
	sort.Float64s(c)
	n := len(c)
	if n%2 == 1 {
		return c[n/2]
	}
	return (c[n/2-1] + c[n/2]) / 2.0
}
