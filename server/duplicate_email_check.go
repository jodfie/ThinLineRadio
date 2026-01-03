package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// checkDuplicateEmails checks for duplicate email addresses (case-insensitive) at startup
// and writes them to a log file if any are found
func (controller *Controller) checkDuplicateEmails() {
	duplicates := controller.Users.CheckDuplicateEmails()
	
	if len(duplicates) == 0 {
		// No duplicates found
		return
	}
	
	// Create log file in server directory
	logFileName := "duplicate_emails.log"
	logFilePath := filepath.Join(controller.Config.BaseDir, logFileName)
	
	// Open file for writing
	f, err := os.Create(logFilePath)
	if err != nil {
		log.Printf("WARNING: Failed to create duplicate emails log file: %v", err)
		return
	}
	defer f.Close()
	
	// Write header
	header := fmt.Sprintf("=== DUPLICATE EMAIL ADDRESSES FOUND ===\n")
	header += fmt.Sprintf("Generated: %s\n", time.Now().Format("2006-01-02 15:04:05"))
	header += fmt.Sprintf("Total duplicate email groups: %d\n\n", len(duplicates))
	header += "IMPORTANT: These users have the same email address with different capitalization.\n"
	header += "Only one of these users will be able to log in. Please manually resolve these duplicates\n"
	header += "by deleting the unwanted accounts or updating their email addresses.\n\n"
	header += strings.Repeat("=", 80) + "\n\n"
	
	f.WriteString(header)
	
	// Sort emails for consistent output
	var sortedEmails []string
	for email := range duplicates {
		sortedEmails = append(sortedEmails, email)
	}
	sort.Strings(sortedEmails)
	
	// Write each duplicate group
	totalAffectedUsers := 0
	for i, normalizedEmail := range sortedEmails {
		userList := duplicates[normalizedEmail]
		totalAffectedUsers += len(userList)
		
		f.WriteString(fmt.Sprintf("Duplicate Group #%d: %s (%d users)\n", i+1, normalizedEmail, len(userList)))
		f.WriteString(strings.Repeat("-", 80) + "\n")
		
		for j, user := range userList {
			f.WriteString(fmt.Sprintf("  [%d] User ID: %d\n", j+1, user.Id))
			f.WriteString(fmt.Sprintf("      Email (stored): %s\n", user.Email))
			f.WriteString(fmt.Sprintf("      Name: %s %s\n", user.FirstName, user.LastName))
			f.WriteString(fmt.Sprintf("      Verified: %v\n", user.Verified))
			f.WriteString(fmt.Sprintf("      User Group ID: %d\n", user.UserGroupId))
			if user.CreatedAt != "" {
				f.WriteString(fmt.Sprintf("      Created: %s\n", user.CreatedAt))
			}
			if user.LastLogin != "" {
				f.WriteString(fmt.Sprintf("      Last Login: %s\n", user.LastLogin))
			}
			f.WriteString("\n")
		}
		f.WriteString("\n")
	}
	
	// Write summary footer
	footer := strings.Repeat("=", 80) + "\n"
	footer += fmt.Sprintf("SUMMARY: %d duplicate email groups found, %d users affected\n", len(duplicates), totalAffectedUsers)
	footer += "\nRECOMMENDED ACTIONS:\n"
	footer += "1. Review each duplicate group\n"
	footer += "2. Determine which account should be kept\n"
	footer += "3. Delete or update the other accounts via the admin panel\n"
	footer += "4. Restart the server to verify duplicates are resolved\n"
	footer += strings.Repeat("=", 80) + "\n"
	
	f.WriteString(footer)
	
	// Log to console
	log.Printf("WARNING: Found %d duplicate email groups (%d users affected)", len(duplicates), totalAffectedUsers)
	log.Printf("WARNING: Duplicate email details written to: %s", logFilePath)
	log.Printf("WARNING: Only one user per duplicate group will be able to log in")
	log.Printf("WARNING: Please review and resolve these duplicates manually")
}

