package utils

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/Pimeng/gphira-mp-next/internal/l10n"
	"github.com/Pimeng/gphira-mp-next/pkg/roomid"
)

// LogLevel represents the severity of a log message.
type LogLevel string

const (
	LevelDebug LogLevel = "DEBUG"
	LevelInfo  LogLevel = "INFO"
	LevelMark  LogLevel = "MARK"
	LevelWarn  LogLevel = "WARN"
	LevelError LogLevel = "ERROR"
)

var levelWeights = map[LogLevel]int{
	LevelDebug: 10,
	LevelInfo:  20,
	LevelMark:  25,
	LevelWarn:  30,
	LevelError: 40,
}

// LogContext provides contextual metadata for log entries.
type LogContext struct {
	UserID         int32
	IP             string
	RoomID         string
	IsConnectionLog bool
}

// Logger provides structured logging with file rotation, rate limiting, and color support.
type Logger struct {
	minLevel        LogLevel
	consoleMinLevel LogLevel
	useColor        bool
	logsDir         string
	fileMu          sync.Mutex
	fileStream      *os.File
	currentDateKey  string
	rateLimiter     *RateLimiter
	testAccountIDs  map[int32]struct{}
	slogLogger      *slog.Logger
}

// NewLogger creates a new Logger.
func NewLogger(level string) *Logger {
	minLvl := parseLogLevel(level)
	useColor := shouldUseColor()
	l := &Logger{
		minLevel:        minLvl,
		consoleMinLevel: minLvl,
		useColor:        useColor,
		logsDir:         "logs",
		testAccountIDs:  make(map[int32]struct{}),
	}
	_ = os.MkdirAll(l.logsDir, 0755)
	// Avoid duplicate output in interactive terminals
	var slogOut io.Writer = os.Stderr
	if useColor {
		slogOut = io.Discard
	}
	l.slogLogger = slog.New(slog.NewTextHandler(slogOut, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	return l
}

// SetRateLimiter attaches a rate limiter for connection logs.
func (l *Logger) SetRateLimiter(rl *RateLimiter) {
	l.rateLimiter = rl
}

// SetTestAccountIDs configures account IDs whose logs are skipped from file output.
func (l *Logger) SetTestAccountIDs(ids []int) {
	l.fileMu.Lock()
	defer l.fileMu.Unlock()
	l.testAccountIDs = make(map[int32]struct{})
	for _, id := range ids {
		l.testAccountIDs[int32(id)] = struct{}{}
	}
}

func extractContext(args []any) ([]any, *LogContext) {
	if len(args) > 0 {
		if c, ok := args[len(args)-1].(*LogContext); ok {
			return args[:len(args)-1], c
		}
	}
	return args, nil
}

// Debug logs a debug message.
func (l *Logger) Debug(msg string, args ...any) {
	args, ctx := extractContext(args)
	l.write(LevelDebug, msg, args, ctx)
}

// Info logs an info message.
func (l *Logger) Info(msg string, args ...any) {
	args, ctx := extractContext(args)
	l.write(LevelInfo, msg, args, ctx)
}

// Mark logs a marked info message.
func (l *Logger) Mark(msg string, args ...any) {
	args, ctx := extractContext(args)
	l.write(LevelMark, msg, args, ctx)
}

// Warn logs a warning message.
func (l *Logger) Warn(msg string, args ...any) {
	args, ctx := extractContext(args)
	l.write(LevelWarn, msg, args, ctx)
}

// Error logs an error message.
func (l *Logger) Error(msg string, args ...any) {
	args, ctx := extractContext(args)
	l.write(LevelError, msg, args, ctx)
}

// Log writes a message with an explicit level and optional context.
func (l *Logger) Log(level LogLevel, msg string, args ...any) {
	var ctx *LogContext
	if len(args) > 0 {
		if c, ok := args[len(args)-1].(*LogContext); ok {
			ctx = c
			args = args[:len(args)-1]
		}
	}
	l.write(level, msg, args, ctx)
}

// Logf provides fmt.Sprintf style logging at INFO level.
func (l *Logger) Logf(format string, args ...any) {
	l.write(LevelInfo, fmt.Sprintf(format, args...), nil, nil)
}

// LogRoomInfo logs a localized room event at INFO level.
func (l *Logger) LogRoomInfo(lang *l10n.Language, roomID roomid.RoomID, key string, args map[string]string) {
	if l == nil || lang == nil {
		return
	}
	msg := lang.Format(key, args)
	l.write(LevelInfo, msg, []any{"room_id", string(roomID)}, nil)
}

// LogRoomMark logs a localized room event at MARK level.
func (l *Logger) LogRoomMark(lang *l10n.Language, roomID roomid.RoomID, key string, args map[string]string) {
	if l == nil || lang == nil {
		return
	}
	msg := lang.Format(key, args)
	l.write(LevelMark, msg, []any{"room_id", string(roomID)}, nil)
}

// DebugL logs a localized debug message.
func (l *Logger) DebugL(lang *l10n.Language, key string, args map[string]string, extra ...any) {
	if l == nil || lang == nil {
		return
	}
	msg := lang.Format(key, args)
	extra, ctx := extractContext(extra)
	l.write(LevelDebug, msg, extra, ctx)
}

// InfoL logs a localized info message.
func (l *Logger) InfoL(lang *l10n.Language, key string, args map[string]string, extra ...any) {
	if l == nil || lang == nil {
		return
	}
	msg := lang.Format(key, args)
	extra, ctx := extractContext(extra)
	l.write(LevelInfo, msg, extra, ctx)
}

// MarkL logs a localized mark message.
func (l *Logger) MarkL(lang *l10n.Language, key string, args map[string]string, extra ...any) {
	if l == nil || lang == nil {
		return
	}
	msg := lang.Format(key, args)
	extra, ctx := extractContext(extra)
	l.write(LevelMark, msg, extra, ctx)
}

// WarnL logs a localized warning message.
func (l *Logger) WarnL(lang *l10n.Language, key string, args map[string]string, extra ...any) {
	if l == nil || lang == nil {
		return
	}
	msg := lang.Format(key, args)
	extra, ctx := extractContext(extra)
	l.write(LevelWarn, msg, extra, ctx)
}

// ErrorL logs a localized error message.
func (l *Logger) ErrorL(lang *l10n.Language, key string, args map[string]string, extra ...any) {
	if l == nil || lang == nil {
		return
	}
	msg := lang.Format(key, args)
	extra, ctx := extractContext(extra)
	l.write(LevelError, msg, extra, ctx)
}

// LogfL logs a localized formatted message at INFO level.
func (l *Logger) LogfL(lang *l10n.Language, key string, args map[string]string) {
	if l == nil || lang == nil {
		return
	}
	msg := lang.Format(key, args)
	l.write(LevelInfo, msg, nil, nil)
}

// GetBlacklistedIPs returns currently blacklisted IPs.
func (l *Logger) GetBlacklistedIPs() []struct{ IP string; ExpiresIn int64 } {
	if l.rateLimiter == nil {
		return nil
	}
	return l.rateLimiter.GetBlacklistedIPs()
}

// RemoveFromBlacklist removes an IP from the blacklist.
func (l *Logger) RemoveFromBlacklist(ip string) {
	if l.rateLimiter == nil {
		return
	}
	l.rateLimiter.RemoveFromBlacklist(ip)
}

// ClearBlacklist clears all blacklisted IPs.
func (l *Logger) ClearBlacklist() {
	if l.rateLimiter == nil {
		return
	}
	l.rateLimiter.ClearBlacklist()
}

// GetLevel returns the current minimum log level.
func (l *Logger) GetLevel() string {
	return string(l.minLevel)
}

// UpdateOptions updates logger options at runtime.
func (l *Logger) UpdateOptions(level string, testAccountIDs []int) {
	l.fileMu.Lock()
	defer l.fileMu.Unlock()
	if level != "" {
		l.minLevel = parseLogLevel(level)
		l.consoleMinLevel = l.minLevel
	}
	if testAccountIDs != nil {
		l.testAccountIDs = make(map[int32]struct{})
		for _, id := range testAccountIDs {
			l.testAccountIDs[int32(id)] = struct{}{}
		}
	}
}

func (l *Logger) write(level LogLevel, msg string, args []any, ctx *LogContext) {
	if l == nil {
		return
	}

	// Check level
	if levelWeights[level] < levelWeights[l.minLevel] {
		return
	}

	// Rate limiting for connection logs
	if ctx != nil && ctx.IsConnectionLog && l.rateLimiter != nil && ctx.IP != "" {
		if !l.rateLimiter.ShouldLogConnection(ctx.IP) {
			return
		}
	}

	// Build the log line
	now := time.Now()
	dateKey := formatDateKey(now)
	ts := formatTimestamp(now)
	meta := formatMeta(args)
	fileLine := fmt.Sprintf("[%s] [%s] %s%s\n", ts, level, msg, meta)

	// File output (with rotation)
	var skipFile bool
	if l.testAccountIDs != nil && ctx != nil {
		if _, ok := l.testAccountIDs[ctx.UserID]; ok && l.minLevel != LevelDebug {
			skipFile = true
		}
	}

	if !skipFile {
		l.fileMu.Lock()
		if l.currentDateKey != dateKey {
			l.rotate(dateKey)
		}
		if l.fileStream != nil {
			_, _ = l.fileStream.WriteString(fileLine)
		}
		l.fileMu.Unlock()
	}

	// Console output with color
	if levelWeights[level] >= levelWeights[l.consoleMinLevel] {
		var consoleLine string
		if l.useColor {
			consoleLine = formatConsoleLine(ts, level, msg)
		} else {
			consoleLine = fileLine
		}
		if level == LevelError || level == LevelWarn {
			_, _ = os.Stderr.WriteString(consoleLine)
		} else {
			_, _ = os.Stdout.WriteString(consoleLine)
		}
	}

	// Also log to slog for structured stderr output
	if l.slogLogger != nil {
		switch level {
		case LevelDebug:
			l.slogLogger.Debug(msg, args...)
		case LevelWarn:
			l.slogLogger.Warn(msg, args...)
		case LevelError:
			l.slogLogger.Error(msg, args...)
		default:
			l.slogLogger.Info(msg, args...)
		}
	}
}

func (l *Logger) rotate(dateKey string) {
	if l.fileStream != nil {
		_ = l.fileStream.Close()
	}
	l.currentDateKey = dateKey
	path := filepath.Join(l.logsDir, dateKey+".log")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		l.fileStream = nil
		return
	}
	l.fileStream = f
}

func parseLogLevel(level string) LogLevel {
	switch strings.ToUpper(strings.TrimSpace(level)) {
	case "DEBUG":
		return LevelDebug
	case "INFO":
		return LevelInfo
	case "MARK":
		return LevelMark
	case "WARN":
		return LevelWarn
	case "ERROR":
		return LevelError
	default:
		return LevelInfo
	}
}

func shouldUseColor() bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	if os.Getenv("TERM") == "dumb" {
		return false
	}
	// Best-effort: check if stdout is a terminal
	if fi, _ := os.Stdout.Stat(); fi != nil {
		if fi.Mode()&os.ModeCharDevice != 0 {
			return true
		}
	}
	// Windows terminals generally support ANSI colors
	if runtime.GOOS == "windows" {
		return true
	}
	return false
}

func formatConsoleLine(ts string, level LogLevel, msg string) string {
	gray := "\x1b[90m"
	reset := "\x1b[0m"

	var levelColor string
	switch level {
	case LevelDebug:
		levelColor = "\x1b[34m" // blue
	case LevelInfo:
		levelColor = "\x1b[32m" // green
	case LevelMark:
		levelColor = "\x1b[90m" // gray
	case LevelWarn:
		levelColor = "\x1b[33m" // yellow
	case LevelError:
		levelColor = "\x1b[31m" // red
	default:
		levelColor = reset
	}

	return fmt.Sprintf("%s[%s]%s %s[%s]%s %s%s\n",
		gray, ts, reset,
		levelColor, level, reset,
		msg, reset)
}

func formatDateKey(t time.Time) string {
	return fmt.Sprintf("%04d-%02d-%02d", t.Year(), t.Month(), t.Day())
}

func formatTimestamp(t time.Time) string {
	return fmt.Sprintf("%04d-%02d-%02d %02d:%02d:%02d.%03d",
		t.Year(), t.Month(), t.Day(),
		t.Hour(), t.Minute(), t.Second(),
		t.Nanosecond()/1e6)
}

func formatMeta(args []any) string {
	if len(args) == 0 {
		return ""
	}
	// Convert slog-style key-value pairs to key: value string
	var pairs []string
	for i := 0; i+1 < len(args); i += 2 {
		key := fmt.Sprintf("%v", args[i])
		val := fmt.Sprintf("%v", args[i+1])
		pairs = append(pairs, fmt.Sprintf("%s: %s", key, val))
	}
	if len(pairs) == 0 {
		return ""
	}
	return " " + strings.Join(pairs, " ")
}
