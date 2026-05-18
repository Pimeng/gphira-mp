package l10n

import (
	"strings"
)

// Language represents a language instance for localization.
type Language struct {
	lang string
}

// Lang returns the language code.
func (l *Language) Lang() string {
	if l == nil {
		return "zh-CN"
	}
	return l.lang
}

var translations = map[string]map[string]string{
	"zh-CN": {
		"room-only-host":              "只有房主可以执行此操作",
		"room-not-whitelisted":        "你不在该房间白名单中",
		"join-room-locked":            "房间已锁定",
		"join-game-ongoing":           "游戏正在进行中",
		"join-cant-monitor":           "权限不足，不能旁观房间",
		"start-no-chart-selected":     "还没有选择谱面",
		"room-invalid-state":          "房间状态不允许此操作",
		"room-already-in-room":        "已在房间中",
		"create-id-occupied":          "房间 ID 已被占用",
		"room-not-found":              "房间不存在",
		"join-room-full":              "房间已满",
		"user-banned-by-server":       "你已被服务器封禁，无法进行任何操作。",
		"room-banned":                 "你已被禁止进入房间 { $id }",
		"room-creation-disabled":      "房间创建功能已被管理员禁用",
		"chart-fetch-failed":          "获取谱面失败",
		"record-fetch-failed":         "获取记录失败",
		"record-invalid":              "记录不合法",
		"record-already-uploaded":     "已上传记录",
		"room-game-aborted":           "对局已中止",
		"auth-repeated-authenticate":  "重复认证",
		"chat-welcome":                "\"{ $userName }\"你好！欢迎来到 { $serverName } 服务器！",
		"chat-hitokoto-from-unknown":  "佚名",
		"chat-roomlist-title":         "当前可用的房间如下：",
		"chat-roomlist-empty":         "当前没有可用房间",
		"chat-roomlist-item":          "{ $id }（{ $count }/{ $max }）",
		"chat-disabled-by-server":     "为避免安全问题，该服务器已禁用聊天",
		"chat-game-summary":           "本局结算：\n{ $scoreText }\n{ $accText }\n{ $stdText }",
		"chat-game-summary-score":     "最高分：\"{ $name } \"({ $id }) { $score }",
		"chat-game-summary-acc":       "最高准度：\"{ $name } \"({ $id }) { $acc }",
		"chat-game-summary-std":       "最佳无瑕度：\"{ $name } \"({ $id }) { $std }ms",
		"log-room-created":            "\"{ $user }\" 创建房间 \"{ $room }\"",
		"log-room-joined":             "\"{ $user }\"{ $suffix } 加入房间 \"{ $room }\"",
		"log-room-left":               "\"{ $user }\"{ $suffix } 离开房间 \"{ $room }\"",
		"log-room-recycled":           "房间 \"{ $room }\" 已回收（无玩家）",
		"log-room-game-start":         "房间 \"{ $room }\" 对局开始，玩家：{ $users }{ $monitorsSuffix }",
		"log-room-game-end":           "房间 \"{ $room }\" 对局结束（已上传：{ $uploaded }，中止：{ $aborted }）",
		"log-room-lock":               "\"{ $user }\" 将房间 \"{ $room }\" { $lock }",
		"log-room-cycle":              "\"{ $user }\" 将房间 \"{ $room }\" { $cycle }",
		"log-room-select-chart":       "\"{ $user }\"（用户ID：{ $userId }）在房间 \"{ $room }\" 选择了 \"{ $chart }\"",
		"log-room-request-start":      "\"{ $user }\" 在房间 \"{ $room }\" 请求开始对局",
		"log-user-chat":               "\"{ $user }\" 在房间 \"{ $room }\" 发送聊天消息",
		"log-room-lock-locked":        "设为锁定",
		"log-room-lock-unlocked":      "取消锁定",
		"log-room-cycle-on":           "开启轮转房主",
		"log-room-cycle-off":          "关闭轮转房主",
		"log-server-starting":         "正在启动 GPhira MP 服务端",
		"log-server-listening":        "服务端正在偷听 { $addr }",
		"log-server-info":             "服务端昵称：{ $name } ，日志等级{ $level }",
		"log-http-started":            "HTTP 服务已启动 { $addr }",
		"log-redis-enabled":           "Redis 缓存已启用",
		"log-shutting-down":           "正在关闭服务端",
		"cli-welcome":                 "欢迎使用 GPhira MP CLI. 输入 \"help\" 获取命令列表",
	},
	"en-US": {
		"room-only-host":              "Only the host can do this",
		"room-not-whitelisted":        "You are not whitelisted for this room",
		"join-room-locked":            "Room is locked",
		"join-game-ongoing":           "Game is ongoing",
		"join-cant-monitor":           "Permission denied. You can't monitor this room.",
		"start-no-chart-selected":     "No chart selected",
		"room-invalid-state":          "Invalid room state",
		"room-already-in-room":        "Already in a room",
		"create-id-occupied":          "Room ID is occupied",
		"room-not-found":              "Room not found",
		"join-room-full":              "Room is full",
		"user-banned-by-server":       "You have been banned from this server and cannot perform any operations.",
		"room-banned":                 "You are banned from room { $id }",
		"room-creation-disabled":      "Room creation has been disabled by administrator",
		"chart-fetch-failed":          "Failed to fetch chart",
		"record-fetch-failed":         "Failed to fetch record",
		"record-invalid":              "Invalid record",
		"record-already-uploaded":     "Record already uploaded",
		"room-game-aborted":           "Game aborted",
		"auth-repeated-authenticate":  "Repeated authenticate",
		"chat-welcome":                "Hello \"{ $userName }\"! Welcome to { $serverName }!",
		"chat-hitokoto-from-unknown":  "Unknown",
		"chat-roomlist-title":         "Available rooms:",
		"chat-roomlist-empty":         "No available rooms",
		"chat-roomlist-item":          "{ $id } ({ $count }/{ $max })",
		"chat-disabled-by-server":     "Chat is disabled on this server to avoid safety issues.",
		"chat-game-summary":           "Match summary:\n{ $scoreText }\n{ $accText }\n{ $stdText }",
		"chat-game-summary-score":     "Best score: \"{ $name } \"({ $id }) { $score }",
		"chat-game-summary-acc":       "Best accuracy: \"{ $name } \"({ $id }) { $acc }",
		"chat-game-summary-std":       "Best std: \"{ $name } \"({ $id }) { $std }ms",
		"log-room-created":            "\"{ $user }\" created room \"{ $room }\"",
		"log-room-joined":             "\"{ $user }\"{ $suffix } joined room \"{ $room }\"",
		"log-room-left":               "\"{ $user }\"{ $suffix } left room \"{ $room }\"",
		"log-room-recycled":           "Room \"{ $room }\" recycled (empty)",
		"log-room-game-start":         "Room \"{ $room }\" game start. users: { $users }{ $monitorsSuffix }",
		"log-room-game-end":           "Room \"{ $room }\" game end (uploaded={ $uploaded }, aborted={ $aborted })",
		"log-room-lock":               "\"{ $user }\" { $lock } room \"{ $room }\"",
		"log-room-cycle":              "\"{ $user }\" { $cycle } room \"{ $room }\"",
		"log-room-select-chart":       "\"{ $user }\" (ID: { $userId }) selected \"{ $chart }\" in room \"{ $room }\"",
		"log-room-request-start":      "\"{ $user }\" requested start in room \"{ $room }\"",
		"log-user-chat":               "\"{ $user }\" sent chat in room \"{ $room }\"",
		"log-room-lock-locked":        "locked",
		"log-room-lock-unlocked":      "unlocked",
		"log-room-cycle-on":           "enabled cycle host",
		"log-room-cycle-off":          "disabled cycle host",
		"log-server-starting":         "Starting GPhira MP server",
		"log-server-listening":        "Server listening on { $addr }",
		"log-server-info":             "Server name: { $name }, log level: { $level }",
		"log-http-started":            "HTTP service started on { $addr }",
		"log-redis-enabled":           "Redis cache enabled",
		"log-shutting-down":           "Shutting down server",
		"cli-welcome":                 "Welcome to GPhira MP CLI. Type \"help\" for available commands.",
	},
}

// New creates a new Language instance.
func New(lang string) *Language {
	return &Language{lang: lang}
}

// Format returns the translated string for the given key,
// performing simple variable replacement using args.
// If the key is not found, the key itself is returned.
func (l *Language) Format(key string, args map[string]string) string {
	m, ok := translations[l.lang]
	if !ok {
		m = translations["zh-CN"]
	}
	tmpl, ok := m[key]
	if !ok {
		return key
	}
	if len(args) == 0 {
		return tmpl
	}
	result := tmpl
	for k, v := range args {
		result = strings.ReplaceAll(result, "{ $"+k+" }", v)
		result = strings.ReplaceAll(result, "{$"+k+"}", v)
	}
	return result
}

// TL is a shorthand for translating a key with the given language and arguments.
func TL(lang *Language, key string, args map[string]string) string {
	if lang == nil {
		return key
	}
	return lang.Format(key, args)
}
