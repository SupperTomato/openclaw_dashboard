package config

// FieldDescription provides user-friendly info about each config field
type FieldDescription struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Description string `json:"description"`
	Type        string `json:"type"` // string, int, bool, float, array
	Default     any    `json:"default"`
	Category    string `json:"category"`
	Warning     string `json:"warning,omitempty"`
}

// GetDescriptions returns all config field descriptions for the UI
func GetDescriptions() []FieldDescription {
	return []FieldDescription{
		// Dashboard
		{Key: "dashboard.host", Label: "Listen Address", Description: "The network address the dashboard listens on. Use '0.0.0.0' to allow access from any device on your network, or '127.0.0.1' to only allow access from this computer.", Type: "string", Default: "0.0.0.0", Category: "Dashboard"},
		{Key: "dashboard.port", Label: "Port Number", Description: "The port number the dashboard runs on. You'll access the dashboard at http://your-ip:PORT. Default is 8080. Change this if another program is already using port 8080.", Type: "int", Default: 8080, Category: "Dashboard"},
		{Key: "dashboard.title", Label: "Dashboard Title", Description: "The name shown at the top of the dashboard. Change this to whatever you'd like!", Type: "string", Default: "OpenClaw Dashboard", Category: "Dashboard"},
		{Key: "dashboard.refresh_interval", Label: "Refresh Interval (seconds)", Description: "How often the dashboard automatically updates its data, in seconds. Lower values = more frequent updates but slightly more CPU usage. 5 seconds is a good balance.", Type: "int", Default: 5, Category: "Dashboard"},
		{Key: "dashboard.theme", Label: "Color Theme", Description: "Choose between 'dark' (easier on the eyes) or 'light' (brighter) color scheme.", Type: "string", Default: "dark", Category: "Dashboard"},
		{Key: "dashboard.language", Label: "Language", Description: "Display language for the dashboard. Currently supports 'en' (English).", Type: "string", Default: "en", Category: "Dashboard"},

		// System Health Module
		{Key: "modules.system_health.enabled", Label: "Enable System Health", Description: "Turn this ON to see your computer's CPU, memory, disk space, and temperature. Turn OFF to save a tiny bit of resources if you don't need it.", Type: "bool", Default: true, Category: "System Health"},
		{Key: "modules.system_health.port", Label: "Module Port", Description: "Internal port used by the System Health module. Only change if there's a conflict with another program.", Type: "int", Default: 9001, Category: "System Health"},
		{Key: "modules.system_health.refresh_interval", Label: "Update Interval (seconds)", Description: "How often to check system stats. Lower = more responsive but uses slightly more CPU. 3 seconds works well.", Type: "int", Default: 3, Category: "System Health"},
		{Key: "modules.system_health.history_hours", Label: "History Duration (hours)", Description: "How many hours of CPU/RAM history to keep for the sparkline charts. 24 hours shows a full day's pattern.", Type: "int", Default: 24, Category: "System Health"},
		{Key: "modules.system_health.temp_warning_celsius", Label: "Temperature Warning (C)", Description: "CPU temperature (in Celsius) that triggers a yellow warning. 70C is a safe default for most systems.", Type: "int", Default: 70, Category: "System Health"},
		{Key: "modules.system_health.temp_critical_celsius", Label: "Temperature Critical (C)", Description: "CPU temperature (in Celsius) that triggers a red critical alert. 85C means your system is running very hot.", Type: "int", Default: 85, Category: "System Health"},

		// Session Manager Module
		{Key: "modules.session_manager.enabled", Label: "Enable Session Manager", Description: "Turn this ON to view and manage all your AI agent sessions. Shows active sessions, their status, and activity.", Type: "bool", Default: true, Category: "Session Manager"},
		{Key: "modules.session_manager.port", Label: "Module Port", Description: "Internal port for the Session Manager module.", Type: "int", Default: 9002, Category: "Session Manager"},
		{Key: "modules.session_manager.sessions_dir", Label: "Sessions Directory", Description: "Where your Claude/agent session files are stored. Use ~ for your home directory.", Type: "string", Default: "~/.claude/projects", Category: "Session Manager"},
		{Key: "modules.session_manager.max_sessions_display", Label: "Max Sessions to Show", Description: "Maximum number of sessions to display in the list. Increase if you have many sessions, decrease to improve loading speed.", Type: "int", Default: 100, Category: "Session Manager"},

		// Live Feed Module
		{Key: "modules.live_feed.enabled", Label: "Enable Live Feed", Description: "Turn this ON to see a real-time stream of all agent messages as they happen.", Type: "bool", Default: true, Category: "Live Feed"},
		{Key: "modules.live_feed.port", Label: "Module Port", Description: "Internal port for the Live Feed module.", Type: "int", Default: 9003, Category: "Live Feed"},
		{Key: "modules.live_feed.max_messages", Label: "Max Messages in Feed", Description: "How many messages to keep in the feed before old ones are removed. Higher values use more memory.", Type: "int", Default: 500, Category: "Live Feed"},
		{Key: "modules.live_feed.auto_scroll", Label: "Auto-Scroll", Description: "When ON, the feed automatically scrolls to show the newest messages. Turn OFF if you prefer to scroll manually.", Type: "bool", Default: true, Category: "Live Feed"},

		// Log Viewer Module
		{Key: "modules.log_viewer.enabled", Label: "Enable Log Viewer", Description: "Turn this ON to view system and application logs in real-time from the dashboard.", Type: "bool", Default: true, Category: "Log Viewer"},
		{Key: "modules.log_viewer.port", Label: "Module Port", Description: "Internal port for the Log Viewer module.", Type: "int", Default: 9004, Category: "Log Viewer"},
		{Key: "modules.log_viewer.log_paths", Label: "Log File Paths", Description: "List of log files to watch. Add paths to any log files you want to monitor. Common ones include /var/log/syslog and application-specific logs.", Type: "array", Default: []string{"/var/log/syslog", "/var/log/openclaw.log"}, Category: "Log Viewer"},
		{Key: "modules.log_viewer.max_lines", Label: "Max Lines to Display", Description: "Maximum number of log lines to show. Higher values let you see more history but use more memory.", Type: "int", Default: 1000, Category: "Log Viewer"},
		{Key: "modules.log_viewer.follow", Label: "Follow Mode", Description: "When ON, the log viewer automatically shows new lines as they appear (like 'tail -f').", Type: "bool", Default: true, Category: "Log Viewer"},

		// File Manager Module
		{Key: "modules.file_manager.enabled", Label: "Enable File Manager", Description: "Turn this ON to browse, view, and edit files on your server from the dashboard.", Type: "bool", Default: true, Category: "File Manager"},
		{Key: "modules.file_manager.port", Label: "Module Port", Description: "Internal port for the File Manager module.", Type: "int", Default: 9005, Category: "File Manager"},
		{Key: "modules.file_manager.root_dir", Label: "Root Directory", Description: "The starting directory for the file browser. Use ~ for your home directory. The file manager cannot access files outside this directory for security.", Type: "string", Default: "~", Category: "File Manager", Warning: "Changing this to / gives access to ALL files on the system. Use with caution!"},
		{Key: "modules.file_manager.allowed_extensions", Label: "Allowed File Types", Description: "Only files with these extensions can be viewed/edited. This prevents accidentally opening binary files or system files.", Type: "array", Default: []string{".md", ".txt", ".json", ".yaml", ".yml", ".toml", ".cfg", ".conf", ".log", ".sh", ".py", ".js", ".go"}, Category: "File Manager"},
		{Key: "modules.file_manager.max_file_size_kb", Label: "Max File Size (KB)", Description: "Maximum file size (in kilobytes) that can be opened in the editor. Larger files may cause the browser to slow down.", Type: "int", Default: 1024, Category: "File Manager"},
		{Key: "modules.file_manager.show_hidden", Label: "Show Hidden Files", Description: "When ON, files and folders starting with a dot (.) are shown. These are usually configuration files.", Type: "bool", Default: false, Category: "File Manager"},

		// Cost Analyzer Module
		{Key: "modules.cost_analyzer.enabled", Label: "Enable Cost Analyzer", Description: "Turn this ON to track and analyze your AI API spending over time.", Type: "bool", Default: true, Category: "Cost Analyzer"},
		{Key: "modules.cost_analyzer.port", Label: "Module Port", Description: "Internal port for the Cost Analyzer module.", Type: "int", Default: 9006, Category: "Cost Analyzer"},
		{Key: "modules.cost_analyzer.currency", Label: "Currency", Description: "Currency code for displaying costs. Use 'USD' for US dollars, 'EUR' for euros, etc.", Type: "string", Default: "USD", Category: "Cost Analyzer"},
		{Key: "modules.cost_analyzer.budget_warning", Label: "Budget Warning ($)", Description: "You'll get a warning notification when your spending reaches this amount.", Type: "float", Default: 50.0, Category: "Cost Analyzer"},
		{Key: "modules.cost_analyzer.budget_critical", Label: "Budget Critical ($)", Description: "You'll get a critical alert when your spending reaches this amount. Consider setting this to your monthly budget.", Type: "float", Default: 100.0, Category: "Cost Analyzer"},

		// Rate Limiter Module
		{Key: "modules.rate_limiter.enabled", Label: "Enable Rate Limiter", Description: "Turn this ON to monitor your Claude API rate limits and see how much capacity you have left.", Type: "bool", Default: true, Category: "Rate Limiter"},
		{Key: "modules.rate_limiter.port", Label: "Module Port", Description: "Internal port for the Rate Limiter module.", Type: "int", Default: 9007, Category: "Rate Limiter"},
		{Key: "modules.rate_limiter.window_hours", Label: "Rate Window (hours)", Description: "The rolling time window for rate limit tracking. Claude uses 5-hour windows by default.", Type: "int", Default: 5, Category: "Rate Limiter"},
		{Key: "modules.rate_limiter.warning_percent", Label: "Warning at (%)", Description: "Show a warning when you've used this percentage of your rate limit. 80% gives you a heads-up before hitting the limit.", Type: "int", Default: 80, Category: "Rate Limiter"},

		// Memory Viewer Module
		{Key: "modules.memory_viewer.enabled", Label: "Enable Memory Viewer", Description: "Turn this ON to browse your AI agent's memory files like MEMORY.md, HEARTBEAT.md, and daily notes.", Type: "bool", Default: true, Category: "Memory Viewer"},
		{Key: "modules.memory_viewer.port", Label: "Module Port", Description: "Internal port for the Memory Viewer module.", Type: "int", Default: 9008, Category: "Memory Viewer"},
		{Key: "modules.memory_viewer.memory_dir", Label: "Memory Directory", Description: "Where your agent stores its memory files. Usually ~/.claude for Claude-based agents.", Type: "string", Default: "~/.claude", Category: "Memory Viewer"},
		{Key: "modules.memory_viewer.watch_files", Label: "Files to Watch", Description: "Specific memory files to monitor and display. Add any file names your agent uses for memory.", Type: "array", Default: []string{"MEMORY.md", "HEARTBEAT.md"}, Category: "Memory Viewer"},

		// Service Control Module
		{Key: "modules.service_control.enabled", Label: "Enable Service Control", Description: "Turn this ON to restart or manage services directly from the dashboard.", Type: "bool", Default: true, Category: "Service Control"},
		{Key: "modules.service_control.port", Label: "Module Port", Description: "Internal port for the Service Control module.", Type: "int", Default: 9009, Category: "Service Control"},
		{Key: "modules.service_control.services", Label: "Managed Services", Description: "List of system service names that can be controlled from the dashboard.", Type: "array", Default: []string{"openclaw", "openclaw-dashboard"}, Category: "Service Control"},
		{Key: "modules.service_control.allow_restart", Label: "Allow Restart", Description: "When ON, you can restart services from the dashboard. Useful for quick fixes.", Type: "bool", Default: true, Category: "Service Control"},
		{Key: "modules.service_control.allow_stop", Label: "Allow Stop", Description: "When ON, you can stop services from the dashboard. Be careful - stopping essential services can cause downtime!", Type: "bool", Default: false, Category: "Service Control", Warning: "Enabling this lets anyone on your network stop services. Make sure you trust all LAN users."},

		// Cron Manager Module
		{Key: "modules.cron_manager.enabled", Label: "Enable Cron Manager", Description: "Turn this ON to view, enable/disable, and manually trigger scheduled cron jobs.", Type: "bool", Default: true, Category: "Cron Manager"},
		{Key: "modules.cron_manager.port", Label: "Module Port", Description: "Internal port for the Cron Manager module.", Type: "int", Default: 9010, Category: "Cron Manager"},
		{Key: "modules.cron_manager.allow_edit", Label: "Allow Editing", Description: "When ON, cron jobs can be edited from the dashboard. Turn OFF for view-only mode.", Type: "bool", Default: true, Category: "Cron Manager"},
		{Key: "modules.cron_manager.allow_trigger", Label: "Allow Manual Trigger", Description: "When ON, you can manually run a cron job immediately without waiting for its schedule.", Type: "bool", Default: true, Category: "Cron Manager"},

		// Notifications
		{Key: "notifications.enabled", Label: "Enable Notifications", Description: "Master switch for all notifications. Turn OFF to disable all alerts.", Type: "bool", Default: true, Category: "Notifications"},
		{Key: "notifications.browser_notifications", Label: "Browser Notifications", Description: "When ON, your browser will show popup notifications for important events. You'll need to allow notifications in your browser.", Type: "bool", Default: true, Category: "Notifications"},
		{Key: "notifications.rate_limit_warning", Label: "Rate Limit Alerts", Description: "Get notified when you're approaching your API rate limits.", Type: "bool", Default: true, Category: "Notifications"},
		{Key: "notifications.budget_warning", Label: "Budget Alerts", Description: "Get notified when your spending reaches warning or critical levels.", Type: "bool", Default: true, Category: "Notifications"},
		{Key: "notifications.service_down_alert", Label: "Service Down Alerts", Description: "Get notified when a monitored service goes down.", Type: "bool", Default: true, Category: "Notifications"},

		// Security
		{Key: "security.lan_only", Label: "LAN Only Access", Description: "When ON, only devices on your local network can access the dashboard. Highly recommended to keep this ON.", Type: "bool", Default: true, Category: "Security", Warning: "Turning this OFF exposes the dashboard to the internet if your router forwards the port!"},
		{Key: "security.allowed_networks", Label: "Allowed Networks", Description: "List of network ranges (in CIDR format) that can access the dashboard. The defaults cover all common home/office networks.", Type: "array", Default: []string{"192.168.0.0/16", "10.0.0.0/8", "172.16.0.0/12"}, Category: "Security"},
		{Key: "security.readonly_mode", Label: "Read-Only Mode", Description: "When ON, nobody can make changes through the dashboard - view only. Good if you want to let others monitor but not modify.", Type: "bool", Default: false, Category: "Security"},

		// Advanced
		{Key: "advanced.log_level", Label: "Log Level", Description: "How much detail to include in logs. 'info' is normal, 'debug' shows extra details for troubleshooting, 'error' only shows problems.", Type: "string", Default: "info", Category: "Advanced"},
		{Key: "advanced.max_module_restarts", Label: "Max Module Restarts", Description: "How many times to automatically restart a crashed module before giving up. Prevents infinite restart loops.", Type: "int", Default: 5, Category: "Advanced"},
		{Key: "advanced.restart_backoff_seconds", Label: "Restart Delay (seconds)", Description: "Base delay between restart attempts. Each attempt waits longer (2s, 4s, 8s...) to avoid hammering a broken module.", Type: "int", Default: 2, Category: "Advanced"},
		{Key: "advanced.sse_keepalive_seconds", Label: "SSE Keepalive (seconds)", Description: "How often the server sends a heartbeat to keep the real-time connection alive. 30 seconds is fine for most networks.", Type: "int", Default: 30, Category: "Advanced"},
		{Key: "advanced.module_health_check_seconds", Label: "Health Check Interval (seconds)", Description: "How often to check if modules are still running and healthy. 10 seconds balances responsiveness with low overhead.", Type: "int", Default: 10, Category: "Advanced"},
		{Key: "advanced.data_retention_days", Label: "Data Retention (days)", Description: "How many days of historical data (metrics, logs, sessions) to keep. Older data is automatically deleted to save disk space.", Type: "int", Default: 30, Category: "Advanced"},
	}
}
