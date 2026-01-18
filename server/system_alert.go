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
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// SystemAlert represents a system-level alert for administrators
type SystemAlert struct {
	Id        uint64 `json:"id"`
	AlertType string `json:"alertType"` // "transcription_failure", "tone_detection_issue", "service_health", "manual"
	Severity  string `json:"severity"`  // "info", "warning", "error", "critical"
	Title     string `json:"title"`
	Message   string `json:"message"`
	Data      string `json:"data"` // JSON data for additional context
	CreatedAt int64  `json:"createdAt"`
	CreatedBy uint64 `json:"createdBy"` // User ID who created it (0 for system-generated)
	Dismissed bool   `json:"dismissed"`
}

// SystemAlertData represents the parsed Data field
type SystemAlertData struct {
	CallId      uint64 `json:"callId,omitempty"`
	SystemId    uint64 `json:"systemId,omitempty"`
	TalkgroupId uint64 `json:"talkgroupId,omitempty"`
	Error       string `json:"error,omitempty"`
	Count       int    `json:"count,omitempty"`
	Service     string `json:"service,omitempty"`
}

// CreateSystemAlert creates a new system alert
func (controller *Controller) CreateSystemAlert(alertType, severity, title, message string, data *SystemAlertData, createdBy uint64) error {
	var dataJSON string
	if data != nil {
		b, err := json.Marshal(data)
		if err != nil {
			dataJSON = "{}"
		} else {
			dataJSON = string(b)
		}
	} else {
		dataJSON = "{}"
	}

	createdAt := time.Now().UnixMilli()

	var query string
	if createdBy > 0 {
		query = fmt.Sprintf(`INSERT INTO "systemAlerts" ("alertType", "severity", "title", "message", "data", "createdAt", "createdBy") VALUES ('%s', '%s', '%s', '%s', '%s', %d, %d)`,
			escapeQuotes(alertType), escapeQuotes(severity), escapeQuotes(title), escapeQuotes(message), escapeQuotes(dataJSON), createdAt, createdBy)
	} else {
		query = fmt.Sprintf(`INSERT INTO "systemAlerts" ("alertType", "severity", "title", "message", "data", "createdAt") VALUES ('%s', '%s', '%s', '%s', '%s', %d)`,
			escapeQuotes(alertType), escapeQuotes(severity), escapeQuotes(title), escapeQuotes(message), escapeQuotes(dataJSON), createdAt)
	}

	if _, err := controller.Database.Sql.Exec(query); err != nil {
		return fmt.Errorf("failed to create system alert: %v", err)
	}

	controller.Logs.LogEvent(LogLevelInfo, fmt.Sprintf("System alert created: [%s] %s - %s", severity, title, message))

	// Send push notification to all system admins
	go controller.SendSystemAlertNotification(title, message, alertType, severity, dataJSON)

	return nil
}

// SendSystemAlertNotification sends a push notification for system alerts
// Manual alerts (sent by admins) go to all verified users
// Health monitoring alerts only go to system admins
func (controller *Controller) SendSystemAlertNotification(title, message, alertType, severity, dataJSON string) {
	var query string
	var targetDescription string

	if alertType == "manual" {
		// Manual alerts: send to ALL verified users
		query = `SELECT "userId" FROM "users" WHERE "verified" = true`
		targetDescription = "verified users"
	} else {
		// Health/monitoring alerts: only send to system admins
		query = `SELECT "userId" FROM "users" WHERE "systemAdmin" = true`
		targetDescription = "system admins"
	}

	rows, err := controller.Database.Sql.Query(query)
	if err != nil {
		controller.Logs.LogEvent(LogLevelError, fmt.Sprintf("failed to get %s: %v", targetDescription, err))
		return
	}
	defer rows.Close()

	var targetUserIds []uint64
	for rows.Next() {
		var userId uint64
		if err := rows.Scan(&userId); err != nil {
			continue
		}
		targetUserIds = append(targetUserIds, userId)
	}

	if len(targetUserIds) == 0 {
		controller.Logs.LogEvent(LogLevelWarn, fmt.Sprintf("no %s found to send system alert notification", targetDescription))
		return
	}

	// Get device tokens for target users
	var playerIds []string
	for _, userId := range targetUserIds {
		tokens := controller.DeviceTokens.GetByUser(userId)
		for _, token := range tokens {
			if token.Token != "" {
				playerIds = append(playerIds, token.Token)
			}
		}
	}

	if len(playerIds) == 0 {
		controller.Logs.LogEvent(LogLevelWarn, fmt.Sprintf("no device tokens found for %s", targetDescription))
		return
	}

	// Determine icon and sound based on severity
	icon := "🔔"
	sound := "startup.wav"
	switch severity {
	case "critical":
		icon = "🚨"
		sound = "startup.wav" // Could be customized per severity
	case "error":
		icon = "❌"
		sound = "startup.wav"
	case "warning":
		icon = "⚠️"
		sound = "startup.wav"
	case "info":
		icon = "ℹ️"
		sound = "startup.wav"
	}

	// Group player IDs by platform (if we had platform info, but we don't store it in device tokens lookup)
	// For now, send to all devices using the batch system
	notificationTitle := fmt.Sprintf("%s System Alert", icon)

	// Send to all player IDs at once
	// The relay server will handle the actual platform-specific formatting
	if len(playerIds) > 0 {
		go controller.sendNotificationBatch(playerIds, notificationTitle, title, message, "android", sound, nil, "", "")
		controller.Logs.LogEvent(LogLevelInfo, fmt.Sprintf("[%s] system alert notification sent to %d device(s) (%s)", alertType, len(playerIds), targetDescription))
	}
}

// GetSystemAlerts retrieves system alerts (optionally filtered by dismissed status)
func (controller *Controller) GetSystemAlerts(limit int, includeDismissed bool) ([]*SystemAlert, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}

	var query string
	if includeDismissed {
		query = fmt.Sprintf(`SELECT "alertId", "alertType", "severity", "title", "message", "data", "createdAt", COALESCE("createdBy", 0), "dismissed" FROM "systemAlerts" ORDER BY "createdAt" DESC LIMIT %d`, limit)
	} else {
		query = fmt.Sprintf(`SELECT "alertId", "alertType", "severity", "title", "message", "data", "createdAt", COALESCE("createdBy", 0), "dismissed" FROM "systemAlerts" WHERE "dismissed" = false ORDER BY "createdAt" DESC LIMIT %d`, limit)
	}

	rows, err := controller.Database.Sql.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query system alerts: %v", err)
	}
	defer rows.Close()

	var alerts []*SystemAlert
	for rows.Next() {
		alert := &SystemAlert{}
		if err := rows.Scan(&alert.Id, &alert.AlertType, &alert.Severity, &alert.Title, &alert.Message, &alert.Data, &alert.CreatedAt, &alert.CreatedBy, &alert.Dismissed); err != nil {
			continue
		}
		alerts = append(alerts, alert)
	}

	return alerts, nil
}

// DismissSystemAlert marks a system alert as dismissed
func (controller *Controller) DismissSystemAlert(alertId uint64) error {
	query := fmt.Sprintf(`UPDATE "systemAlerts" SET "dismissed" = true WHERE "alertId" = %d`, alertId)
	if _, err := controller.Database.Sql.Exec(query); err != nil {
		return fmt.Errorf("failed to dismiss system alert: %v", err)
	}
	return nil
}

// CleanupOldSystemAlerts removes system alerts older than retention days
func (controller *Controller) CleanupOldSystemAlerts() {
	retentionDays := controller.Options.AlertRetentionDays
	if retentionDays == 0 {
		retentionDays = 5 // Default: 5 days
	}

	cutoffTime := time.Now().Add(-time.Duration(retentionDays) * 24 * time.Hour).UnixMilli()
	var query string
	if controller.Database.Config.DbType == DbTypePostgresql {
		query = `DELETE FROM "systemAlerts" WHERE "createdAt" < $1`
	} else {
		query = `DELETE FROM "systemAlerts" WHERE "createdAt" < ?`
	}

	result, err := controller.Database.Sql.Exec(query, cutoffTime)
	if err != nil {
		controller.Logs.LogEvent(LogLevelWarn, fmt.Sprintf("failed to cleanup old system alerts: %v", err))
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected > 0 {
		controller.Logs.LogEvent(LogLevelInfo, fmt.Sprintf("cleaned up %d old system alerts (older than %d days)", rowsAffected, retentionDays))
	}
}

// getProviderDisplayName converts a provider string to a user-friendly display name
func getProviderDisplayName(provider string) string {
	switch provider {
	case "whisper-api":
		return "Whisper API Server"
	case "azure":
		return "Azure Speech Services"
	case "google":
		return "Google Cloud Speech-to-Text"
	case "assemblyai":
		return "AssemblyAI"
	default:
		// Default fallback if provider is unknown or empty
		if provider == "" {
			return "transcription service"
		}
		return provider
	}
}

// MonitorTranscriptionFailures monitors for transcription failures and creates system alerts
func (controller *Controller) MonitorTranscriptionFailures() {
	// Check if transcription failure alerts are enabled
	if !controller.Options.TranscriptionFailureAlertsEnabled || !controller.Options.SystemHealthAlertsEnabled {
		return
	}

	// Get configurable time window (default 24 hours)
	timeWindowHours := int(controller.Options.TranscriptionFailureTimeWindow)
	if timeWindowHours <= 0 {
		timeWindowHours = 24
	}
	timeWindowAgo := time.Now().Add(-time.Duration(timeWindowHours) * time.Hour).UnixMilli()

	query := fmt.Sprintf(`SELECT COUNT(*) FROM "calls" WHERE "transcriptionStatus" = 'failed' AND "timestamp" >= %d`, timeWindowAgo)

	var failureCount int
	if err := controller.Database.Sql.QueryRow(query).Scan(&failureCount); err != nil {
		controller.Logs.LogEvent(LogLevelError, fmt.Sprintf("failed to check transcription failures: %v", err))
		return
	}

	// Use configurable threshold, default to 10
	threshold := int(controller.Options.TranscriptionFailureThreshold)
	if threshold <= 0 {
		threshold = 10
	}

	// If we have more than threshold failures in last 24 hours, create an alert
	if failureCount >= threshold && controller.Options.SystemHealthAlertsEnabled {
		// Check if there's already an active alert for transcription failures
		// Only create a new alert if the last one is older than the repeat interval
		repeatMinutes := int(controller.Options.TranscriptionFailureRepeatMinutes)
		if repeatMinutes <= 0 {
			repeatMinutes = 60 // Default: 60 minutes
		}

		checkAlertQuery := `SELECT MAX("createdAt") FROM "systemAlerts" 
			WHERE "alertType" = 'transcription_failure' 
				AND "dismissedAt" IS NULL`

		var lastAlertTime sql.NullInt64
		shouldCreateAlert := true
		if err := controller.Database.Sql.QueryRow(checkAlertQuery).Scan(&lastAlertTime); err == nil && lastAlertTime.Valid {
			lastAlertTimeObj := time.UnixMilli(lastAlertTime.Int64)
			minutesSinceLastAlert := int(time.Since(lastAlertTimeObj).Minutes())
			// Only create new alert if last one is older than repeat interval
			if minutesSinceLastAlert < repeatMinutes {
				shouldCreateAlert = false
			}
		}

		if shouldCreateAlert {
			// Get provider name for the alert message
			providerName := getProviderDisplayName(controller.Options.TranscriptionConfig.Provider)

			data := &SystemAlertData{
				Count:   failureCount,
				Service: "transcription",
			}

			timeWindowStr := fmt.Sprintf("%d hour(s)", timeWindowHours)
			if timeWindowHours == 24 {
				timeWindowStr = "24 hours"
			}
			controller.CreateSystemAlert(
				"transcription_failure",
				"warning",
				"Transcription Service Issues",
				fmt.Sprintf("%d transcription failures detected in the last %s. Check %s service status.", failureCount, timeWindowStr, providerName),
				data,
				0, // System-generated
			)
		}
	}
}

// MonitorToneDetectionIssues monitors for tone detection problems
func (controller *Controller) MonitorToneDetectionIssues() {
	// Check if tone detection alerts are enabled
	if !controller.Options.ToneDetectionAlertsEnabled || !controller.Options.SystemHealthAlertsEnabled {
		return
	}

	// Get configurable time window (default 24 hours)
	timeWindowHours := int(controller.Options.ToneDetectionTimeWindow)
	if timeWindowHours <= 0 {
		timeWindowHours = 24
	}
	timeWindowAgo := time.Now().Add(-time.Duration(timeWindowHours) * time.Hour).UnixMilli()

	// Get talkgroups with tone detection enabled
	query := `SELECT "talkgroupId", "label", "systemId" FROM "talkgroups" WHERE "toneDetectionEnabled" = true`
	rows, err := controller.Database.Sql.Query(query)
	if err != nil {
		controller.Logs.LogEvent(LogLevelError, fmt.Sprintf("failed to check tone detection: %v", err))
		return
	}
	defer rows.Close()

	for rows.Next() {
		var talkgroupId, systemId uint64
		var label string
		if err := rows.Scan(&talkgroupId, &label, &systemId); err != nil {
			continue
		}

		// Check if this talkgroup has had any calls with tones in the time window
		checkQuery := fmt.Sprintf(`SELECT COUNT(*) FROM "calls" WHERE "talkgroupId" = %d AND "hasTones" = true AND "timestamp" >= %d`, talkgroupId, timeWindowAgo)

		var toneCount int
		if err := controller.Database.Sql.QueryRow(checkQuery).Scan(&toneCount); err != nil {
			continue
		}

		// Also check if there have been ANY calls on this talkgroup
		callCountQuery := fmt.Sprintf(`SELECT COUNT(*) FROM "calls" WHERE "talkgroupId" = %d AND "timestamp" >= %d`, talkgroupId, timeWindowAgo)

		var callCount int
		if err := controller.Database.Sql.QueryRow(callCountQuery).Scan(&callCount); err != nil {
			continue
		}

		// Only alert if there have been calls but no tones (might indicate tone detection issue)
		threshold := int(controller.Options.ToneDetectionIssueThreshold)
		if threshold <= 0 {
			threshold = 5 // Default: 5 calls
		}
		if callCount >= threshold && toneCount == 0 {
			// Check if there's already an active alert for this talkgroup
			// Only create a new alert if the last one is older than the repeat interval
			repeatMinutes := int(controller.Options.ToneDetectionRepeatMinutes)
			if repeatMinutes <= 0 {
				repeatMinutes = 60 // Default: 60 minutes
			}

			checkAlertQuery := fmt.Sprintf(`
				SELECT MAX("createdAt") FROM "systemAlerts" 
				WHERE "alertType" = 'tone_detection_issue' 
					AND "data" LIKE '%%"talkgroupId":%d%%'
					AND "dismissedAt" IS NULL
			`, talkgroupId)

			var lastAlertTime sql.NullInt64
			shouldCreateAlert := true
			if err := controller.Database.Sql.QueryRow(checkAlertQuery).Scan(&lastAlertTime); err == nil && lastAlertTime.Valid {
				lastAlertTimeObj := time.UnixMilli(lastAlertTime.Int64)
				minutesSinceLastAlert := int(time.Since(lastAlertTimeObj).Minutes())
				// Only create new alert if last one is older than repeat interval
				if minutesSinceLastAlert < repeatMinutes {
					shouldCreateAlert = false
				}
			}

			if shouldCreateAlert {
				data := &SystemAlertData{
					TalkgroupId: talkgroupId,
					SystemId:    systemId,
					Count:       callCount,
				}

				timeWindowStr := fmt.Sprintf("%d hour(s)", timeWindowHours)
				if timeWindowHours == 24 {
					timeWindowStr = "24 hours"
				}
				controller.CreateSystemAlert(
					"tone_detection_issue",
					"info",
					"No Tones Detected",
					fmt.Sprintf("Talkgroup '%s' has tone detection enabled but no tones detected in %d calls over %s.", label, callCount, timeWindowStr),
					data,
					0, // System-generated
				)
			}
		}
	}
}

// MonitorNoAudio monitors systems for lack of audio activity
func (controller *Controller) MonitorNoAudio() {
	// Check if no-audio alerts are enabled
	if !controller.Options.NoAudioAlertsEnabled || !controller.Options.SystemHealthAlertsEnabled {
		return
	}

	// Get configurable time window (default 24 hours)
	// Note: This is used for future filtering if needed, but currently we check all systems
	timeWindowHours := int(controller.Options.NoAudioTimeWindow)
	if timeWindowHours <= 0 {
		timeWindowHours = 24
	}

	// Get base threshold
	baseThresholdMinutes := int(controller.Options.NoAudioThresholdMinutes)
	if baseThresholdMinutes <= 0 {
		baseThresholdMinutes = 30 // Default: 30 minutes
	}

	// Get multiplier for adaptive threshold
	multiplier := controller.Options.NoAudioMultiplier
	if multiplier <= 0 {
		multiplier = 1.5 // Default: 1.5x
	}

	// Get historical data days
	historicalDays := int(controller.Options.NoAudioHistoricalDataDays)
	if historicalDays <= 0 {
		historicalDays = 7 // Default: 7 days
	}

	// Get all systems (systems don't have an enabled field, so we check all)
	query := `SELECT "systemId", "label" FROM "systems"`
	rows, err := controller.Database.Sql.Query(query)
	if err != nil {
		controller.Logs.LogEvent(LogLevelError, fmt.Sprintf("failed to query systems for no-audio monitoring: %v", err))
		return
	}
	defer rows.Close()

	currentTime := time.Now()
	currentHour := currentTime.Hour()

	for rows.Next() {
		var systemId uint64
		var systemLabel string
		if err := rows.Scan(&systemId, &systemLabel); err != nil {
			continue
		}

		// Get the most recent call timestamp for this system
		lastCallQuery := fmt.Sprintf(`SELECT MAX("timestamp") FROM "calls" WHERE "systemId" = %d`, systemId)
		var lastCallTimestamp sql.NullInt64
		if err := controller.Database.Sql.QueryRow(lastCallQuery).Scan(&lastCallTimestamp); err != nil {
			// No calls found for this system - skip it
			continue
		}

		if !lastCallTimestamp.Valid {
			// No calls ever received for this system
			continue
		}

		lastCallTime := time.UnixMilli(lastCallTimestamp.Int64)
		minutesSinceLastCall := int(currentTime.Sub(lastCallTime).Minutes())

		// Calculate adaptive threshold based on historical data
		thresholdMinutes := baseThresholdMinutes

		// Only use adaptive threshold if we have enough historical data
		if historicalDays > 0 {
			// Look at the same hour of day over the last N days
			historicalStartTime := currentTime.Add(-time.Duration(historicalDays) * 24 * time.Hour)
			historicalStartTimestamp := historicalStartTime.UnixMilli()

			// Query to get average time between calls for this hour of day (PostgreSQL only)
			// We'll calculate gaps between consecutive calls in the same hour window
			avgGapQuery := fmt.Sprintf(`
				WITH call_times AS (
					SELECT "timestamp", 
						LAG("timestamp") OVER (ORDER BY "timestamp") as prev_timestamp
					FROM "calls"
					WHERE "systemId" = %d 
						AND "timestamp" >= %d
						AND EXTRACT(HOUR FROM to_timestamp("timestamp" / 1000.0)) = %d
				)
				SELECT AVG("timestamp" - prev_timestamp) / 60000.0 as avg_gap_minutes
				FROM call_times
				WHERE prev_timestamp IS NOT NULL
			`, systemId, historicalStartTimestamp, currentHour)

			var avgGapMinutes sql.NullFloat64
			if err := controller.Database.Sql.QueryRow(avgGapQuery).Scan(&avgGapMinutes); err == nil && avgGapMinutes.Valid && avgGapMinutes.Float64 > 0 {
				// Use adaptive threshold: max(base, historical_average × multiplier)
				adaptiveThreshold := int(avgGapMinutes.Float64 * multiplier)
				if adaptiveThreshold > thresholdMinutes {
					thresholdMinutes = adaptiveThreshold
				}
			}
		}

		// Check if we should alert
		if minutesSinceLastCall >= thresholdMinutes {
			// Check if there's already an active alert for this system
			// Only create a new alert if the last one is older than the repeat interval
			repeatMinutes := int(controller.Options.NoAudioRepeatMinutes)
			if repeatMinutes <= 0 {
				repeatMinutes = 30 // Default: 30 minutes
			}

			checkAlertQuery := fmt.Sprintf(`
				SELECT MAX("createdAt") FROM "systemAlerts" 
				WHERE "alertType" = 'no_audio_received' 
					AND "data" LIKE '%%"systemId":%d%%'
					AND "dismissedAt" IS NULL
			`, systemId)

			var lastAlertTime sql.NullInt64
			shouldCreateAlert := true
			if err := controller.Database.Sql.QueryRow(checkAlertQuery).Scan(&lastAlertTime); err == nil && lastAlertTime.Valid {
				lastAlertTimeObj := time.UnixMilli(lastAlertTime.Int64)
				minutesSinceLastAlert := int(currentTime.Sub(lastAlertTimeObj).Minutes())
				// Only create new alert if last one is older than repeat interval
				if minutesSinceLastAlert < repeatMinutes {
					shouldCreateAlert = false
				}
			}

			if shouldCreateAlert {
				data := &SystemAlertData{
					SystemId: systemId,
					Count:    minutesSinceLastCall,
				}

				thresholdStr := fmt.Sprintf("%d minutes", thresholdMinutes)
				if thresholdMinutes >= 60 {
					hours := thresholdMinutes / 60
					mins := thresholdMinutes % 60
					if mins == 0 {
						thresholdStr = fmt.Sprintf("%d hour(s)", hours)
					} else {
						thresholdStr = fmt.Sprintf("%d hour(s) %d minute(s)", hours, mins)
					}
				}

				timeSinceLastCall := fmt.Sprintf("%d minutes", minutesSinceLastCall)
				if minutesSinceLastCall >= 60 {
					hours := minutesSinceLastCall / 60
					mins := minutesSinceLastCall % 60
					if mins == 0 {
						timeSinceLastCall = fmt.Sprintf("%d hour(s)", hours)
					} else {
						timeSinceLastCall = fmt.Sprintf("%d hour(s) %d minute(s)", hours, mins)
					}
				}

				controller.CreateSystemAlert(
					"no_audio_received",
					"warning",
					"No Audio Received",
					fmt.Sprintf("System '%s' has not received audio for %s (threshold: %s).", systemLabel, timeSinceLastCall, thresholdStr),
					data,
					0, // System-generated
				)
			}
		}
	}
}

// StartSystemHealthMonitoring starts periodic system health checks
func (controller *Controller) StartSystemHealthMonitoring() {
	ticker := time.NewTicker(1 * time.Hour) // Check every hour
	go func() {
		for range ticker.C {
			controller.MonitorTranscriptionFailures()
			controller.MonitorToneDetectionIssues()
			controller.MonitorNoAudio()
		}
	}()

	controller.Logs.LogEvent(LogLevelInfo, "system health monitoring started")
}
