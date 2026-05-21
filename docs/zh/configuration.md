# 配置参考

配置可以通过以下方式提供：

1. **命令行参数**（最高优先级）
2. **环境变量**
3. **配置文件**（默认 `config.yaml`）
4. **内置默认值**（最低优先级）

---

## 配置文件

默认路径：`config.yaml`

YAML 文件中的所有键均为**大写**：

```yaml
HOST: "::"
PORT: 12346
HTTP_SERVICE: false
HTTP_PORT: 12347
LOG_LEVEL: INFO
LANG: zh-CN
ROOM_MAX_USERS: 8
CHAT_ENABLED: true
REPLAY_ENABLED: false
ADMIN_TOKEN: "your-secure-token"
```

---

## 命令行参数

```bash
./gphira-mp \
  --config config.yaml \
  --host 0.0.0.0 \
  --port 12346 \
  --httpService true \
  --httpPort 12347 \
  --roomMaxUsers 12 \
  --serverName "My Server" \
  --monitors "2,100"
```

| 参数 | 短参数 | 描述 |
|------|--------|------|
| `--config` | — | 配置文件路径 |
| `--host` | — | 监听主机地址 |
| `--port` | `-p` | 监听端口（1-65535） |
| `--httpService` | — | 启用 HTTP 服务（`true`/`false`） |
| `--httpPort` | — | HTTP 服务端口（1-65535） |
| `--roomMaxUsers` | — | 每个房间的最大用户数（1-64） |
| `--serverName` | — | 服务器显示名称 |
| `--monitors` | — | 监视用户 ID（逗号分隔） |

---

## 环境变量

每个配置选项都有对应的同名环境变量：

| 变量 | 类型 | 默认值 |
|------|------|--------|
| `HOST` | string | `::` |
| `PORT` | int | `12346` |
| `HTTP_SERVICE` | bool | `false` |
| `HTTP_PORT` | int | `12347` |
| `ROOM_MAX_USERS` | int | `8` |
| `ROOM_CREATION_ENABLED` | bool | `true` |
| `CHAT_ENABLED` | bool | `true` |
| `REPLAY_ENABLED` | bool | `false` |
| `REPLAY_BASE_DIR` | string | `./record` |
| `REPLAY_AUTO_UPLOAD` | bool | `false` |
| `MONITORS` | int[] | `[2]` |
| `TEST_ACCOUNT_IDS` | int[] | `[1739989]` |
| `SERVER_NAME` | string | `Phira MP` |
| `ADMIN_TOKEN` | string | *(空)* |
| `ADMIN_DATA_PATH` | string | `./admin_data.json` |
| `ROOM_LIST_TIP` | string | *(空)* |
| `LOG_LEVEL` | string | `INFO` |
| `REAL_IP_HEADER` | string | `X-Forwarded-For` |
| `HAPROXY_PROTOCOL` | bool | `false` |
| `LANG` | string | `zh-CN` |
| `PHIRA_API_ENDPOINT` | string | `https://phira.5wyxi.com` |
| `OUTBOUND_PROXY` | string \| false | *(空)* |
| `HITOKOTO_API_URL` | string | `https://v1.hitokoto.cn/` |
| `SHARE_STATION_URL` | string | *(空)* |
| `SHARE_STATION_TOKEN` | string | *(空)* |
| `REDIS_ENABLED` | bool | `false` |
| `REDIS_HOST` | string | `127.0.0.1` |
| `REDIS_PORT` | int | `6379` |
| `REDIS_PASSWORD` | string | *(空)* |
| `REDIS_DB` | int | `0` |

### 语言与本地化

`LANG` 设置控制服务器端日志、CLI 输出和系统消息的语言。

**支持的值：** `zh-CN`、`en-US`

**工作原理：**
1. 启动时，服务器在工作目录相对的 `l10n/{LANG}.ftl` 处查找外部语言文件。
2. 如果文件存在，则从中加载翻译 — 您可以直接编辑这些文件，无需重新编译。
3. 如果文件缺失，服务器回退到嵌入二进制文件中的内置默认值。

**自定义翻译：**
```bash
# 编辑或添加翻译键
cat l10n/zh-CN.ftl
# log-server-starting=正在启动 GPhira MP 服务端
# cli-welcome=欢迎使用 GPhira MP CLI
```

要添加新语言，请创建 `l10n/xx-XX.ftl` 并在配置中设置 `LANG: xx-XX`。

### 出站代理

`OUTBOUND_PROXY` 设置接受：

- **空 / 未设置**：使用系统默认值（可能遵循 `HTTP_PROXY` 环境变量）。
- **`false`**：强制直接连接，绕过任何代理。
- **`http://host:port`**：HTTP 代理。
- **`https://host:port`**：HTTPS 代理。
- **`socks://host:port`**：SOCKS5 代理。
- **`socks4://host:port`**：SOCKS4 代理。
- **`socks5://host:port`**：SOCKS5 代理。

支持通过 URL 进行认证：`http://user:pass@host:port`。

---

## 配置示例

### 开发环境

```yaml
HOST: "127.0.0.1"
PORT: 12346
HTTP_SERVICE: true
HTTP_PORT: 12347
LOG_LEVEL: DEBUG
REPLAY_ENABLED: true
ADMIN_TOKEN: "dev-token"
```

### 生产环境

```yaml
HOST: "0.0.0.0"
PORT: 12346
HTTP_SERVICE: true
HTTP_PORT: 12347
LOG_LEVEL: INFO
REAL_IP_HEADER: "X-Forwarded-For"
HAPROXY_PROTOCOL: true
REDIS_ENABLED: true
REDIS_HOST: "redis.internal"
```

### Docker

在 Docker 中运行时，使用环境变量或挂载 `config.yaml`：

```bash
docker run -e PORT=12346 -e HTTP_SERVICE=true -e ADMIN_TOKEN=secret phira-mp
```
