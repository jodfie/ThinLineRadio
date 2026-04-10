// Standalone tool to compute actual energy similarity scores for false positive
// duplicate detections. Connects to the dev DB, pulls audio for each false
// positive call and its likely match (nearest verified-dup on same talkgroup),
// and prints the similarity scores so we can pick the right threshold.
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

	_ "github.com/lib/pq"
	"database/sql"
)

const (
	energyThreshold    = 0.80
	energyFrameMs      = 50
	energySampleHz     = 8000
	energyMinFrames    = 4
	energyAlignShift   = 10
	durationRatioMin   = 0.85

	dsn = "host=localhost port=5432 dbname=rdio_scanner user=michaelchambers password=asdfasd5456456df sslmode=disable"
)

func main() {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "db open: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	// Pull all false positive calls (isDuplicate=true, verifiedDuplicate=false) <= 200
	fpRows, err := db.Query(`
		SELECT "callId", "talkgroupRef", "timestamp", "audioDuration", "audio", "audioMime"
		FROM "calls"
		WHERE "callId" <= 200
		  AND "isDuplicate" = true
		  AND "verifiedDuplicate" = false
		ORDER BY "talkgroupRef", "timestamp"`)
	if err != nil {
		fmt.Fprintf(os.Stderr, "query fp: %v\n", err)
		os.Exit(1)
	}
	defer fpRows.Close()

	type callRec struct {
		id       int64
		tgRef    int
		ts       int64
		dur      float64
		audio    []byte
		mime     string
	}

	var fps []callRec
	for fpRows.Next() {
		var c callRec
		if err := fpRows.Scan(&c.id, &c.tgRef, &c.ts, &c.dur, &c.audio, &c.mime); err != nil {
			continue
		}
		fps = append(fps, c)
	}

	fmt.Printf("%-8s %-8s %-14s %-7s   %-8s %-8s %-14s %-7s   %s\n",
		"FP_ID", "TG", "FP_TS", "FP_DUR",
		"MATCH_ID", "MATCH_TG", "MATCH_TS", "MATCH_DUR",
		"SIMILARITY")
	fmt.Println(strings.Repeat("-", 110))

	for _, fp := range fps {
		// Find all verified-dup calls on same talkgroup within ±120s
		candRows, err := db.Query(`
			SELECT "callId", "talkgroupRef", "timestamp", "audioDuration", "audio", "audioMime"
			FROM "calls"
			WHERE "callId" <= 200
			  AND "talkgroupRef" = $1
			  AND "callId" != $2
			  AND ABS("timestamp" - $3) <= 120000
			  AND "audioDuration" > 0
			ORDER BY ABS("timestamp" - $3)`,
			fp.tgRef, fp.id, fp.ts)
		if err != nil {
			continue
		}

		fpProfile, err := computeEnergyProfile(fp.audio, fp.mime)
		fpHash := audioHash(fp.audio, fp.mime)

		var bestSim float64
		var bestID int64
		var bestTS int64
		var bestDur float64
		var bestTG int

		for candRows.Next() {
			var c callRec
			if err := candRows.Scan(&c.id, &c.tgRef, &c.ts, &c.dur, &c.audio, &c.mime); err != nil {
				continue
			}

			// Duration ratio filter
			lo, hi := fp.dur, c.dur
			if hi < lo { lo, hi = hi, lo }
			if hi == 0 || lo/hi < durationRatioMin { continue }

			// Hash check
			cHash := audioHash(c.audio, c.mime)
			if fpHash != "" && cHash != "" && fpHash == cHash {
				fmt.Printf("%-8d %-8d %-14d %-7.3f   %-8d %-8d %-14d %-7.3f   HASH_MATCH\n",
					fp.id, fp.tgRef, fp.ts, fp.dur,
					c.id, c.tgRef, c.ts, c.dur)
				continue
			}

			if err != nil || len(fpProfile) == 0 { continue }
			candProfile, err := computeEnergyProfile(c.audio, c.mime)
			if err != nil || len(candProfile) == 0 { continue }

			sim := energySimilarity(fpProfile, candProfile)
			if sim > bestSim {
				bestSim = sim
				bestID = c.id
				bestTS = c.ts
				bestDur = c.dur
				bestTG = c.tgRef
			}
		}
		candRows.Close()

		if bestID > 0 {
			marker := ""
			if bestSim >= energyThreshold {
				marker = " ← WOULD FLAG"
			}
			fmt.Printf("%-8d %-8d %-14d %-7.3f   %-8d %-8d %-14d %-7.3f   %.4f%s\n",
				fp.id, fp.tgRef, fp.ts, fp.dur,
				bestID, bestTG, bestTS, bestDur,
				bestSim, marker)
		}
	}

	fmt.Println()
	fmt.Println("Now checking TRUE POSITIVES to see their similarity floor...")
	fmt.Println(strings.Repeat("-", 110))

	// Sample of true positives — find their pair and score
	tpRows, err := db.Query(`
		SELECT a."callId", a."talkgroupRef", a."timestamp", a."audioDuration", a."audio", a."audioMime"
		FROM "calls" a
		WHERE a."callId" <= 200
		  AND a."isDuplicate" = true
		  AND a."verifiedDuplicate" = true
		ORDER BY a."talkgroupRef", a."timestamp"
		LIMIT 30`)
	if err != nil {
		fmt.Fprintf(os.Stderr, "query tp: %v\n", err)
		os.Exit(1)
	}
	defer tpRows.Close()

	for tpRows.Next() {
		var c callRec
		if err := tpRows.Scan(&c.id, &c.tgRef, &c.ts, &c.dur, &c.audio, &c.mime); err != nil {
			continue
		}

		candRows, err := db.Query(`
			SELECT "callId", "talkgroupRef", "timestamp", "audioDuration", "audio", "audioMime"
			FROM "calls"
			WHERE "callId" <= 200
			  AND "talkgroupRef" = $1
			  AND "callId" != $2
			  AND ABS("timestamp" - $3) <= 120000
			  AND "audioDuration" > 0
			  AND "verifiedDuplicate" = true
			ORDER BY ABS("timestamp" - $3)
			LIMIT 5`,
			c.tgRef, c.id, c.ts)
		if err != nil { continue }

		profile, err := computeEnergyProfile(c.audio, c.mime)

		for candRows.Next() {
			var cand callRec
			if err := candRows.Scan(&cand.id, &cand.tgRef, &cand.ts, &cand.dur, &cand.audio, &cand.mime); err != nil {
				continue
			}
			lo, hi := c.dur, cand.dur
			if hi < lo { lo, hi = hi, lo }
			if hi == 0 || lo/hi < durationRatioMin { continue }

			if err != nil || len(profile) == 0 { continue }
			candProfile, err := computeEnergyProfile(cand.audio, cand.mime)
			if err != nil || len(candProfile) == 0 { continue }

			sim := energySimilarity(profile, candProfile)
			fmt.Printf("TP %-6d %-8d %-14d %-7.3f   %-8d %-8d %-14d %-7.3f   %.4f\n",
				c.id, c.tgRef, c.ts, c.dur,
				cand.id, cand.tgRef, cand.ts, cand.dur, sim)
			break
		}
		candRows.Close()
	}
}

func audioHash(audio []byte, mime string) string {
	pcm, err := decodePCM(audio, mime)
	if err != nil || len(pcm) == 0 { return "" }
	sum := sha256.Sum256(pcm)
	return hex.EncodeToString(sum[:])
}

func computeEnergyProfile(audio []byte, mime string) ([]float64, error) {
	pcm, err := decodePCM(audio, mime)
	if err != nil { return nil, err }

	const samplesPerFrame = energySampleHz * energyFrameMs / 1000
	const bytesPerFrame = samplesPerFrame * 2

	numFrames := len(pcm) / bytesPerFrame
	if numFrames < energyMinFrames {
		return nil, fmt.Errorf("too short (%d frames)", numFrames)
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
		if v > maxVal { maxVal = v }
	}
	if maxVal > 0 {
		for i := range profile { profile[i] /= maxVal }
	}
	return profile, nil
}

func decodePCM(audio []byte, mime string) ([]byte, error) {
	ext := ".mp3"
	if strings.Contains(mime, "mp4") || strings.Contains(mime, "m4a") || strings.Contains(mime, "aac") {
		ext = ".m4a"
	}
	tmp, err := os.CreateTemp("", "tlr-analyze-*"+ext)
	if err != nil { return nil, err }
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(audio); err != nil { tmp.Close(); return nil, err }
	tmp.Close()

	cmd := exec.Command("ffmpeg", "-i", tmp.Name(), "-f", "s16le",
		"-ar", fmt.Sprintf("%d", energySampleHz), "-ac", "1", "-loglevel", "quiet", "pipe:1")
	return cmd.Output()
}

func energySimilarity(a, b []float64) float64 {
	if len(a) == 0 || len(b) == 0 { return 0 }
	best := 0.0
	for shift := -energyAlignShift; shift <= energyAlignShift; shift++ {
		var oa, ob int
		if shift >= 0 { oa = shift } else { ob = -shift }
		length := len(a) - oa
		if lb := len(b) - ob; lb < length { length = lb }
		if length < energyMinFrames { continue }
		var dot, magA, magB float64
		for i := 0; i < length; i++ {
			va, vb := a[oa+i], b[ob+i]
			dot += va * vb
			magA += va * va
			magB += vb * vb
		}
		if magA == 0 || magB == 0 { continue }
		if sim := dot / (math.Sqrt(magA) * math.Sqrt(magB)); sim > best { best = sim }
	}
	return best
}
