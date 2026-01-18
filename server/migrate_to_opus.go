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
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"path"
	"strings"
	"sync"
	"time"
)

// MigrateToOpus converts all existing M4A/AAC audio in the database to Opus format
// This provides ~50% storage savings and better voice quality at lower bitrates
func (db *Database) MigrateToOpus(batchSize int, dryRun bool, autoConfirm bool) error {
	if db.Sql == nil {
		return fmt.Errorf("database connection is nil")
	}

	// Check if FFmpeg is available and supports Opus
	if err := checkOpusSupport(); err != nil {
		return fmt.Errorf("FFmpeg Opus support check failed: %v", err)
	}

	fmt.Println("=================================================================")
	fmt.Println("                    OPUS MIGRATION TOOL")
	fmt.Println("=================================================================")
	fmt.Println("")

	if dryRun {
		fmt.Println("🔍 DRY RUN MODE - No changes will be made")
	} else {
		fmt.Println("⚠️  LIVE MODE - Database will be modified")
	}
	fmt.Println("")

	// Count total calls to migrate
	var totalCalls int
	var m4aCalls int
	var aacCalls int
	var mp4Calls int
	var mp3Calls int

	if db.Config.DbType == DbTypePostgresql {
		db.Sql.QueryRow(`SELECT COUNT(*) FROM "calls" WHERE "audioMime" IN ('audio/mp4', 'audio/m4a', 'audio/aac', 'audio/x-m4a', 'audio/mpeg', 'audio/mp3')`).Scan(&totalCalls)
		db.Sql.QueryRow(`SELECT COUNT(*) FROM "calls" WHERE "audioMime" = 'audio/m4a'`).Scan(&m4aCalls)
		db.Sql.QueryRow(`SELECT COUNT(*) FROM "calls" WHERE "audioMime" = 'audio/aac'`).Scan(&aacCalls)
		db.Sql.QueryRow(`SELECT COUNT(*) FROM "calls" WHERE "audioMime" IN ('audio/mp4', 'audio/x-m4a')`).Scan(&mp4Calls)
		db.Sql.QueryRow(`SELECT COUNT(*) FROM "calls" WHERE "audioMime" IN ('audio/mpeg', 'audio/mp3')`).Scan(&mp3Calls)
	} else {
		db.Sql.QueryRow(`SELECT COUNT(*) FROM "calls" WHERE "audioMime" IN ('audio/mp4', 'audio/m4a', 'audio/aac', 'audio/x-m4a', 'audio/mpeg', 'audio/mp3')`).Scan(&totalCalls)
		db.Sql.QueryRow(`SELECT COUNT(*) FROM "calls" WHERE "audioMime" = 'audio/m4a'`).Scan(&m4aCalls)
		db.Sql.QueryRow(`SELECT COUNT(*) FROM "calls" WHERE "audioMime" = 'audio/aac'`).Scan(&aacCalls)
		db.Sql.QueryRow(`SELECT COUNT(*) FROM "calls" WHERE "audioMime" IN ('audio/mp4', 'audio/x-m4a')`).Scan(&mp4Calls)
		db.Sql.QueryRow(`SELECT COUNT(*) FROM "calls" WHERE "audioMime" IN ('audio/mpeg', 'audio/mp3')`).Scan(&mp3Calls)
	}

	fmt.Printf("📊 Found %d calls to migrate:\n", totalCalls)
	fmt.Printf("   - audio/m4a:  %d calls\n", m4aCalls)
	fmt.Printf("   - audio/mp4:  %d calls\n", mp4Calls)
	fmt.Printf("   - audio/aac:  %d calls\n", aacCalls)
	fmt.Printf("   - audio/mp3:  %d calls\n", mp3Calls)
	fmt.Println("")

	if totalCalls == 0 {
		fmt.Println("✅ No calls need migration - all done!")
		return nil
	}

	// Calculate estimated storage savings
	var totalSize int64
	if db.Config.DbType == DbTypePostgresql {
		db.Sql.QueryRow(`SELECT SUM(length("audio")) FROM "calls" WHERE "audioMime" IN ('audio/mp4', 'audio/m4a', 'audio/aac', 'audio/x-m4a', 'audio/mpeg', 'audio/mp3')`).Scan(&totalSize)
	} else {
		db.Sql.QueryRow(`SELECT SUM(length("audio")) FROM "calls" WHERE "audioMime" IN ('audio/mp4', 'audio/m4a', 'audio/aac', 'audio/x-m4a', 'audio/mpeg', 'audio/mp3')`).Scan(&totalSize)
	}

	estimatedSavings := float64(totalSize) * 0.5 // 50% savings expected
	fmt.Printf("💾 Current storage: %.2f MB\n", float64(totalSize)/(1024*1024))
	fmt.Printf("💰 Estimated savings: %.2f MB (50%%)\n", estimatedSavings/(1024*1024))
	fmt.Printf("📦 Final size: %.2f MB\n", float64(totalSize-int64(estimatedSavings))/(1024*1024))
	fmt.Println("")

	if dryRun {
		fmt.Println("✅ Dry run complete - no changes made")
		return nil
	}

	// Confirm migration
	fmt.Println("⏱️  Estimated time: ~" + estimateTime(totalCalls))
	fmt.Println("")
	fmt.Println("⚠️  WARNING: This operation will modify your database!")
	fmt.Println("⚠️  Please ensure you have a backup before proceeding.")
	fmt.Println("")

	if !autoConfirm {
		fmt.Print("Continue with migration? (yes/no): ")

		var response string
		fmt.Scanln(&response)
		if strings.ToLower(response) != "yes" {
			fmt.Println("❌ Migration cancelled")
			return nil
		}
	} else {
		fmt.Println("✅ Auto-confirmed (opus_migration from INI file)")
	}

	fmt.Println("")
	fmt.Println("🚀 Starting migration...")
	fmt.Println("")

	// Process in batches
	// NOTE: We use LIMIT without OFFSET because the WHERE clause changes as we convert
	// Always select the first batch of unconverted files
	migrated := 0
	failed := 0
	skipped := 0
	totalSaved := int64(0)
	startTime := time.Now()

	for migrated+failed+skipped < totalCalls {
		var query string
		// Always get first N unconverted files (no OFFSET needed since they're converted as we go)
		if db.Config.DbType == DbTypePostgresql {
			query = fmt.Sprintf(`SELECT "callId", "audio", "audioFilename", "audioMime" FROM "calls" WHERE "audioMime" IN ('audio/mp4', 'audio/m4a', 'audio/aac', 'audio/x-m4a', 'audio/mpeg', 'audio/mp3') ORDER BY "callId" LIMIT %d`, batchSize)
		} else {
			query = fmt.Sprintf(`SELECT "callId", "audio", "audioFilename", "audioMime" FROM "calls" WHERE "audioMime" IN ('audio/mp4', 'audio/m4a', 'audio/aac', 'audio/x-m4a', 'audio/mpeg', 'audio/mp3') ORDER BY "callId" LIMIT %d`, batchSize)
		}

		rows, err := db.Sql.Query(query)
		if err != nil {
			fmt.Printf("❌ Error querying batch: %v\n", err)
			continue
		}

		batchCount := 0
		type convertJob struct {
			callId   uint64
			audio    []byte
			filename string
			mimeType string
		}
		var jobs []convertJob

		for rows.Next() {
			var callId uint64
			var audio []byte
			var filename string
			var mimeType string

			if err := rows.Scan(&callId, &audio, &filename, &mimeType); err != nil {
				fmt.Printf("❌ Error scanning row: %v\n", err)
				failed++
				continue
			}

			batchCount++

			// Skip if already Opus (shouldn't happen, but safe)
			if mimeType == "audio/opus" {
				skipped++
				continue
			}

			jobs = append(jobs, convertJob{callId, audio, filename, mimeType})
		}
		rows.Close()

		// If no rows were returned, we're done (all calls converted)
		if batchCount == 0 {
			break
		}

		// Process conversions in parallel using worker pool
		// Adjust workers based on batch size:
		// - Small batches (<=100): 1 worker for ultra-gentle background processing (can run for days)
		// - Medium batches (<=1000): 50 workers
		// - Large batches (>1000): 200 workers for maximum speed
		numWorkers := 200
		if batchSize <= 100 {
			numWorkers = 1
		} else if batchSize <= 1000 {
			numWorkers = 50
		}

		jobChan := make(chan convertJob, len(jobs))
		resultChan := make(chan struct {
			callId      uint64
			opusAudio   []byte
			newFilename string
			originalLen int
			err         error
		}, len(jobs))

		// Start workers
		var wg sync.WaitGroup
		for i := 0; i < numWorkers; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				defer func() {
					// Recover from panics to prevent worker death
					if r := recover(); r != nil {
						fmt.Printf("⚠️  Worker panic recovered: %v\n", r)
					}
				}()
				for job := range jobChan {
					// Convert to Opus (with timeout protection)
					opusAudio, err := convertToOpus(job.audio)
					newFilename := strings.TrimSuffix(job.filename, path.Ext(job.filename)) + ".opus"
					resultChan <- struct {
						callId      uint64
						opusAudio   []byte
						newFilename string
						originalLen int
						err         error
					}{job.callId, opusAudio, newFilename, len(job.audio), err}
				}
			}()
		}

		// Send jobs to workers
		for _, job := range jobs {
			jobChan <- job
		}
		close(jobChan)

		// Wait for all workers to finish
		go func() {
			wg.Wait()
			close(resultChan)
		}()

		// Collect results and batch database updates
		var updateBatch []struct {
			callId      uint64
			opusAudio   []byte
			newFilename string
			originalLen int
		}
		// Adjust DB batch size based on conversion batch size:
		// - Small batches: write 1 at a time (minimal DB impact)
		// - Medium batches: write 20 at a time
		// - Large batches: write 50 at a time
		batchUpdateSize := 20
		if batchSize <= 100 {
			batchUpdateSize = 1
		} else if batchSize <= 1000 {
			batchUpdateSize = 20
		} else {
			batchUpdateSize = 50
		}

		resultsProcessed := 0
		for result := range resultChan {
			resultsProcessed++

			if result.err != nil {
				// Silently skip failed conversions to avoid log spam
				failed++

				// Progress heartbeat every 100 results (including failures)
				if resultsProcessed%100 == 0 {
					fmt.Printf("⏳ Processed %d results (pending DB write)...\n", resultsProcessed)
				}
				continue
			}

			// Add to batch
			updateBatch = append(updateBatch, struct {
				callId      uint64
				opusAudio   []byte
				newFilename string
				originalLen int
			}{result.callId, result.opusAudio, result.newFilename, result.originalLen})

			// When batch is full, write to database
			if len(updateBatch) >= batchUpdateSize {
				fmt.Printf("💾 Writing batch of %d calls to database...\n", len(updateBatch))

				if err := db.batchUpdateCalls(updateBatch); err != nil {
					fmt.Printf("❌ Batch update failed: %v\n", err)
					failed += len(updateBatch)
					updateBatch = nil
					continue
				}

				// Track savings and progress
				for _, item := range updateBatch {
					saved := item.originalLen - len(item.opusAudio)
					totalSaved += int64(saved)
					migrated++
				}

				// Progress update every 100 calls
				if migrated%100 == 0 {
					elapsed := time.Since(startTime)
					rate := float64(migrated) / elapsed.Seconds()
					remaining := int(float64(totalCalls-migrated) / rate)
					fmt.Printf("✅ Progress: %d/%d (%.1f%%) | Saved: %.2f MB | Rate: %.0f/sec | ETA: %s\n",
						migrated, totalCalls,
						float64(migrated)/float64(totalCalls)*100,
						float64(totalSaved)/(1024*1024),
						rate,
						time.Duration(remaining)*time.Second)
				}

				// Clear batch
				updateBatch = nil
			}
		}

		// Write remaining items in batch
		if len(updateBatch) > 0 {
			if err := db.batchUpdateCalls(updateBatch); err != nil {
				fmt.Printf("❌ Final batch update failed: %v\n", err)
				failed += len(updateBatch)
			} else {
				for _, item := range updateBatch {
					saved := item.originalLen - len(item.opusAudio)
					totalSaved += int64(saved)
					migrated++
				}
			}
		}
	}

	fmt.Println("")
	fmt.Println("=================================================================")
	fmt.Println("                    MIGRATION COMPLETE")
	fmt.Println("=================================================================")
	fmt.Printf("✅ Migrated: %d calls\n", migrated)
	fmt.Printf("❌ Failed: %d calls\n", failed)
	fmt.Printf("⏭️  Skipped: %d calls\n", skipped)
	fmt.Printf("💾 Space saved: %.2f MB (%.1f%%)\n",
		float64(totalSaved)/(1024*1024),
		float64(totalSaved)/float64(totalSize)*100)
	fmt.Printf("⏱️  Total time: %s\n", time.Since(startTime).Round(time.Second))
	fmt.Println("")

	// Automatically run VACUUM FULL for PostgreSQL
	if db.Config.DbType == DbTypePostgresql {
		fmt.Println("🔧 Running VACUUM FULL to reclaim disk space...")
		fmt.Println("   (This may take several minutes depending on database size)")
		fmt.Println("")

		vacuumStart := time.Now()
		if _, err := db.Sql.Exec(`VACUUM FULL "calls"`); err != nil {
			fmt.Printf("⚠️  Warning: VACUUM FULL failed: %v\n", err)
			fmt.Println("💡 You can manually run: psql -d yourdb -c 'VACUUM FULL calls;'")
		} else {
			fmt.Printf("✅ VACUUM FULL completed in %s\n", time.Since(vacuumStart).Round(time.Second))
			fmt.Println("💾 Disk space has been reclaimed")
		}
		fmt.Println("")
	}

	return nil
}

// batchUpdateCalls updates multiple calls in a single transaction
func (db *Database) batchUpdateCalls(batch []struct {
	callId      uint64
	opusAudio   []byte
	newFilename string
	originalLen int
}) error {
	// Start transaction with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	tx, err := db.Sql.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %v", err)
	}
	defer tx.Rollback()

	// Execute updates without preparing (faster for PostgreSQL)
	for _, item := range batch {
		var err error
		if db.Config.DbType == DbTypePostgresql {
			_, err = tx.ExecContext(ctx, `UPDATE "calls" SET "audio" = $1, "audioFilename" = $2, "audioMime" = 'audio/opus' WHERE "callId" = $3`, item.opusAudio, item.newFilename, item.callId)
		} else {
			_, err = tx.ExecContext(ctx, `UPDATE "calls" SET "audio" = ?, "audioFilename" = ?, "audioMime" = 'audio/opus' WHERE "callId" = ?`, item.opusAudio, item.newFilename, item.callId)
		}
		if err != nil {
			return fmt.Errorf("failed to execute update for call %d: %v", item.callId, err)
		}
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %v", err)
	}

	return nil
}

// convertToOpus converts audio bytes to Opus format using FFmpeg
func convertToOpus(audio []byte) ([]byte, error) {
	args := []string{
		"-y", "-loglevel", "error",
		"-i", "pipe:0", // Read from stdin
		"-ar", "16000", // 16kHz sample rate
		"-ac", "1", // Mono
		"-c:a", "libopus",
		"-b:a", "16k", // 16 kbps
		"-vbr", "on", // Variable bitrate
		"-application", "voip", // Voice optimization
		"-compression_level", "10", // Max compression
		"-f", "opus", // Opus format
		"pipe:1", // Write to stdout
	}

	cmd := exec.Command("ffmpeg", args...)
	cmd.Stdin = bytes.NewReader(audio)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Add timeout to prevent hanging
	done := make(chan error, 1)
	go func() {
		done <- cmd.Run()
	}()

	select {
	case err := <-done:
		if err != nil {
			// Skip detailed error output to avoid spam
			return nil, fmt.Errorf("ffmpeg conversion failed")
		}
	case <-time.After(10 * time.Second):
		// Kill process if it takes too long
		cmd.Process.Kill()
		return nil, fmt.Errorf("ffmpeg timeout after 10 seconds")
	}

	if stdout.Len() == 0 {
		return nil, fmt.Errorf("ffmpeg produced no output")
	}

	return stdout.Bytes(), nil
}

// checkOpusSupport verifies FFmpeg can encode Opus
func checkOpusSupport() error {
	cmd := exec.Command("ffmpeg", "-encoders")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ffmpeg not found or not executable")
	}

	output := stdout.String()
	if !strings.Contains(output, "libopus") {
		return fmt.Errorf("FFmpeg does not have libopus encoder support. Please install ffmpeg with libopus.")
	}

	return nil
}

// estimateTime estimates how long the migration will take
func estimateTime(totalCalls int) string {
	// Estimate ~0.5 seconds per call (conservative)
	seconds := totalCalls / 2
	if seconds < 60 {
		return fmt.Sprintf("%d seconds", seconds)
	}
	if seconds < 3600 {
		return fmt.Sprintf("%d minutes", seconds/60)
	}
	return fmt.Sprintf("%.1f hours", float64(seconds)/3600)
}
