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
	"encoding/json"
	"fmt"
	"log"
	"time"
)

// migrateFixKeywordListIds is called during database migrations to repair orphaned keyword list IDs
func migrateFixKeywordListIds(db *Database) error {
	// Check if migration has already been applied
	var migrationExists bool
	checkQuery := `SELECT EXISTS(SELECT 1 FROM "migrations" WHERE "name" = 'fix_keyword_list_ids_v1')`
	if err := db.Sql.QueryRow(checkQuery).Scan(&migrationExists); err != nil {
		// migrations table might not exist in older versions, continue anyway
		migrationExists = false
	}

	if migrationExists {
		// Migration already applied, skip
		return nil
	}

	log.Printf("Running keyword list ID repair migration...")

	// Get all current keyword lists (sorted by order, then createdAt)
	currentListsQuery := `SELECT "keywordListId", "label", "order" FROM "keywordLists" ORDER BY "order" ASC, "createdAt" ASC`
	currentRows, err := db.Sql.Query(currentListsQuery)
	if err != nil {
		return fmt.Errorf("failed to query current keyword lists: %v", err)
	}
	defer currentRows.Close()

	type keywordList struct {
		id    uint64
		label string
		order int
	}
	var currentLists []keywordList
	currentIdsMap := make(map[uint64]bool)

	for currentRows.Next() {
		var list keywordList
		if err := currentRows.Scan(&list.id, &list.label, &list.order); err != nil {
			continue
		}
		currentLists = append(currentLists, list)
		currentIdsMap[list.id] = true
	}

	if len(currentLists) == 0 {
		log.Printf("No keyword lists found, skipping migration")
		// Mark migration as complete even if no lists exist
		_, _ = db.Sql.Exec(`INSERT INTO "migrations" ("name", "appliedAt") VALUES ('fix_keyword_list_ids_v1', $1)`, time.Now().Unix())
		return nil
	}

	// Find all unique orphaned IDs (referenced but don't exist)
	prefsQuery := `SELECT DISTINCT "keywordListIds" FROM "userAlertPreferences" WHERE "keywordListIds" != '[]' AND "keywordListIds" != ''`
	prefsRows, err := db.Sql.Query(prefsQuery)
	if err != nil {
		return fmt.Errorf("failed to query user preferences: %v", err)
	}
	defer prefsRows.Close()

	orphanedIdsMap := make(map[uint64]bool)
	for prefsRows.Next() {
		var keywordListIdsJson string
		if err := prefsRows.Scan(&keywordListIdsJson); err != nil {
			continue
		}

		var ids []uint64
		if err := json.Unmarshal([]byte(keywordListIdsJson), &ids); err != nil {
			continue
		}

		for _, id := range ids {
			// If ID doesn't exist in current lists, it's orphaned
			if !currentIdsMap[id] {
				orphanedIdsMap[id] = true
			}
		}
	}

	// Convert to sorted slice
	var orphanedIds []uint64
	for id := range orphanedIdsMap {
		orphanedIds = append(orphanedIds, id)
	}

	// Sort orphaned IDs numerically (ascending)
	for i := 0; i < len(orphanedIds)-1; i++ {
		for j := i + 1; j < len(orphanedIds); j++ {
			if orphanedIds[i] > orphanedIds[j] {
				orphanedIds[i], orphanedIds[j] = orphanedIds[j], orphanedIds[i]
			}
		}
	}

	if len(orphanedIds) == 0 {
		log.Printf("✓ No orphaned keyword list IDs found")
		// Mark migration as complete
		_, _ = db.Sql.Exec(`INSERT INTO "migrations" ("name", "appliedAt") VALUES ('fix_keyword_list_ids_v1', $1)`, time.Now().Unix())
		return nil
	}

	log.Printf("Found %d orphaned keyword list IDs: %v", len(orphanedIds), orphanedIds)

	// Build mapping - map orphaned IDs to current IDs by position
	idMapping := make(map[uint64]uint64)
	for i, orphanedId := range orphanedIds {
		if i < len(currentLists) {
			idMapping[orphanedId] = currentLists[i].id
			log.Printf("  %d → %d ('%s')", orphanedId, currentLists[i].id, currentLists[i].label)
		} else {
			// More orphaned IDs than current lists - remove extras
			idMapping[orphanedId] = 0
			log.Printf("  %d → REMOVE (no corresponding list)", orphanedId)
		}
	}

	// Update all user preferences with orphaned IDs
	query := `SELECT "userAlertPreferenceId", "userId", "systemId", "talkgroupId", "keywordListIds" FROM "userAlertPreferences" WHERE "keywordListIds" != '[]' AND "keywordListIds" != ''`
	rows, err := db.Sql.Query(query)
	if err != nil {
		return fmt.Errorf("failed to query affected preferences: %v", err)
	}
	defer rows.Close()

	type preference struct {
		id             uint64
		userId         uint64
		systemId       uint64
		talkgroupId    uint64
		keywordListIds []uint64
	}

	var preferencesToUpdate []preference
	for rows.Next() {
		var pref preference
		var keywordListIdsJson string
		if err := rows.Scan(&pref.id, &pref.userId, &pref.systemId, &pref.talkgroupId, &keywordListIdsJson); err != nil {
			continue
		}

		if err := json.Unmarshal([]byte(keywordListIdsJson), &pref.keywordListIds); err != nil {
			continue
		}

		// Check if this preference has any orphaned IDs
		hasOrphanedIds := false
		for _, id := range pref.keywordListIds {
			if _, isOrphaned := idMapping[id]; isOrphaned {
				hasOrphanedIds = true
				break
			}
		}

		if hasOrphanedIds {
			preferencesToUpdate = append(preferencesToUpdate, pref)
		}
	}

	if len(preferencesToUpdate) == 0 {
		log.Printf("No preferences need updating")
		// Mark migration as complete
		_, _ = db.Sql.Exec(`INSERT INTO "migrations" ("name", "appliedAt") VALUES ('fix_keyword_list_ids_v1', $1)`, time.Now().Unix())
		return nil
	}

	log.Printf("Updating %d user preferences...", len(preferencesToUpdate))

	// Update each preference
	updatedCount := 0
	for _, pref := range preferencesToUpdate {
		// Apply mapping
		var newIds []uint64
		seenIds := make(map[uint64]bool) // Prevent duplicates

		for _, oldId := range pref.keywordListIds {
			newId := oldId
			if mappedId, exists := idMapping[oldId]; exists {
				newId = mappedId
			}

			// Only add if not zero (removal) and not duplicate
			if newId > 0 && !seenIds[newId] {
				newIds = append(newIds, newId)
				seenIds[newId] = true
			}
		}

		// Check if anything changed
		if !equalSlices(pref.keywordListIds, newIds) {
			newIdsJson, _ := json.Marshal(newIds)

			updateQuery := fmt.Sprintf(`UPDATE "userAlertPreferences" SET "keywordListIds" = $1 WHERE "userAlertPreferenceId" = %d`, pref.id)
			if _, err := db.Sql.Exec(updateQuery, string(newIdsJson)); err != nil {
				log.Printf("Failed to update preference %d: %v", pref.id, err)
				continue
			}
			updatedCount++
		}
	}

	log.Printf("✓ Updated %d user preferences with corrected keyword list IDs", updatedCount)

	// Mark migration as complete
	_, _ = db.Sql.Exec(`INSERT INTO "migrations" ("name", "appliedAt") VALUES ('fix_keyword_list_ids_v1', $1)`, time.Now().Unix())

	return nil
}

// FixKeywordListIds repairs orphaned keyword list ID references in user alert preferences (for command-line tool)
// Automatically detects orphaned IDs and maps them to current keyword lists by position
func (controller *Controller) FixKeywordListIds(dryRun bool) error {
	mode := "LIVE"
	if dryRun {
		mode = "DRY RUN"
	}

	controller.Logs.LogEvent(LogLevelWarn, "========================================")
	controller.Logs.LogEvent(LogLevelWarn, fmt.Sprintf("Keyword List ID Repair Tool [%s]", mode))
	controller.Logs.LogEvent(LogLevelWarn, "========================================")

	// Step 1: Get all current keyword lists (sorted by order, then createdAt)
	currentListsQuery := `SELECT "keywordListId", "label", "order" FROM "keywordLists" ORDER BY "order" ASC, "createdAt" ASC`
	currentRows, err := controller.Database.Sql.Query(currentListsQuery)
	if err != nil {
		return fmt.Errorf("failed to query current keyword lists: %v", err)
	}
	defer currentRows.Close()

	type keywordList struct {
		id    uint64
		label string
		order int
	}
	var currentLists []keywordList
	currentIdsMap := make(map[uint64]bool)

	for currentRows.Next() {
		var list keywordList
		if err := currentRows.Scan(&list.id, &list.label, &list.order); err != nil {
			continue
		}
		currentLists = append(currentLists, list)
		currentIdsMap[list.id] = true
	}

	if len(currentLists) == 0 {
		controller.Logs.LogEvent(LogLevelWarn, "No keyword lists found in database")
		return nil
	}

	controller.Logs.LogEvent(LogLevelInfo, fmt.Sprintf("Found %d current keyword lists:", len(currentLists)))
	for i, list := range currentLists {
		controller.Logs.LogEvent(LogLevelInfo, fmt.Sprintf("  [%d] ID=%d, Label='%s', Order=%d", i+1, list.id, list.label, list.order))
	}

	// Step 2: Find all unique orphaned IDs (referenced but don't exist)
	prefsQuery := `SELECT DISTINCT "keywordListIds" FROM "userAlertPreferences" WHERE "keywordListIds" != '[]' AND "keywordListIds" != ''`
	prefsRows, err := controller.Database.Sql.Query(prefsQuery)
	if err != nil {
		return fmt.Errorf("failed to query user preferences: %v", err)
	}
	defer prefsRows.Close()

	orphanedIdsMap := make(map[uint64]bool)
	for prefsRows.Next() {
		var keywordListIdsJson string
		if err := prefsRows.Scan(&keywordListIdsJson); err != nil {
			continue
		}

		var ids []uint64
		if err := json.Unmarshal([]byte(keywordListIdsJson), &ids); err != nil {
			continue
		}

		for _, id := range ids {
			// If ID doesn't exist in current lists, it's orphaned
			if !currentIdsMap[id] {
				orphanedIdsMap[id] = true
			}
		}
	}

	// Convert to sorted slice
	var orphanedIds []uint64
	for id := range orphanedIdsMap {
		orphanedIds = append(orphanedIds, id)
	}

	// Sort orphaned IDs numerically (ascending)
	for i := 0; i < len(orphanedIds)-1; i++ {
		for j := i + 1; j < len(orphanedIds); j++ {
			if orphanedIds[i] > orphanedIds[j] {
				orphanedIds[i], orphanedIds[j] = orphanedIds[j], orphanedIds[i]
			}
		}
	}

	if len(orphanedIds) == 0 {
		controller.Logs.LogEvent(LogLevelInfo, "✓ No orphaned keyword list IDs found - all references are valid!")
		return nil
	}

	controller.Logs.LogEvent(LogLevelWarn, fmt.Sprintf("Found %d orphaned keyword list IDs: %v", len(orphanedIds), orphanedIds))

	// Step 3: Build mapping - map orphaned IDs to current IDs by position
	idMapping := make(map[uint64]uint64)
	for i, orphanedId := range orphanedIds {
		if i < len(currentLists) {
			idMapping[orphanedId] = currentLists[i].id
		} else {
			// More orphaned IDs than current lists - remove extras
			idMapping[orphanedId] = 0
		}
	}

	controller.Logs.LogEvent(LogLevelInfo, "ID Mapping:")
	for _, orphanedId := range orphanedIds {
		newId := idMapping[orphanedId]
		if newId == 0 {
			controller.Logs.LogEvent(LogLevelInfo, fmt.Sprintf("  %d → REMOVE (no corresponding list)", orphanedId))
		} else {
			var label string
			for _, list := range currentLists {
				if list.id == newId {
					label = list.label
					break
				}
			}
			controller.Logs.LogEvent(LogLevelInfo, fmt.Sprintf("  %d → %d ('%s')", orphanedId, newId, label))
		}
	}

	// Step 4: Update all user preferences with orphaned IDs
	query := `SELECT "userAlertPreferenceId", "userId", "systemId", "talkgroupId", "keywordListIds" FROM "userAlertPreferences" WHERE "keywordListIds" != '[]' AND "keywordListIds" != ''`
	rows, err := controller.Database.Sql.Query(query)
	if err != nil {
		return fmt.Errorf("failed to query user preferences: %v", err)
	}
	defer rows.Close()

	type preference struct {
		id             uint64
		userId         uint64
		systemId       uint64
		talkgroupId    uint64
		keywordListIds []uint64
	}

	var preferencesToUpdate []preference
	for rows.Next() {
		var pref preference
		var keywordListIdsJson string
		if err := rows.Scan(&pref.id, &pref.userId, &pref.systemId, &pref.talkgroupId, &keywordListIdsJson); err != nil {
			continue
		}

		if err := json.Unmarshal([]byte(keywordListIdsJson), &pref.keywordListIds); err != nil {
			continue
		}

		// Check if this preference has any orphaned IDs
		hasOrphanedIds := false
		for _, id := range pref.keywordListIds {
			if _, isOrphaned := idMapping[id]; isOrphaned {
				hasOrphanedIds = true
				break
			}
		}

		if hasOrphanedIds {
			preferencesToUpdate = append(preferencesToUpdate, pref)
		}
	}

	controller.Logs.LogEvent(LogLevelInfo, fmt.Sprintf("Found %d user preferences with orphaned keyword list IDs", len(preferencesToUpdate)))

	if len(preferencesToUpdate) == 0 {
		controller.Logs.LogEvent(LogLevelInfo, "✓ No preferences need updating")
		return nil
	}

	// Update each preference
	updatedCount := 0
	for _, pref := range preferencesToUpdate {
		oldIds := make([]uint64, len(pref.keywordListIds))
		copy(oldIds, pref.keywordListIds)

		// Apply mapping
		var newIds []uint64
		seenIds := make(map[uint64]bool) // Prevent duplicates

		for _, oldId := range pref.keywordListIds {
			newId := oldId
			if mappedId, exists := idMapping[oldId]; exists {
				newId = mappedId
			}

			// Only add if not zero (removal) and not duplicate
			if newId > 0 && !seenIds[newId] {
				newIds = append(newIds, newId)
				seenIds[newId] = true
			}
		}

		// Check if anything changed
		idsChanged := !equalSlices(oldIds, newIds)

		if idsChanged {
			newIdsJson, _ := json.Marshal(newIds)

			controller.Logs.LogEvent(LogLevelInfo, fmt.Sprintf("User %d (System %d, Talkgroup %d): %v → %v",
				pref.userId, pref.systemId, pref.talkgroupId, oldIds, newIds))

			if !dryRun {
				updateQuery := fmt.Sprintf(`UPDATE "userAlertPreferences" SET "keywordListIds" = $1 WHERE "userAlertPreferenceId" = %d`, pref.id)
				if _, err := controller.Database.Sql.Exec(updateQuery, string(newIdsJson)); err != nil {
					controller.Logs.LogEvent(LogLevelError, fmt.Sprintf("Failed to update preference %d: %v", pref.id, err))
					continue
				}
			}
			updatedCount++
		}
	}

	controller.Logs.LogEvent(LogLevelWarn, "========================================")
	if dryRun {
		controller.Logs.LogEvent(LogLevelWarn, "DRY RUN COMPLETE - No changes made")
		controller.Logs.LogEvent(LogLevelWarn, fmt.Sprintf("Would have updated %d user preferences", updatedCount))
		controller.Logs.LogEvent(LogLevelWarn, "Run without -fix_keyword_ids_dry_run to apply changes")
	} else {
		controller.Logs.LogEvent(LogLevelWarn, "MIGRATION COMPLETE")
		controller.Logs.LogEvent(LogLevelWarn, fmt.Sprintf("Updated %d user preferences", updatedCount))
	}
	controller.Logs.LogEvent(LogLevelWarn, "========================================")

	return nil
}

// equalSlices checks if two uint64 slices are equal
func equalSlices(a, b []uint64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
