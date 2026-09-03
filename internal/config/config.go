package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	ListenAddr string
	DataDir    string
	Timezone   *time.Location
	LogLevel   string

	PortainerURL         string
	PortainerAPIKey      string
	PortainerEndpointIDs []int

	DockerHost string
	SmartURL   string
	HostProc   string
	HostSys    string
	HostRoot   string

	TrackedMounts   []string
	ExcludePatterns []string
	OwnStack        string

	StatsInterval  time.Duration
	HostInterval   time.Duration
	UpdateInterval time.Duration
	StatsWorkers   int
	StatsTimeout   time.Duration

	RetentionDays int
	DBMaxBytes    int64

	AdminPassword     string
	AdminPasswordHash string
	SessionSecret     string
	SessionTTL        time.Duration
	SecureCookies     bool

	TelegramBotToken string
	TelegramChatID   string

	RegistryAuth map[string]RegistryCredential

	Alerts AlertThresholds

	LogErrorPatterns []string
}

type RegistryCredential struct {
	Username string
	Password string
}

type AlertThresholds struct {
	CPUPercent         float64
	MemoryPercent      float64
	DiskPercent        float64
	DiskTempCelsius    int
	RestartLoopCount   int
	RestartLoopWindow  time.Duration
	SustainedFor       time.Duration
	Debounce           time.Duration
	HostUnreachableFor time.Duration
}

func Load() (*Config, error) {
	var errs []error
	fail := func(format string, args ...any) {
		errs = append(errs, fmt.Errorf(format, args...))
	}

	c := &Config{
		ListenAddr:       envString("LISTEN_ADDR", ":8080"),
		DataDir:          envString("DATA_DIR", "/data"),
		LogLevel:         strings.ToLower(envString("LOG_LEVEL", "info")),
		PortainerURL:     strings.TrimRight(os.Getenv("PORTAINER_URL"), "/"),
		DockerHost:       strings.TrimRight(envString("DOCKER_HOST", "http://socket-proxy:2375"), "/"),
		SmartURL:         strings.TrimRight(envString("SMART_URL", "http://smart:9633"), "/"),
		HostProc:         envString("HOST_PROC", "/host/proc"),
		HostSys:          envString("HOST_SYS", "/host/sys"),
		HostRoot:         envString("HOST_ROOT", "/hostfs"),
		TrackedMounts:    envList("TRACKED_MOUNTS", "/,/Volume1"),
		ExcludePatterns:  envList("EXCLUDE_PATTERNS", ""),
		OwnStack:         envString("OWN_STACK", "homelab-dashboard"),
		StatsWorkers:     envInt("STATS_WORKERS", 8, &errs),
		RetentionDays:    envInt("RETENTION_DAYS", 90, &errs),
		SessionTTL:       envDuration("SESSION_TTL", 7*24*time.Hour, &errs),
		SecureCookies:    envBool("SECURE_COOKIES", true, &errs),
		TelegramBotToken: os.Getenv("TELEGRAM_BOT_TOKEN"),
		TelegramChatID:   os.Getenv("TELEGRAM_CHAT_ID"),
		LogErrorPatterns: envList("LOG_ERROR_PATTERNS", "(?i)error|exception|fatal|panic|traceback"),
	}

	c.StatsInterval = envDuration("STATS_INTERVAL", 15*time.Second, &errs)
	c.HostInterval = envDuration("HOST_INTERVAL", 60*time.Second, &errs)
	c.UpdateInterval = envDuration("UPDATE_INTERVAL", 6*time.Hour, &errs)
	c.StatsTimeout = envDuration("STATS_TIMEOUT", 5*time.Second, &errs)
	c.DBMaxBytes = int64(envInt("DB_MAX_MB", 500, &errs)) * 1024 * 1024

	c.Alerts = AlertThresholds{
		CPUPercent:         envFloat("ALERT_CPU_PCT", 90, &errs),
		MemoryPercent:      envFloat("ALERT_MEM_PCT", 90, &errs),
		DiskPercent:        envFloat("ALERT_DISK_PCT", 90, &errs),
		DiskTempCelsius:    envInt("ALERT_DISK_TEMP_C", 55, &errs),
		RestartLoopCount:   envInt("ALERT_RESTART_COUNT", 3, &errs),
		RestartLoopWindow:  envDuration("ALERT_RESTART_WINDOW", 10*time.Minute, &errs),
		SustainedFor:       envDuration("ALERT_SUSTAINED_FOR", 5*time.Minute, &errs),
		Debounce:           envDuration("ALERT_DEBOUNCE", 2*time.Minute, &errs),
		HostUnreachableFor: envDuration("ALERT_HOST_UNREACHABLE_FOR", 2*time.Minute, &errs),
	}

	tzName := envString("TZ", "Asia/Riyadh")
	loc, err := time.LoadLocation(tzName)
	if err != nil {
		fail("TZ %q is not a valid IANA zone", tzName)
	} else {
		c.Timezone = loc
	}

	if c.PortainerURL == "" {
		fail("PORTAINER_URL is required")
	} else if !strings.HasPrefix(c.PortainerURL, "http://") && !strings.HasPrefix(c.PortainerURL, "https://") {
		fail("PORTAINER_URL must start with http:// or https://")
	}

	key, err := secretValue("PORTAINER_API_KEY")
	if err != nil {
		fail("%v", err)
	}
	if key == "" {
		fail("PORTAINER_API_KEY or PORTAINER_API_KEY_FILE is required")
	}
	c.PortainerAPIKey = key

	for _, raw := range envList("PORTAINER_ENDPOINT_IDS", "") {
		id, convErr := strconv.Atoi(raw)
		if convErr != nil || id <= 0 {
			fail("PORTAINER_ENDPOINT_IDS contains invalid id %q", raw)
			continue
		}
		c.PortainerEndpointIDs = append(c.PortainerEndpointIDs, id)
	}

	pw, err := secretValue("ADMIN_PASSWORD")
	if err != nil {
		fail("%v", err)
	}
	c.AdminPassword = pw
	c.AdminPasswordHash = os.Getenv("ADMIN_PASSWORD_HASH")
	if c.AdminPassword == "" && c.AdminPasswordHash == "" {
		fail("ADMIN_PASSWORD, ADMIN_PASSWORD_FILE, or ADMIN_PASSWORD_HASH is required")
	}
	if c.AdminPassword != "" && len(c.AdminPassword) < 8 {
		fail("ADMIN_PASSWORD must be at least 8 characters")
	}

	secret, err := secretValue("SESSION_SECRET")
	if err != nil {
		fail("%v", err)
	}
	c.SessionSecret = secret

	if (c.TelegramBotToken == "") != (c.TelegramChatID == "") {
		fail("TELEGRAM_BOT_TOKEN and TELEGRAM_CHAT_ID must be set together")
	}

	c.RegistryAuth, err = parseRegistryAuth(os.Getenv("REGISTRY_AUTH"))
	if err != nil {
		fail("%v", err)
	}

	if c.StatsWorkers < 1 || c.StatsWorkers > 64 {
		fail("STATS_WORKERS must be between 1 and 64")
	}
	if c.StatsInterval < 5*time.Second {
		fail("STATS_INTERVAL must be at least 5s")
	}
	if c.HostInterval < 10*time.Second {
		fail("HOST_INTERVAL must be at least 10s")
	}
	if c.StatsTimeout >= c.StatsInterval {
		fail("STATS_TIMEOUT must be shorter than STATS_INTERVAL")
	}
	if c.RetentionDays < 1 {
		fail("RETENTION_DAYS must be at least 1")
	}
	if c.DBMaxBytes < 16*1024*1024 {
		fail("DB_MAX_MB must be at least 16")
	}
	switch c.LogLevel {
	case "debug", "info", "warn", "error":
	default:
		fail("LOG_LEVEL must be debug, info, warn, or error")
	}
	if !filepath.IsAbs(c.DataDir) {
		fail("DATA_DIR must be an absolute path")
	}
	for _, m := range c.TrackedMounts {
		if !strings.HasPrefix(m, "/") {
			fail("TRACKED_MOUNTS entry %q must be an absolute path", m)
		}
	}

	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	return c, nil
}

func (c *Config) HostPath(mount string) string {
	return filepath.Join(c.HostRoot, mount)
}

func envString(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && strings.TrimSpace(v) != "" {
		return strings.TrimSpace(v)
	}
	return def
}

func envList(key, def string) []string {
	raw := envString(key, def)
	if raw == "" {
		return nil
	}
	var out []string
	for _, part := range strings.Split(raw, ",") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func envInt(key string, def int, errs *[]error) int {
	raw, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(raw) == "" {
		return def
	}
	v, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		*errs = append(*errs, fmt.Errorf("%s must be an integer, got %q", key, raw))
		return def
	}
	return v
}

func envFloat(key string, def float64, errs *[]error) float64 {
	raw, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(raw) == "" {
		return def
	}
	v, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil {
		*errs = append(*errs, fmt.Errorf("%s must be a number, got %q", key, raw))
		return def
	}
	return v
}

func envBool(key string, def bool, errs *[]error) bool {
	raw, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(raw) == "" {
		return def
	}
	v, err := strconv.ParseBool(strings.TrimSpace(raw))
	if err != nil {
		*errs = append(*errs, fmt.Errorf("%s must be true or false, got %q", key, raw))
		return def
	}
	return v
}

func envDuration(key string, def time.Duration, errs *[]error) time.Duration {
	raw, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(raw) == "" {
		return def
	}
	v, err := time.ParseDuration(strings.TrimSpace(raw))
	if err != nil {
		*errs = append(*errs, fmt.Errorf("%s must be a duration like 15s or 6h, got %q", key, raw))
		return def
	}
	return v
}

func secretValue(key string) (string, error) {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v, nil
	}
	path := strings.TrimSpace(os.Getenv(key + "_FILE"))
	if path == "" {
		return "", nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("%s_FILE could not be read: %w", key, err)
	}
	return strings.TrimSpace(string(data)), nil
}

func parseRegistryAuth(raw string) (map[string]RegistryCredential, error) {
	out := map[string]RegistryCredential{}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return out, nil
	}
	for _, entry := range strings.Split(raw, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		host, cred, ok := strings.Cut(entry, "=")
		if !ok {
			return nil, fmt.Errorf("REGISTRY_AUTH entry %q must look like host=user:password", redact(entry))
		}
		user, pass, ok := strings.Cut(cred, ":")
		if !ok || user == "" || pass == "" {
			return nil, fmt.Errorf("REGISTRY_AUTH entry for %q must include user:password", host)
		}
		out[strings.TrimSpace(host)] = RegistryCredential{Username: user, Password: pass}
	}
	return out, nil
}

func redact(s string) string {
	if len(s) <= 4 {
		return "****"
	}
	return s[:4] + "****"
}
