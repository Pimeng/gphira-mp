# GPhira MP 简体中文语言文件
# 格式: Project Fluent (FTL)

room-only-host = 只有房主可以执行此操作
room-not-whitelisted = 你不在该房间白名单中
join-room-locked = 房间已锁定
join-game-ongoing = 游戏正在进行中
join-cant-monitor = 权限不足，不能旁观房间
start-no-chart-selected = 还没有选择谱面
room-invalid-state = 房间状态不允许此操作
room-already-in-room = 已在房间中
create-id-occupied = 房间 ID 已被占用
room-not-found = 房间不存在
join-room-full = 房间已满
user-banned-by-server = 你已被服务器封禁，无法进行任何操作。
room-banned = 你已被禁止进入房间 { $id }
room-creation-disabled = 房间创建功能已被管理员禁用
chart-fetch-failed = 获取谱面失败
record-fetch-failed = 获取记录失败
record-invalid = 记录不合法
record-already-uploaded = 已上传记录
room-game-aborted = 对局已中止
auth-repeated-authenticate = 重复认证
chat-welcome = "{ $userName }"你好！欢迎来到 { $serverName } 服务器！
chat-hitokoto-from-unknown = 佚名
chat-roomlist-title = 当前可用的房间如下：
chat-roomlist-empty = 当前没有可用房间
chat-roomlist-item = { $id }（{ $count }/{ $max }）
chat-disabled-by-server = 为避免安全问题，该服务器已禁用聊天
chat-game-summary = 
    本局结算：
    { $scoreText }
    { $accText }
    { $stdText }
chat-game-summary-score = 最高分："{ $name } "({ $id }) { $score }
chat-game-summary-acc = 最高准度："{ $name } "({ $id }) { $acc }
chat-game-summary-std = 最佳无瑕度："{ $name } "({ $id }) { $std }ms
log-room-created = "{ $user }" 创建房间 "{ $room }"
log-room-joined = "{ $user }"{ $suffix } 加入房间 "{ $room }"
log-room-left = "{ $user }"{ $suffix } 离开房间 "{ $room }"
log-room-recycled = 房间 "{ $room }" 已回收（无玩家）
log-room-game-start = 房间 "{ $room }" 对局开始，玩家：{ $users }{ $monitorsSuffix }
log-room-game-end = 房间 "{ $room }" 对局结束（已上传：{ $uploaded }，中止：{ $aborted }）
log-room-lock = "{ $user }" 将房间 "{ $room }" { $lock }
log-room-cycle = "{ $user }" 将房间 "{ $room }" { $cycle }
log-room-select-chart = "{ $user }"（用户ID：{ $userId }）在房间 "{ $room }" 选择了 "{ $chart }"
log-room-request-start = "{ $user }" 在房间 "{ $room }" 请求开始对局
log-user-chat = "{ $user }" 在房间 "{ $room }" 发送聊天消息
log-msg-create-room = { $user } 创建了房间
log-msg-join-room = { $name } 加入了房间
log-msg-leave-room = { $name } 离开了房间
log-msg-new-host = { $user } 成为了新的房主
log-msg-select-chart = 房主 { $user } 选择了谱面 { $name } (#{ $id })
log-msg-game-start = 房主 { $user } 开始了游戏，请其他玩家准备
log-msg-ready = { $user } 已就绪
log-msg-cancel-ready = { $user } 取消了准备
log-msg-cancel-game = { $user } 取消了对局
log-msg-start-playing = 游戏开始
log-msg-played = { $user } 结束了游玩：{ $score } ({ $acc }%){ $fc }
log-msg-game-end = 游戏结束
log-msg-abort = { $user } 放弃了游戏
log-msg-lock-room = { $status }
log-msg-cycle-room = { $status }
log-room-lock-locked = 设为锁定
log-room-lock-unlocked = 取消锁定
log-room-cycle-on = 开启轮转房主
log-room-cycle-off = 关闭轮转房主
log-server-starting = 正在启动 GPhira MP 服务端
log-server-listening = 服务端正在偷听 { $addr }
log-server-info = 服务端昵称：{ $name } ，日志等级{ $level }
log-http-started = HTTP 服务已启动 { $addr }
log-redis-enabled = Redis 缓存已启用
log-shutting-down = 正在关闭服务端
log-restarting-server = 正在重启 GPhira MP 服务端
cli-welcome = 欢迎使用 GPhira MP CLI. 输入 "help" 获取命令列表

# 缺失的已有 key
label-monitor-suffix = （旁观）
replay-recorder-name = 回放录制器
lang-check = zh
log-room-game-start-monitors = ，旁观：{ $monitors }
room-already-ready = 你已经准备就绪
room-not-ready = 你尚未准备就绪
room-no-room = 你不在任何房间中

# state.go
log-config-applied = 配置已应用：服务器名称={ $serverName }，语言={ $lang }，回放={ $replay }，房间创建={ $roomCreation }
log-admin-data-not-found = admin 数据文件未找到：{ $path }
log-admin-data-loaded = admin 数据已加载：封禁用户={ $bannedUsers }，房间封禁={ $bannedRoomUsers }

# server.go
log-admin-data-load-failed = 加载 admin 数据失败：{ $err }
log-http-start-failed = 启动 HTTP 服务失败：{ $err }
log-config-reloaded = 配置已重载
log-accept-failed = 接受连接失败：{ $err }
log-rate-limit-exceeded = 连接速率限制：{ $remote }
log-connection-accepted = 连接已接受：{ $remote }
log-proxy-protocol-failed = 代理协议解析失败：{ $err }
log-proxy-protocol-ok = 代理协议成功：源地址={ $source }
log-new-connection = 新连接：ID={ $id }，远程地址={ $remote }
log-stream-error = 流错误：ID={ $id }，阶段={ $phase }，错误={ $err }
log-handshake-failed = 握手失败：ID={ $id }，错误={ $err }
log-handshake-ok = 握手成功：ID={ $id }
log-http-close-error = 关闭 HTTP 服务错误：{ $err }

# session.go
log-auth-received = 收到认证：会话={ $session }，远程地址={ $remote }
log-command-before-auth = 认证前收到命令：会话={ $session }，命令={ $cmd }
log-auth-api-failed = 认证 API 失败：会话={ $session }，错误={ $error }
log-auth-failed = 认证失败：会话={ $session }，错误={ $error }
log-user-reconnected = 用户重连：会话={ $session }，用户={ $user }，房间={ $room }
log-user-authenticated = 用户认证成功：会话={ $session }，用户={ $user }，ID={ $id }
log-auth-restored-room = 认证恢复房间：会话={ $session }，用户={ $user }，房间={ $room }
log-heartbeat-timeout = 心跳超时：会话={ $session }，用户={ $user }
log-stream-closed = 流已关闭：会话={ $session }，用户={ $user }
log-session-marked-lost = 会话标记为丢失：会话={ $session }，用户={ $user }，保留房间={ $preserveRoom }
log-banned-user-disconnected = 封禁用户断开连接：会话={ $session }，用户={ $user }，名称={ $name }
log-user-disconnected-playing = 用户游戏中断开：会话={ $session }，用户={ $user }，名称={ $name }，房间={ $room }
log-user-dangling = 用户挂起：会话={ $session }，用户={ $user }，名称={ $name }，房间={ $room }
log-user-leave-remove = 用户离开并移除：会话={ $session }，用户={ $user }，名称={ $name }
log-dangle-cleanup-skipped = 挂起清理跳过（已重连）：会话={ $session }，用户={ $user }，名称={ $name }
log-dangle-cleanup-started = 挂起清理开始：会话={ $session }，用户={ $user }，名称={ $name }
log-dangle-cleanup-leaving = 挂起清理离开房间：会话={ $session }，用户={ $user }，名称={ $name }，房间={ $room }

# command_router.go
log-process-command = 处理命令：用户={ $user }，名称={ $name }，命令={ $cmd }
log-repeated-authenticate = 重复认证：用户={ $user }，名称={ $name }
log-chat = 聊天：用户={ $user }，名称={ $name }，房间={ $room }，内容={ $content }
log-create-room = 创建房间：用户={ $user }，名称={ $name }，房间={ $room }
log-join-room = 加入房间：用户={ $user }，名称={ $name }，房间={ $room }，旁观={ $monitor }
log-leave-room = 离开房间：用户={ $user }，名称={ $name }，房间={ $room }
log-lock-room = 锁定房间：用户={ $user }，名称={ $name }，房间={ $room }，锁定={ $lock }
log-cycle-room = 轮转房间：用户={ $user }，名称={ $name }，房间={ $room }，轮转={ $cycle }
log-select-chart = 选择谱面：用户={ $user }，名称={ $name }，房间={ $room }，谱面ID={ $chartId }
log-request-start = 请求开始：用户={ $user }，名称={ $name }，房间={ $room }
log-ready = 准备就绪：用户={ $user }，名称={ $name }，房间={ $room }
log-cancel-ready = 取消准备：用户={ $user }，名称={ $name }，房间={ $room }
log-played = 上传成绩：用户={ $user }，名称={ $name }，房间={ $room }，记录ID={ $recordId }
log-abort = 中止对局：用户={ $user }，名称={ $name }，房间={ $room }
log-unknown-command-type = 未知命令类型：{ $type }

# websocket.go
log-ws-upgrade-failed = WebSocket 升级失败：{ $err }，远程地址={ $remote }
log-ws-connected = WebSocket 已连接：{ $remote }
log-ws-client-registered = WebSocket 客户端注册：客户端数={ $clients }
log-ws-client-leaving = WebSocket 客户端离开：房间={ $room }
log-ws-broadcast = WebSocket 广播：房间={ $room }，类型={ $type }，订阅数={ $subs }，发送数={ $sent }
log-ws-unexpected-close = WebSocket 异常关闭：{ $err }
log-ws-subscribe = WebSocket 订阅：房间={ $room }，用户={ $user }
log-ws-admin-subscribe = WebSocket 管理员订阅

# welcome.go
log-welcome-panic = 欢迎消息 panic：{ $error }

# room.go
log-room-all-ready = 房间全部准备就绪：房间={ $room }，用户数={ $users }
log-room-game-ended = 房间对局结束：房间={ $room }，成绩数={ $results }，中止数={ $aborted }
log-game-ended = 房间 { $room } 对局结束
log-contest-game-ended = 比赛房间 { $room } 对局结束：谱面={ $chart }，成绩={ $results }，中止={ $aborted }
log-room-host-cycled = 房间房主轮转：房间={ $room }，旧房主={ $oldHost }，新房主={ $newHost }
log-host-changed = 房间 { $room } 房主从 { $oldHost } 变更为 { $newHost }

# cli.go
cli-unknown-command = 未知命令：{ $cmd }。输入 "help" 获取可用命令列表。
cli-stopping-server = 正在停止服务端...
cli-restarting-server = 正在重启服务端...
cli-restart-failed = 重启失败：{ $err }
cli-restarted = 服务端已重启
cli-no-active-rooms = 没有活跃房间。
cli-no-online-users = 没有在线用户。
cli-none = 无
cli-yes = 是
cli-no = 否
cli-state-on = 开启
cli-state-off = 关闭
cli-user-status-online = 在线
cli-user-status-offline = 离线
cli-user-role-monitor = 观战
cli-user-role-player = 玩家
cli-usage-user = 用法：user <用户ID>
cli-user-info-header = 用户信息：
cli-user-info-id = ID：{ $id }
cli-user-info-name = 名称：{ $name }
cli-user-info-status = 状态：{ $status }
cli-user-info-role = 角色：{ $role }
cli-user-info-room = 房间：{ $room }
cli-user-info-banned = 封禁：{ $banned }
cli-user-info-game-time = 游戏时间：{ $time }
cli-user-info-language = 语言：{ $lang }
cli-usage-kick = 用法：kick <用户ID>
cli-invalid-user-id = 无效的用户ID
cli-kicked-user = 已踢出用户 { $id }（{ $name }）
cli-user-not-found = 用户未找到或不在线
cli-usage-ban = 用法：ban <用户ID>
cli-banned-user = 已封禁用户 { $id }
cli-usage-unban = 用法：unban <用户ID>
cli-unbanned-user = 已解封用户 { $id }
cli-no-banned-users = 没有封禁用户。
cli-banned-users = 封禁用户列表：
cli-usage-banroom = 用法：banroom <用户ID> <房间ID>
cli-usage-unbanroom = 用法：unbanroom <用户ID> <房间ID>
cli-room-user-banned = 已禁止用户 { $userId } 进入房间 { $room }
cli-room-user-unbanned = 已解除用户 { $userId } 对房间 { $room } 的禁入
cli-message-empty = 消息不能为空
cli-message-too-long = 消息过长（最多 { $max } 字符）
cli-usage-broadcast = 用法：broadcast <消息>
cli-broadcast-sent = 广播已发送。
cli-usage-roomsay = 用法：roomsay <房间ID> <消息>
cli-room-message-sent = 已向房间 { $room } 发送消息
cli-usage-maxusers = 用法：maxusers <房间ID> <人数>
cli-bad-max-users = 无效的人数（1-64）
cli-room-max-users-set = 已设置房间 { $room } 最大人数为 { $count }
cli-usage-disband = 用法：disband <房间ID>
cli-room-disbanded = 已解散房间 { $room }
room-disbanded-by-admin = 房间已被管理员解散
cli-usage-replay = 用法：replay <on|off|status>
cli-replay-status = 回放录制状态：{ $state }
cli-replay-toggled-on = 回放录制已开启
cli-replay-toggled-off = 回放录制已关闭
cli-usage-roomcreation = 用法：roomcreation <on|off|status>
cli-room-creation-status = 房间创建状态：{ $state }
cli-room-creation-toggled-on = 房间创建已开启
cli-room-creation-toggled-off = 房间创建已关闭
cli-usage-ipblacklist = 用法：ipblacklist <list|remove|clear>
cli-usage-ipblacklist-remove = 用法：ipblacklist remove <IP>
cli-blacklist-empty = IP 黑名单为空
cli-blacklist-header = IP 黑名单（共 { $count } 个）：
cli-blacklist-line = { $ip }（{ $minutes } 分钟后过期）
cli-blacklist-removed = 已从黑名单移除：{ $ip }
cli-blacklist-cleared = 已清空 IP 黑名单
cli-ipblacklist-unknown-subcommand = 未知子命令。可用：list、remove、clear
cli-usage-approve = 用法：approve <ssid>（支持完整 ssid 或前缀短码）
cli-usage-deny = 用法：deny <ssid>（支持完整 ssid 或前缀短码）
cli-approve-not-found = 未找到匹配的提权申请：{ $input }
cli-approve-ambiguous = 短码 { $input } 匹配到多个提权申请，请提供更长的前缀
cli-approve-expired = 提权申请 { $ssid } 已过期
cli-approve-already-handled = 提权申请 { $ssid } 已处于 { $status } 状态，无法再次处理
cli-approve-success = 已批准提权申请 { $ssid }（请求IP：{ $ip }），临时 TOKEN 已签发
cli-deny-success = 已拒绝提权申请 { $ssid }（请求IP：{ $ip }）
cli-pending-empty = 当前没有待处理的 CLI 提权申请
cli-pending-header = 待处理的 CLI 提权申请（共 { $count } 个）：
cli-pending-line = [{ $ssid }] 完整 ssid: { $full } | 请求IP: { $ip } | 剩余 { $seconds } 秒
cli-usage-contest = 用法：contest <房间ID> <enable|disable|whitelist|start> [参数...]
cli-invalid-room-id = 无效的房间ID
cli-unknown-contest-subcommand = 未知比赛子命令：{ $cmd }
cli-contest-enabled = 房间 { $room } 比赛模式已启用
cli-room-not-found = 房间未找到
cli-contest-disabled = 房间 { $room } 比赛模式已禁用
cli-usage-contest-whitelist = 用法：contest <房间> whitelist <用户ID>...
cli-contest-whitelist-updated = 房间 { $room } 比赛白名单已更新
cli-room-not-found-or-contest-disabled = 房间未找到或比赛模式未启用
cli-cannot-start-contest = 无法开始比赛：{ $reason }
cli-contest-started = 房间 { $room } 比赛已开始
cli-header-room-id = 房间ID
cli-header-host = 房主
cli-header-users = 用户
cli-header-contest = 比赛
cli-header-state = 状态
cli-room-state-select = 选谱
cli-room-state-waiting = 等待准备
cli-room-state-playing = 游戏中
cli-header-id = ID
cli-header-name = 名称
cli-header-room = 房间
cli-header-monitor = 旁观

cli-help = 可用命令：
  help, h                  显示此帮助信息
  list, rooms              列出所有房间
  users                    列出所有在线用户
  user <id>                查看用户详情
  kick <id>                按ID踢出用户
  ban <id>                 按ID封禁用户
  unban <id>               按ID解封用户
  banlist                  列出封禁用户
  banroom <id> <room>      禁止用户进入指定房间
  unbanroom <id> <room>    解除用户的房间禁入
  broadcast <msg>          向所有用户广播消息
  roomsay <room> <msg>     向指定房间发送消息
  maxusers <room> <count>  设置房间最大人数
  disband <room>           解散房间
  replay <on|off|status>   切换回放录制
  roomcreation <...>       切换房间创建
  ipblacklist <...>        管理 IP 黑名单
  approve <ssid>           批准 CLI 管理员提权申请
  deny <ssid>              拒绝 CLI 管理员提权申请
  pending                  列出待处理的 CLI 提权申请
  contest <room> enable    为房间启用比赛模式
  contest <room> disable   为房间禁用比赛模式
  contest <room> whitelist <id>...
                           更新比赛白名单
  contest <room> start [force]
                           手动开始比赛
  restart, r               重启服务端
  stop, exit, quit         停止服务端
