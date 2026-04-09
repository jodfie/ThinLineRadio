// Copyright (C) 2025 Thinline Dynamic Solutions
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program.  If not, see <http://www.gnu.org/licenses/>

package main

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math"
	"os"
	"os/exec"
	"strings"
)

const (
	// energyFrameMs is the RMS window size in milliseconds.
	energyFrameMs = 50
	// energySampleHz is the PCM decode rate used for energy profiling.
	energySampleHz = 8000
	// energyMinFrames is the minimum number of frames required for a valid
	// energy fingerprint (~200ms of audio).
	energyMinFrames = 4
)

// ── Energy profile fingerprinting ────────────────────────────────────────────

// ComputeEnergyFingerprint decodes audio to raw PCM using ffmpeg and returns
// a normalised RMS energy profile (one value per 50ms frame). Works on any
// clip length ≥ ~200ms. Only requires ffmpeg, which TLR already depends on.
func ComputeEnergyFingerprint(audio []byte, mime string) ([]float64, error) {
	ext := audioExtFromMime(mime)
	tmp, err := os.CreateTemp("", "tlr-efp-*"+ext)
	if err != nil {
		return nil, fmt.Errorf("energy fingerprint: create temp: %w", err)
	}
	defer os.Remove(tmp.Name())

	if _, err := tmp.Write(audio); err != nil {
		tmp.Close()
		return nil, fmt.Errorf("energy fingerprint: write temp: %w", err)
	}
	tmp.Close()

	cmd := exec.Command("ffmpeg",
		"-i", tmp.Name(),
		"-f", "s16le",
		"-ar", fmt.Sprintf("%d", energySampleHz),
		"-ac", "1",
		"-loglevel", "quiet",
		"pipe:1",
	)
	pcm, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("energy fingerprint: ffmpeg decode: %w", err)
	}

	const (
		samplesPerFrame = energySampleHz * energyFrameMs / 1000 // 400
		bytesPerFrame   = samplesPerFrame * 2                   // s16le
	)

	numFrames := len(pcm) / bytesPerFrame
	if numFrames < energyMinFrames {
		return nil, fmt.Errorf("energy fingerprint: audio too short (%d frames)", numFrames)
	}

	profile := make([]float64, numFrames)
	for i := 0; i < numFrames; i++ {
		offset := i * bytesPerFrame
		var sumSq float64
		for j := 0; j < samplesPerFrame; j++ {
			s := int16(binary.LittleEndian.Uint16(pcm[offset+j*2 : offset+j*2+2]))
			sumSq += float64(s) * float64(s)
		}
		profile[i] = math.Sqrt(sumSq / float64(samplesPerFrame))
	}

	maxVal := 0.0
	for _, v := range profile {
		if v > maxVal {
			maxVal = v
		}
	}
	if maxVal > 0 {
		for i := range profile {
			profile[i] /= maxVal
		}
	}

	return profile, nil
}

// energyAlignShift is the maximum number of 50ms frames to slide when looking
// for the best-aligned cosine similarity. 10 frames = ±500ms, which covers the
// typical recording start/stop timing drift between simulcast recorders.
const energyAlignShift = 10

// EnergyFingerprintSimilarity returns the best cosine similarity between two
// normalised energy profiles across a ±500ms sliding alignment window.
// This handles the case where two simulcast recorders capture the same P25
// transmission but start/stop at slightly different times (up to ~200-500ms
// offset), which would otherwise collapse the raw cosine score even though the
// underlying audio content is identical.
func EnergyFingerprintSimilarity(a, b []float64) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	best := 0.0
	for shift := -energyAlignShift; shift <= energyAlignShift; shift++ {
		var oa, ob int
		if shift >= 0 {
			oa = shift
		} else {
			ob = -shift
		}
		length := len(a) - oa
		if lb := len(b) - ob; lb < length {
			length = lb
		}
		if length < energyMinFrames {
			continue
		}
		var dot, magA, magB float64
		for i := 0; i < length; i++ {
			va, vb := a[oa+i], b[ob+i]
			dot += va * vb
			magA += va * va
			magB += vb * vb
		}
		if magA == 0 || magB == 0 {
			continue
		}
		if sim := dot / (math.Sqrt(magA) * math.Sqrt(magB)); sim > best {
			best = sim
		}
	}
	return best
}

// ── PCM content hash ─────────────────────────────────────────────────────────

// ComputeAudioHash decodes audio to raw PCM using ffmpeg (same pipeline as the
// energy fingerprint) and returns a SHA-256 hex digest of the raw samples.
// Because the hash is computed on decoded PCM — not the container bytes — it is
// independent of codec, container format, and metadata. Two recordings of the
// exact same audio signal (same samples) always produce the same hash.
//
// This is used as the first, cheapest duplicate check: if the hash matches an
// existing call it is a bit-perfect duplicate and no further fingerprinting is
// needed. If it doesn't match, the energy/Chromaprint paths still run to catch
// near-duplicates from different recording chains.
func ComputeAudioHash(audio []byte, mime string) (string, error) {
	ext := audioExtFromMime(mime)
	tmp, err := os.CreateTemp("", "tlr-hash-*"+ext)
	if err != nil {
		return "", fmt.Errorf("audio hash: create temp: %w", err)
	}
	defer os.Remove(tmp.Name())

	if _, err := tmp.Write(audio); err != nil {
		tmp.Close()
		return "", fmt.Errorf("audio hash: write temp: %w", err)
	}
	tmp.Close()

	cmd := exec.Command("ffmpeg",
		"-i", tmp.Name(),
		"-f", "s16le",
		"-ar", fmt.Sprintf("%d", energySampleHz),
		"-ac", "1",
		"-loglevel", "quiet",
		"pipe:1",
	)
	pcm, err := cmd.Output()
	if err != nil || len(pcm) == 0 {
		return "", fmt.Errorf("audio hash: ffmpeg decode: %w", err)
	}

	sum := sha256.Sum256(pcm)
	return hex.EncodeToString(sum[:]), nil
}

// ── Shared ────────────────────────────────────────────────────────────────────

func audioExtFromMime(mime string) string {
	switch {
	case strings.Contains(mime, "mp3") || strings.Contains(mime, "mpeg"):
		return ".mp3"
	case strings.Contains(mime, "mp4") || strings.Contains(mime, "m4a") || strings.Contains(mime, "aac"):
		return ".m4a"
	case strings.Contains(mime, "ogg"):
		return ".ogg"
	case strings.Contains(mime, "wav"):
		return ".wav"
	default:
		return ".mp3"
	}
}
