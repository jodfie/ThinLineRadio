// Export LFD (78 LRDS FD / TG 46038) calls with audio for local tone-debug.
// Run on the production host with thinline-radio.ini or pass -dsn.
package main

import (
	"encoding/csv"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	_ "github.com/jackc/pgx/v5/stdlib"
	"gopkg.in/ini.v1"
	"database/sql"
)

const defaultTalkgroupRef = 46038

func main() {
	outDir := flag.String("out", "/tmp/tlr-debug/tlr-lfd-export", "output directory")
	iniPath := flag.String("ini", "thinline-radio.ini", "database config ini")
	dsn := flag.String("dsn", "", "postgres DSN (overrides ini)")
	limit := flag.Int("limit", 600, "max calls to export")
	talkgroupRef := flag.Int("talkgroup-ref", defaultTalkgroupRef, "talkgroupRef filter")
	flag.Parse()

	if *limit <= 0 {
		*limit = 600
	}
	if *limit > 5000 {
		*limit = 5000
	}

	connection := *dsn
	if connection == "" {
		cfg, err := ini.Load(*iniPath)
		if err != nil {
			fatalf("load ini %s: %v (or pass -dsn)", *iniPath, err)
		}
		sec := cfg.Section("")
		connection = fmt.Sprintf(
			"postgresql://%s:%s@%s:%d/%s",
			sec.Key("db_user").String(),
			sec.Key("db_pass").String(),
			sec.Key("db_host").String(),
			sec.Key("db_port").MustInt(5432),
			sec.Key("db_name").String(),
		)
	}

	db, err := sql.Open("pgx", connection)
	if err != nil {
		fatalf("db open: %v", err)
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		fatalf("db ping: %v", err)
	}

	audioDir := filepath.Join(*outDir, "audio")
	if err := os.MkdirAll(audioDir, 0o755); err != nil {
		fatalf("mkdir audio: %v", err)
	}

	rows, err := db.Query(`
		SELECT c."callId", c."systemId", c."talkgroupId", t."talkgroupRef", c."timestamp",
		       COALESCE(c."audioMime", ''), c."audioFilename", c."audio",
		       COALESCE(c."transcript", '')
		FROM "calls" c
		JOIN "talkgroups" t ON t."talkgroupId" = c."talkgroupId"
		WHERE t."talkgroupRef" = $1 AND length(c."audio") > 0
		ORDER BY c."timestamp" DESC
		LIMIT $2`, *talkgroupRef, *limit)
	if err != nil {
		fatalf("query: %v", err)
	}
	defer rows.Close()

	csvPath := filepath.Join(*outDir, fmt.Sprintf("calls_%d_last%d.csv", *talkgroupRef, *limit))
	csvFile, err := os.Create(csvPath)
	if err != nil {
		fatalf("create csv: %v", err)
	}
	w := csv.NewWriter(csvFile)
	_ = w.Write([]string{"callId", "systemId", "talkgroupId", "talkgroupRef", "timestamp", "audioMime", "audioFilename", "audio_bytes", "transcript"})

	exported := 0
	for rows.Next() {
		var (
			callID        uint64
			systemID      uint64
			talkgroupID   uint64
			tgRef         uint
			timestamp     int64
			audioMime     string
			audioFilename string
			audio         []byte
			transcript    string
		)
		if err := rows.Scan(&callID, &systemID, &talkgroupID, &tgRef, &timestamp, &audioMime, &audioFilename, &audio, &transcript); err != nil {
			fatalf("scan: %v", err)
		}
		if audioFilename == "" {
			audioFilename = fmt.Sprintf("%d.mp3", callID)
		}
		audioFilename = filepath.Base(strings.TrimSpace(audioFilename))
		outName := fmt.Sprintf("%d_%s", callID, audioFilename)
		if err := os.WriteFile(filepath.Join(audioDir, outName), audio, 0o644); err != nil {
			fatalf("write %s: %v", outName, err)
		}
		_ = w.Write([]string{
			strconv.FormatUint(callID, 10),
			strconv.FormatUint(systemID, 10),
			strconv.FormatUint(talkgroupID, 10),
			strconv.FormatUint(uint64(tgRef), 10),
			strconv.FormatInt(timestamp, 10),
			audioMime,
			audioFilename,
			strconv.Itoa(len(audio)),
			transcript,
		})
		exported++
	}
	if err := rows.Err(); err != nil {
		fatalf("rows: %v", err)
	}
	w.Flush()
	_ = csvFile.Close()

	fmt.Printf("exported %d calls to %s\n", exported, *outDir)
	fmt.Printf("  csv: %s\n", csvPath)
	fmt.Printf("  audio: %s/\n", audioDir)
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
