package game

import (
	"fmt"

	"github.com/Pimeng/gphira-mp-next/internal/l10n"
	"github.com/Pimeng/gphira-mp-next/pkg/protocol"
)

// FormatMessageForLog formats a protocol.Message into a human-readable log string.
// It mirrors the behavior of the TypeScript room.formatMessageForLog method.
func FormatMessageForLog(msg protocol.Message, lang *l10n.Language, userName func(int32) string) string {
	if lang == nil {
		lang = l10n.New("")
	}
	name := func(id int32) string {
		if userName != nil {
			if n := userName(id); n != "" {
				return n
			}
		}
		return fmt.Sprintf("%d", id)
	}

	switch msg.Type {
	case protocol.MessageChat:
		return msg.Content
	case protocol.MessageCreateRoom:
		return lang.Format("log-msg-create-room", map[string]string{"user": name(msg.User)})
	case protocol.MessageJoinRoom:
		return lang.Format("log-msg-join-room", map[string]string{"name": msg.Name})
	case protocol.MessageLeaveRoom:
		return lang.Format("log-msg-leave-room", map[string]string{"name": msg.Name})
	case protocol.MessageNewHost:
		return lang.Format("log-msg-new-host", map[string]string{"user": name(msg.User)})
	case protocol.MessageSelectChart:
		return lang.Format("log-msg-select-chart", map[string]string{
			"user": name(msg.User),
			"name": msg.Name,
			"id":   fmt.Sprintf("%d", msg.ChartID),
		})
	case protocol.MessageGameStart:
		return lang.Format("log-msg-game-start", map[string]string{"user": name(msg.User)})
	case protocol.MessageReady:
		return lang.Format("log-msg-ready", map[string]string{"user": name(msg.User)})
	case protocol.MessageCancelReady:
		return lang.Format("log-msg-cancel-ready", map[string]string{"user": name(msg.User)})
	case protocol.MessageCancelGame:
		return lang.Format("log-msg-cancel-game", map[string]string{"user": name(msg.User)})
	case protocol.MessageStartPlaying:
		return lang.Format("log-msg-start-playing", nil)
	case protocol.MessagePlayed:
		fc := ""
		if msg.FullCombo {
			if lang.Lang() == "zh-CN" {
				fc = "，全连"
			} else {
				fc = ", FC"
			}
		}
		return lang.Format("log-msg-played", map[string]string{
			"user":  name(msg.User),
			"score": fmt.Sprintf("%d", msg.Score),
			"acc":   fmt.Sprintf("%.2f", float64(msg.Accuracy)*100),
			"fc":    fc,
		})
	case protocol.MessageGameEnd:
		return lang.Format("log-msg-game-end", nil)
	case protocol.MessageAbort:
		return lang.Format("log-msg-abort", map[string]string{"user": name(msg.User)})
	case protocol.MessageLockRoom:
		status := lang.Format("log-room-lock-unlocked", nil)
		if msg.Lock {
			status = lang.Format("log-room-lock-locked", nil)
		}
		return lang.Format("log-msg-lock-room", map[string]string{"status": status})
	case protocol.MessageCycleRoom:
		status := lang.Format("log-room-cycle-off", nil)
		if msg.Cycle {
			status = lang.Format("log-room-cycle-on", nil)
		}
		return lang.Format("log-msg-cycle-room", map[string]string{"status": status})
	default:
		return ""
	}
}
