# GPhira MP English language file
# Format: key=value

room-only-host=Only the host can do this
room-not-whitelisted=You are not whitelisted for this room
join-room-locked=Room is locked
join-game-ongoing=Game is ongoing
join-cant-monitor=Permission denied. You can't monitor this room.
start-no-chart-selected=No chart selected
room-invalid-state=Invalid room state
room-already-in-room=Already in a room
create-id-occupied=Room ID is occupied
room-not-found=Room not found
join-room-full=Room is full
user-banned-by-server=You have been banned from this server and cannot perform any operations.
room-banned=You are banned from room { $id }
room-creation-disabled=Room creation has been disabled by administrator
chart-fetch-failed=Failed to fetch chart
record-fetch-failed=Failed to fetch record
record-invalid=Invalid record
record-already-uploaded=Record already uploaded
room-game-aborted=Game aborted
auth-repeated-authenticate=Repeated authenticate
chat-welcome=Hello "{ $userName }"! Welcome to { $serverName }!
chat-hitokoto-from-unknown=Unknown
chat-roomlist-title=Available rooms:
chat-roomlist-empty=No available rooms
chat-roomlist-item={ $id } ({ $count }/{ $max })
chat-disabled-by-server=Chat is disabled on this server to avoid safety issues.
chat-game-summary=Match summary:\n{ $scoreText }\n{ $accText }\n{ $stdText }
chat-game-summary-score=Best score: "{ $name } "({ $id }) { $score }
chat-game-summary-acc=Best accuracy: "{ $name } "({ $id }) { $acc }
chat-game-summary-std=Best std: "{ $name } "({ $id }) { $std }ms
log-room-created="{ $user }" created room "{ $room }"
log-room-joined="{ $user }"{ $suffix } joined room "{ $room }"
log-room-left="{ $user }"{ $suffix } left room "{ $room }"
log-room-recycled=Room "{ $room }" recycled (empty)
log-room-game-start=Room "{ $room }" game start. users: { $users }{ $monitorsSuffix }
log-room-game-end=Room "{ $room }" game end (uploaded={ $uploaded }, aborted={ $aborted })
log-room-lock="{ $user }" { $lock } room "{ $room }"
log-room-cycle="{ $user }" { $cycle } room "{ $room }"
log-room-select-chart="{ $user }" (ID: { $userId }) selected "{ $chart }" in room "{ $room }"
log-room-request-start="{ $user }" requested start in room "{ $room }"
log-user-chat="{ $user }" sent chat in room "{ $room }"
log-room-lock-locked=locked
log-room-lock-unlocked=unlocked
log-room-cycle-on=enabled cycle host
log-room-cycle-off=disabled cycle host
log-server-starting=Starting GPhira MP server
log-server-listening=Server listening on { $addr }
log-server-info=Server name: { $name }, log level: { $level }
log-http-started=HTTP service started on { $addr }
log-redis-enabled=Redis cache enabled
log-shutting-down=Shutting down server
log-restarting-server=Restarting GPhira MP server
cli-welcome=Welcome to GPhira MP CLI. Type "help" for available commands.

# Missing existing keys
label-monitor-suffix= (monitor)
replay-recorder-name=Replay Recorder
lang-check=en
log-room-game-start-monitors=, monitors: { $monitors }
room-already-ready=You are already ready
room-not-ready=You are not ready
room-no-room=You are not in any room

# state.go
log-config-applied=Config applied: server_name={ $serverName }, lang={ $lang }, replay={ $replay }, room_creation={ $roomCreation }
log-admin-data-not-found=Admin data file not found: { $path }
log-admin-data-loaded=Admin data loaded: banned_users={ $bannedUsers }, banned_room_users={ $bannedRoomUsers }

# server.go
log-admin-data-load-failed=Failed to load admin data: { $err }
log-http-start-failed=Failed to start HTTP service: { $err }
log-config-reloaded=Config reloaded
log-accept-failed=Accept failed: { $err }
log-rate-limit-exceeded=Rate limit exceeded: { $remote }
log-connection-accepted=Connection accepted: { $remote }
log-proxy-protocol-failed=Proxy protocol parse failed: { $err }
log-proxy-protocol-ok=Proxy protocol ok: source={ $source }
log-new-connection=New connection: id={ $id }, remote={ $remote }
log-stream-error=Stream error: id={ $id }, phase={ $phase }, err={ $err }
log-handshake-failed=Handshake failed: id={ $id }, err={ $err }
log-handshake-ok=Handshake ok: id={ $id }
log-http-close-error=HTTP close error: { $err }

# session.go
log-auth-received=Auth received: session={ $session }, remote={ $remote }
log-command-before-auth=Command before auth: session={ $session }, cmd={ $cmd }
log-auth-api-failed=Auth API failed: session={ $session }, error={ $error }
log-auth-failed=Auth failed: session={ $session }, error={ $error }
log-user-reconnected=User reconnected: session={ $session }, user={ $user }, room={ $room }
log-user-authenticated=User authenticated: session={ $session }, user={ $user }, id={ $id }
log-auth-restored-room=Auth restored room: session={ $session }, user={ $user }, room={ $room }
log-heartbeat-timeout=Heartbeat timeout: session={ $session }, user={ $user }
log-stream-closed=Stream closed: session={ $session }, user={ $user }
log-session-marked-lost=Session marked lost: session={ $session }, user={ $user }, preserve_room={ $preserveRoom }
log-banned-user-disconnected=Banned user disconnected: session={ $session }, user={ $user }, name={ $name }
log-user-disconnected-playing=User disconnected while playing: session={ $session }, user={ $user }, name={ $name }, room={ $room }
log-user-dangling=User dangling: session={ $session }, user={ $user }, name={ $name }, room={ $room }
log-user-leave-remove=User leave and remove: session={ $session }, user={ $user }, name={ $name }
log-dangle-cleanup-skipped=Dangle cleanup skipped (reconnected): session={ $session }, user={ $user }, name={ $name }
log-dangle-cleanup-started=Dangle cleanup started: session={ $session }, user={ $user }, name={ $name }
log-dangle-cleanup-leaving=Dangle cleanup leaving room: session={ $session }, user={ $user }, name={ $name }, room={ $room }

# command_router.go
log-process-command=Process command: user={ $user }, name={ $name }, cmd={ $cmd }
log-repeated-authenticate=Repeated authenticate: user={ $user }, name={ $name }
log-chat=Chat: user={ $user }, name={ $name }, room={ $room }, content={ $content }
log-create-room=Create room: user={ $user }, name={ $name }, room={ $room }
log-join-room=Join room: user={ $user }, name={ $name }, room={ $room }, monitor={ $monitor }
log-leave-room=Leave room: user={ $user }, name={ $name }, room={ $room }
log-lock-room=Lock room: user={ $user }, name={ $name }, room={ $room }, lock={ $lock }
log-cycle-room=Cycle room: user={ $user }, name={ $name }, room={ $room }, cycle={ $cycle }
log-select-chart=Select chart: user={ $user }, name={ $name }, room={ $room }, chart_id={ $chartId }
log-request-start=Request start: user={ $user }, name={ $name }, room={ $room }
log-ready=Ready: user={ $user }, name={ $name }, room={ $room }
log-cancel-ready=Cancel ready: user={ $user }, name={ $name }, room={ $room }
log-played=Played: user={ $user }, name={ $name }, room={ $room }, record_id={ $recordId }
log-abort=Abort: user={ $user }, name={ $name }, room={ $room }
log-unknown-command-type=Unknown command type: { $type }

# websocket.go
log-ws-upgrade-failed=WebSocket upgrade failed: { $err }, remote={ $remote }
log-ws-connected=WebSocket connected: { $remote }
log-ws-client-registered=WebSocket client registered: clients={ $clients }
log-ws-client-leaving=WebSocket client leaving: room={ $room }
log-ws-broadcast=WebSocket broadcast: room={ $room }, type={ $type }, subs={ $subs }, sent={ $sent }
log-ws-unexpected-close=WebSocket unexpected close: { $err }
log-ws-subscribe=WebSocket subscribe: room={ $room }, user={ $user }
log-ws-admin-subscribe=WebSocket admin subscribe

# welcome.go
log-welcome-panic=Welcome message panic: { $error }

# room.go
log-room-all-ready=Room all ready: room={ $room }, users={ $users }
log-room-game-ended=Room game ended: room={ $room }, results={ $results }, aborted={ $aborted }
log-game-ended=Game ended in room { $room }
log-contest-game-ended=Contest game ended in room { $room }: chart={ $chart }, results={ $results }, aborted={ $aborted }
log-room-host-cycled=Room host cycled: room={ $room }, old_host={ $oldHost }, new_host={ $newHost }
log-host-changed=Host changed from { $oldHost } to { $newHost } in room { $room }

# cli.go
cli-unknown-command=Unknown command: { $cmd }. Type 'help' for available commands.
cli-stopping-server=Stopping server...
cli-restarting-server=Restarting server...
cli-restart-failed=Restart failed: { $err }
cli-restarted=Server restarted
cli-no-active-rooms=No active rooms.
cli-no-online-users=No online users.
cli-none=None
cli-yes=Yes
cli-no=No
cli-state-on=Enabled
cli-state-off=Disabled
cli-user-status-online=Online
cli-user-status-offline=Offline
cli-user-role-monitor=Monitor
cli-user-role-player=Player
cli-usage-user=Usage: user <user-id>
cli-user-info-header=User info:
cli-user-info-id=  ID: { $id }
cli-user-info-name=  Name: { $name }
cli-user-info-status=  Status: { $status }
cli-user-info-role=  Role: { $role }
cli-user-info-room=  Room: { $room }
cli-user-info-banned=  Banned: { $banned }
cli-user-info-game-time=  Game time: { $time }
cli-user-info-language=  Language: { $lang }
cli-usage-kick=Usage: kick <user-id>
cli-invalid-user-id=Invalid user ID
cli-kicked-user=Kicked user { $id } ({ $name })
cli-user-not-found=User not found or not online
cli-usage-ban=Usage: ban <user-id>
cli-banned-user=Banned user { $id }
cli-usage-unban=Usage: unban <user-id>
cli-unbanned-user=Unbanned user { $id }
cli-no-banned-users=No banned users.
cli-banned-users=Banned users:
cli-usage-banroom=Usage: banroom <user-id> <room-id>
cli-usage-unbanroom=Usage: unbanroom <user-id> <room-id>
cli-room-user-banned=Banned user { $userId } from room { $room }
cli-room-user-unbanned=Unbanned user { $userId } from room { $room }
cli-message-empty=Message cannot be empty
cli-message-too-long=Message too long (max { $max } characters)
cli-usage-broadcast=Usage: broadcast <message>
cli-broadcast-sent=Broadcast sent.
cli-usage-roomsay=Usage: roomsay <room-id> <message>
cli-room-message-sent=Message sent to room { $room }
cli-usage-maxusers=Usage: maxusers <room-id> <count>
cli-bad-max-users=Invalid count (1-64)
cli-room-max-users-set=Set room { $room } max users to { $count }
cli-usage-disband=Usage: disband <room-id>
cli-room-disbanded=Disbanded room { $room }
room-disbanded-by-admin=Room disbanded by admin
cli-usage-replay=Usage: replay <on|off|status>
cli-replay-status=Replay recording: { $state }
cli-replay-toggled-on=Replay recording enabled
cli-replay-toggled-off=Replay recording disabled
cli-usage-roomcreation=Usage: roomcreation <on|off|status>
cli-room-creation-status=Room creation: { $state }
cli-room-creation-toggled-on=Room creation enabled
cli-room-creation-toggled-off=Room creation disabled
cli-usage-ipblacklist=Usage: ipblacklist <list|remove|clear>
cli-usage-ipblacklist-remove=Usage: ipblacklist remove <ip>
cli-blacklist-empty=IP blacklist is empty
cli-blacklist-header=IP Blacklist ({ $count }):
cli-blacklist-line={ $ip } (expires in { $minutes } minutes)
cli-blacklist-removed=Removed from blacklist: { $ip }
cli-blacklist-cleared=Cleared IP blacklist
cli-ipblacklist-unknown-subcommand=Unknown subcommand. Available: list, remove, clear
cli-usage-approve=Usage: approve <ssid> (full ssid or prefix shortcode)
cli-usage-deny=Usage: deny <ssid> (full ssid or prefix shortcode)
cli-approve-not-found=No pending elevation request matched: { $input }
cli-approve-ambiguous=Shortcode { $input } matches multiple elevation requests; please use a longer prefix
cli-approve-expired=Elevation request { $ssid } has expired
cli-approve-already-handled=Elevation request { $ssid } is already in { $status } state and cannot be handled again
cli-approve-success=Approved elevation request { $ssid } (requester IP: { $ip }); temporary TOKEN issued
cli-deny-success=Denied elevation request { $ssid } (requester IP: { $ip })
cli-pending-empty=No pending CLI elevation requests
cli-pending-header=Pending CLI elevation requests ({ $count }):
cli-pending-line=[{ $ssid }] full ssid: { $full } | IP: { $ip } | remaining { $seconds }s
cli-usage-contest=Usage: contest <room-id> <enable|disable|whitelist|start> [args...]
cli-invalid-room-id=Invalid room ID
cli-unknown-contest-subcommand=Unknown contest subcommand: { $cmd }
cli-contest-enabled=Contest mode enabled for room { $room }
cli-room-not-found=Room not found
cli-contest-disabled=Contest mode disabled for room { $room }
cli-usage-contest-whitelist=Usage: contest <room> whitelist <user-id>...
cli-contest-whitelist-updated=Contest whitelist updated for room { $room }
cli-room-not-found-or-contest-disabled=Room not found or contest not enabled
cli-cannot-start-contest=Cannot start contest: { $reason }
cli-contest-started=Contest game started in room { $room }
cli-header-room-id=Room ID
cli-header-host=Host
cli-header-users=Users
cli-header-contest=Contest
cli-header-state=State
cli-header-id=ID
cli-header-name=Name
cli-header-room=Room
cli-header-monitor=Monitor

cli-help=Available commands:
  help, h                  Show this help message
  list, rooms              List all rooms
  users                    List all online users
  user <id>                Show user details
  kick <id>                Kick a user by ID
  ban <id>                 Ban a user by ID
  unban <id>               Unban a user by ID
  banlist                  List banned users
  banroom <id> <room>      Ban a user from a room
  unbanroom <id> <room>    Remove a room ban
  broadcast <msg>          Broadcast a message to all users
  roomsay <room> <msg>     Send a message to one room
  maxusers <room> <count>  Set room max users
  disband <room>           Disband a room
  replay <on|off|status>   Toggle replay recording
  roomcreation <...>       Toggle room creation
  ipblacklist <...>        Manage IP blacklist
  approve <ssid>           Approve a CLI admin elevation request
  deny <ssid>              Deny a CLI admin elevation request
  pending                  List pending CLI elevation requests
  contest <room> enable    Enable contest mode for a room
  contest <room> disable   Disable contest mode for a room
  contest <room> whitelist <id>...
                           Update contest whitelist
  contest <room> start [force]
                           Manually start a contest game
  restart, r               Restart the server
  stop, exit, quit         Stop the server
