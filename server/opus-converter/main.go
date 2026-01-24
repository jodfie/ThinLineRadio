// Copyright (C) 2025 Thinline Dynamic Solutions
//
// Standalone Opus Audio Converter for Thinline Radio
// Converts M4A/AAC/MP3 audio in database to Opus format
//
// Usage:
//   opus-converter.exe --host localhost --port 5432 --db rdio_scanner --user username --pass password
//   opus-converter.exe --dry-run  (preview without converting)
//   opus-converter.exe --batch 100  (process 100 calls at a time)
//   opus-converter.exe --auto-confirm  (skip confirmation prompt)

package main

import (
	"bufio"
	"bytes"
	"database/sql"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	_ "github.com/lib/pq"
)

var (
	iniFile      = flag.String("ini", "thinline-radio.ini", "Path to INI file")
	batchSize    = flag.Int("batch", 1000, "Batch size (100=gentle, 1000=normal, 5000=fast)")
	dryRun       = flag.Bool("dry-run", false, "Preview only, don't convert")
	autoConfirm  = flag.Bool("auto-confirm", false, "Skip confirmation prompt")
)

func main() {
	flag.Parse()

	// Create error log file
	errorLog, err := os.Create("opus-converter-errors.log")
	if err != nil {
		fmt.Printf("Warning: Could not create error log: %v\n", err)
	} else {
		defer errorLog.Close()
	}

	fmt.Println("╔════════════════════════════════════════════════════════╗")
	fmt.Println("║   Thinline Radio - Opus Audio Converter v7.0          ║")
	fmt.Println("║   50% storage savings, better voice quality           ║")
	fmt.Println("╚════════════════════════════════════════════════════════╝")
	fmt.Println()

	// Read INI file
	config, err := readINI(*iniFile)
	if err != nil {
		fmt.Printf("❌ Error reading INI file: %v\n", err)
		fmt.Println()
		fmt.Println("Usage:")
		fmt.Println("  Place opus-converter.exe next to thinline-radio.ini")
		fmt.Println("  Or: opus-converter --ini /path/to/thinline-radio.ini")
		os.Exit(1)
	}

	fmt.Printf("✅ Loaded configuration from %s\n", *iniFile)
	fmt.Printf("   Database: %s@%s:%s/%s\n", config["db_user"], config["db_host"], config["db_port"], config["db_name"])
	fmt.Println()

	// Check ffmpeg
	if err := checkOpusSupport(); err != nil {
		fmt.Printf("❌ Error: %v\n", err)
		fmt.Println()
		fmt.Println("Please install ffmpeg with libopus support:")
		fmt.Println("  - Windows: Download from https://ffmpeg.org/download.html")
		fmt.Println("  - Linux: sudo apt install ffmpeg")
		fmt.Println("  - macOS: brew install ffmpeg")
		os.Exit(1)
	}
	fmt.Println("✅ FFmpeg with Opus support detected")
	fmt.Println()

	// Connect to database
	connStr := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		config["db_host"], config["db_port"], config["db_user"], config["db_pass"], config["db_name"])

	db, err := sql.Open("postgres", connStr)
	if err != nil {
		fmt.Printf("❌ Error connecting to database: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	// Set connection pool limits to match database capacity
	db.SetMaxOpenConns(50)   // Limit to 50 concurrent connections
	db.SetMaxIdleConns(10)   // Keep 10 idle connections ready
	db.SetConnMaxLifetime(5 * time.Minute)

	if err := db.Ping(); err != nil {
		fmt.Printf("❌ Error connecting to database: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("✅ Database connection successful (max 50 concurrent)")
	fmt.Println()

	// Run migration
	if err := migrateToOpus(db, *batchSize, *dryRun, *autoConfirm); err != nil {
		fmt.Printf("❌ Migration failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Println()
	fmt.Println("✅ Migration complete!")
	fmt.Println()
	fmt.Println("Next steps:")
	fmt.Println("  1. Run VACUUM FULL on your database to reclaim disk space")
	fmt.Println("  2. Restart Thinline Radio server")
}

func migrateToOpus(db *sql.DB, batchSize int, dryRun bool, autoConfirm bool) error {
	// Count calls to convert
	var totalCalls int
	query := `SELECT COUNT(*) FROM "calls" WHERE "audioMime" IN ('audio/mp4', 'audio/m4a', 'audio/aac', 'audio/x-m4a', 'audio/mpeg', 'audio/mp3')`
	if err := db.QueryRow(query).Scan(&totalCalls); err != nil {
		return fmt.Errorf("error counting calls: %v", err)
	}

	if totalCalls == 0 {
		fmt.Println("✅ No calls need conversion - all audio is already Opus!")
		return nil
	}

	fmt.Printf("📊 Found %d calls to convert\n", totalCalls)

	// Calculate storage info
	var totalSize int64
	db.QueryRow(`SELECT SUM(length("audio")) FROM "calls" WHERE "audioMime" IN ('audio/mp4', 'audio/m4a', 'audio/aac', 'audio/x-m4a', 'audio/mpeg', 'audio/mp3')`).Scan(&totalSize)

	estimatedSavings := float64(totalSize) * 0.5
	fmt.Printf("💾 Current storage: %.2f MB\n", float64(totalSize)/(1024*1024))
	fmt.Printf("💰 Estimated savings: %.2f MB (50%%)\n", estimatedSavings/(1024*1024))
	fmt.Printf("📦 Final size: %.2f MB\n", float64(totalSize-int64(estimatedSavings))/(1024*1024))
	fmt.Println()

	if dryRun {
		fmt.Println("✅ Dry run complete - no changes made")
		return nil
	}

	// Estimate time
	fmt.Printf("⏱️  Estimated time: %s\n", estimateTime(totalCalls))
	fmt.Println()
	fmt.Println("⚠️  WARNING: This operation will modify your database!")
	fmt.Println("⚠️  Please ensure you have a backup before proceeding.")
	fmt.Println()

	if !autoConfirm {
		fmt.Print("Continue with migration? (yes/no): ")
		var response string
		fmt.Scanln(&response)
		if strings.ToLower(response) != "yes" {
			fmt.Println("❌ Migration cancelled")
			return nil
		}
	}

	fmt.Println()
	fmt.Println("🚀 Starting migration...")
	fmt.Println()

	// Process in batches
	migrated := 0
	failed := 0
	skipped := 0
	totalSaved := int64(0)
	startTime := time.Now()

	for migrated+failed+skipped < totalCalls {
		query := fmt.Sprintf(`SELECT "callId", "audio", "audioFilename", "audioMime" FROM "calls" WHERE "audioMime" IN ('audio/mp4', 'audio/m4a', 'audio/aac', 'audio/x-m4a', 'audio/mpeg', 'audio/mp3') ORDER BY "callId" LIMIT %d`, batchSize)

		rows, err := db.Query(query)
		if err != nil {
			fmt.Printf("❌ Error querying batch: %v\n", err)
			continue
		}

		type convertJob struct {
			callId   uint64
			audio    []byte
			filename string
			mimeType string
		}
		var jobs []convertJob
		batchCount := 0

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

			if mimeType == "audio/opus" {
				skipped++
				continue
			}

			jobs = append(jobs, convertJob{callId, audio, filename, mimeType})
		}
		rows.Close()

		if batchCount == 0 {
			break
		}

		// Worker pool - 10 workers for reliable conversion
		numWorkers := 10

		jobChan := make(chan convertJob, len(jobs))
		var wg sync.WaitGroup
		var mu sync.Mutex

		// Start workers
		for i := 0; i < numWorkers; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for job := range jobChan {
					originalSize := len(job.audio)
					opusAudio, err := convertToOpus(job.audio)
					if err != nil {
						fmt.Printf("\n❌ Call %d conversion failed: %v (skipping)\n", job.callId, err)
						// Mark as "failed" by setting to a dummy opus mime so it won't be retried
						skipQuery := `UPDATE "calls" SET "audioMime" = 'audio/opus-failed' WHERE "callId" = $1`
						db.Exec(skipQuery, job.callId)
						mu.Lock()
						failed++
						mu.Unlock()
						continue
					}

					newSize := len(opusAudio)
					saved := originalSize - newSize

					// Update database
					newFilename := strings.TrimSuffix(job.filename, ".m4a")
					newFilename = strings.TrimSuffix(newFilename, ".mp3")
					newFilename = strings.TrimSuffix(newFilename, ".aac") + ".opus"

					updateQuery := `UPDATE "calls" SET "audio" = $1, "audioMime" = 'audio/opus', "audioFilename" = $2 WHERE "callId" = $3`
					if _, err := db.Exec(updateQuery, opusAudio, newFilename, job.callId); err != nil {
						fmt.Printf("\n❌ Call %d database update failed: %v\n", job.callId, err)
						mu.Lock()
						failed++
						mu.Unlock()
						continue
					}

					mu.Lock()
					migrated++
					totalSaved += int64(saved)
					mu.Unlock()
				}
			}()
		}

		// Send jobs
		for _, job := range jobs {
			jobChan <- job
		}
		close(jobChan)
		wg.Wait()

		// Progress update
		progress := float64(migrated+failed+skipped) / float64(totalCalls) * 100
		elapsed := time.Since(startTime)
		remaining := time.Duration(float64(elapsed) / float64(migrated+failed+skipped) * float64(totalCalls-(migrated+failed+skipped)))

		fmt.Printf("\r✓ %d migrated | ✗ %d failed | ⊘ %d skipped | %.1f%% | ETA: %s   ",
			migrated, failed, skipped, progress, remaining.Round(time.Second))
	}

	fmt.Println()
	fmt.Println()
	fmt.Printf("✅ Migration complete!\n")
	fmt.Printf("   Migrated: %d\n", migrated)
	fmt.Printf("   Failed: %d\n", failed)
	fmt.Printf("   Skipped: %d\n", skipped)
	fmt.Printf("   Space saved: %.2f MB\n", float64(totalSaved)/(1024*1024))
	fmt.Printf("   Time taken: %s\n", time.Since(startTime).Round(time.Second))

	return nil
}

func convertToOpus(audio []byte) ([]byte, error) {
	args := []string{
		"-y", "-loglevel", "error",
		"-i", "pipe:0",
		"-ar", "16000",
		"-ac", "1",
		"-c:a", "libopus",
		"-b:a", "16k",
		"-vbr", "on",
		"-application", "voip",
		"-compression_level", "10",
		"-f", "opus",
		"pipe:1",
	}

	cmd := exec.Command("ffmpeg", args...)
	cmd.Stdin = bytes.NewReader(audio)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	done := make(chan error, 1)
	go func() {
		done <- cmd.Run()
	}()

	select {
	case err := <-done:
		if err != nil {
			return nil, fmt.Errorf("ffmpeg conversion failed")
		}
	case <-time.After(10 * time.Second):
		cmd.Process.Kill()
		return nil, fmt.Errorf("ffmpeg timeout")
	}

	if stdout.Len() == 0 {
		return nil, fmt.Errorf("ffmpeg produced no output")
	}

	return stdout.Bytes(), nil
}

func checkOpusSupport() error {
	cmd := exec.Command("ffmpeg", "-encoders")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ffmpeg not found")
	}

	if !strings.Contains(stdout.String(), "libopus") {
		return fmt.Errorf("FFmpeg does not have libopus encoder")
	}

	return nil
}

func estimateTime(totalCalls int) string {
	seconds := totalCalls / 2
	if seconds < 60 {
		return fmt.Sprintf("%d seconds", seconds)
	}
	if seconds < 3600 {
		return fmt.Sprintf("%d minutes", seconds/60)
	}
	return fmt.Sprintf("%.1f hours", float64(seconds)/3600)
}

func readINI(filename string) (map[string]string, error) {
	config := make(map[string]string)

	file, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to open INI file: %v", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Parse key = value
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		config[key] = value
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading INI file: %v", err)
	}

	// Validate required fields
	required := []string{"db_host", "db_port", "db_name", "db_user", "db_pass"}
	for _, field := range required {
		if config[field] == "" {
			return nil, fmt.Errorf("missing required field in INI: %s", field)
		}
	}

	return config, nil
}

