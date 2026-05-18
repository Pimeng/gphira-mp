package network

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Pimeng/gphira-mp-next/internal/game"
	"github.com/Pimeng/gphira-mp-next/internal/l10n"
	"github.com/Pimeng/gphira-mp-next/internal/state"
	"github.com/Pimeng/gphira-mp-next/internal/utils"
	"github.com/Pimeng/gphira-mp-next/pkg/protocol"
)

const (
	roomListCacheTTLMs      = 2000
	roomListMaxCachedLangs  = 10
	hitokotoFetchTimeoutMs  = 3000
	hitokotoCacheTTLMs      = 60000
	hitokotoMinIntervalMs   = 600
	welcomeClearLines       = 30
	welcomeSeparatorLen     = 73
)

// HitokotoValue represents a cached hitokoto (quote).
type HitokotoValue struct {
	Quote string `json:"quote"`
	From  string `json:"from"`
}

// roomListCache holds cached room list text per language.
type roomListCache struct {
	mu        sync.RWMutex
	text      map[string]string
	timestamp int64
}

var globalRoomListCache = &roomListCache{
	text: make(map[string]string),
}

// hitokotoCache holds cached hitokoto with request coalescing.
type hitokotoCache struct {
	mu          sync.Mutex
	value       *HitokotoValue
	timestamp   int64
	lastAttempt int64
	inFlight    chan *HitokotoValue
}

var globalHitokotoCache = &hitokotoCache{}

// getCached returns the cached room list text if still valid.
func (c *roomListCache) getCached(lang string) (string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if time.Now().UnixMilli()-c.timestamp < roomListCacheTTLMs {
		if v, ok := c.text[lang]; ok {
			return v, true
		}
	}
	return "", false
}

// setCache stores the room list text for the given language.
func (c *roomListCache) setCache(lang, text string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.text) >= roomListMaxCachedLangs {
		for k := range c.text {
			delete(c.text, k)
			break
		}
	}
	c.text[lang] = text
	c.timestamp = time.Now().UnixMilli()
}

// getAvailableRoomsText returns localized text of available rooms.
func getAvailableRoomsText(st *state.ServerState, lang *l10n.Language) string {
	if cached, ok := globalRoomListCache.getCached(lang.Lang()); ok {
		return cached
	}

	type roomInfo struct {
		id    string
		count int
		max   int
	}

	var rooms []roomInfo
	st.WithRLock(func() {
		for id, room := range st.Rooms {
			if strings.HasPrefix(string(id), "_") {
				continue
			}
			if room.Locked {
				continue
			}
			if _, ok := room.State.(*game.StateSelectChart); !ok {
				if _, ok := room.State.(*game.StatePlaying); !ok {
					continue
				}
			}
			count := len(room.UserIDs())
			if count >= room.MaxUsers {
				continue
			}
			rooms = append(rooms, roomInfo{id: string(id), count: count, max: room.MaxUsers})
		}
	})

	sort.Slice(rooms, func(i, j int) bool {
		return rooms[i].id < rooms[j].id
	})

	if len(rooms) == 0 {
		return lang.Format("chat-roomlist-empty", nil)
	}

	joiner := "; "
	if lang.Lang() == "zh-CN" {
		joiner = "；"
	}

	items := make([]string, len(rooms))
	for i, r := range rooms {
		items[i] = lang.Format("chat-roomlist-item", map[string]string{
			"id":    r.id,
			"count": fmt.Sprintf("%d", r.count),
			"max":   fmt.Sprintf("%d", r.max),
		})
	}

	text := strings.Join(items, joiner)
	globalRoomListCache.setCache(lang.Lang(), text)
	return text
}

// fetchHitokoto fetches a hitokoto from the API.
func fetchHitokoto(proxy, url string) (*HitokotoValue, error) {
	if url == "" {
		return nil, nil
	}

	client := utils.NewHTTPClient(proxy, hitokotoFetchTimeoutMs*time.Millisecond)
	req, err := http.NewRequest("GET", strings.TrimSpace(url), nil)
	if err != nil {
		return nil, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("hitokoto API returned %d", resp.StatusCode)
	}

	var result struct {
		Hitokoto string `json:"hitokoto"`
		From     string `json:"from"`
		FromWho  string `json:"from_who"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	quote := strings.TrimSpace(result.Hitokoto)
	quote = strings.ReplaceAll(quote, `\n`, "\n")
	if quote == "" {
		return nil, nil
	}

	fromWho := strings.TrimSpace(result.FromWho)
	from := strings.TrimSpace(result.From)
	displayFrom := fromWho
	if displayFrom == "" {
		displayFrom = from
	}

	return &HitokotoValue{Quote: quote, From: displayFrom}, nil
}

// getHitokotoCached returns a cached hitokoto, fetching if necessary.
func getHitokotoCached(proxy, url string) *HitokotoValue {
	cache := globalHitokotoCache
	now := time.Now().UnixMilli()

	cache.mu.Lock()
	// Check cache
	if cache.value != nil && now-cache.timestamp < hitokotoCacheTTLMs {
		v := cache.value
		cache.mu.Unlock()
		return v
	}
	// Wait for in-flight request
	if cache.inFlight != nil {
		ch := cache.inFlight
		cache.mu.Unlock()
		return <-ch
	}
	// Rate limit
	if now-cache.lastAttempt < hitokotoMinIntervalMs {
		v := cache.value
		cache.mu.Unlock()
		return v
	}

	cache.lastAttempt = now
	ch := make(chan *HitokotoValue, 1)
	cache.inFlight = ch
	cache.mu.Unlock()

	go func() {
		val, err := fetchHitokoto(proxy, url)
		cache.mu.Lock()
		if val != nil && err == nil {
			cache.value = val
			cache.timestamp = time.Now().UnixMilli()
		} else if val == nil {
			// fetch failed, return cached value if available
			val = cache.value
		}
		cache.inFlight = nil
		cache.mu.Unlock()
		ch <- val
	}()

	return <-ch
}

// SendWelcomeExtras generates and sends the welcome message as a system chat.
// It runs asynchronously to avoid blocking the authentication flow.
func SendWelcomeExtras(user *game.User, st *state.ServerState, sendSystemChat func(string)) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				st.Logger.WarnL(st.ServerLang, "log-welcome-panic", map[string]string{"error": fmt.Sprintf("%v", r)})
			}
		}()

		lang := user.Lang
		tip := st.Config.RoomListTip
		hitokoto := getHitokotoCached(st.Config.OutboundProxy, st.Config.HitokotoAPIURL)

		// 30 newlines to clear screen
		var sb strings.Builder
		for i := 0; i < welcomeClearLines; i++ {
			sb.WriteByte('\n')
		}

		sb.WriteString(lang.Format("chat-welcome", map[string]string{
			"userName":   user.Name,
			"serverName": st.ServerName,
		}))
		sb.WriteByte('\n')
		sb.WriteString(strings.Repeat("=", welcomeSeparatorLen))
		sb.WriteByte('\n')
		sb.WriteString(lang.Format("chat-roomlist-title", nil))
		sb.WriteByte('\n')
		sb.WriteString(getAvailableRoomsText(st, lang))
		sb.WriteByte('\n')
		sb.WriteString(strings.Repeat("=", welcomeSeparatorLen))
		sb.WriteByte('\n')

		if tip != "" {
			sb.WriteString(tip)
			sb.WriteByte('\n')
		}

		if hitokoto != nil {
			fromText := hitokoto.From
			if fromText == "" {
				fromText = lang.Format("chat-hitokoto-from-unknown", nil)
			}
			sb.WriteString(hitokoto.Quote)
			sb.WriteString(" —— ")
			sb.WriteString(fromText)
		}

		sendSystemChat(sb.String())
	}()
}

// sendSystemChat sends a system chat message to the session.
func (s *Session) sendSystemChat(content string) {
	s.Send(protocol.ServerCommand{
		Type: protocol.ServerCmdMessage,
		Message: protocol.Message{
			Type:    protocol.MessageChat,
			User:    0,
			Content: content,
		},
	})
}
