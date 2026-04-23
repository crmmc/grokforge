package main

import "github.com/crmmc/grokforge/internal/logging"

const tempAdminPasswordLogMarker = "TEMP_ADMIN_PASSWORD"

func logTemporaryBootstrapAdminPassword(password string) {
	logging.Warn("admin app_key not configured; generated temporary bootstrap admin password for this process only",
		"search_marker", tempAdminPasswordLogMarker,
		"persistence", "runtime_only",
		"expires", "process_exit")
	logging.Warn(tempAdminPasswordLogMarker, "temp_admin_password", password)
}
