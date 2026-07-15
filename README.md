# vibe-apply

Do not apply jobs manually, vibe it!

A small Go CLI bot that watches **one** Discord channel. When a human posts a
message there, the bot **opens a thread** on that message and greets them with
`Hi, my name is Vibe Apply`. When a human posts inside any thread under that
channel, it greets them again. It ignores bots and itself, so it never loops.

It talks to Discord over the **Gateway** (a real-time WebSocket) — messages are
pushed to the bot the instant they're posted, so there's no polling.

---

## 1. Prerequisites

- **Go 1.24+** — check with `go version`. Install from <https://go.dev/dl/> if needed.
- A **Discord account** and a **server you can manage** (you need permission to
  add a bot to it).

---

## 2. Create the Discord bot (one time)

### 2.1 Create an application
1. Go to the **Discord Developer Portal**: <https://discord.com/developers/applications>
2. Click **New Application**, give it a name (e.g. `vibe-apply`), accept the
   terms, and **Create**.

### 2.2 Add a bot and get the token
1. In your application, open the **Bot** tab (left sidebar).
2. Click **Reset Token** → **Yes, do it!** → **Copy**.
   This string is your `DISCORD_TOKEN`. **Treat it like a password** — it is
   shown only once, and anyone with it can control your bot.
   > It is *not* the "Application ID" and *not* the "Public Key" — those are
   > different values and won't work here.

### 2.3 Enable the Message Content Intent (required)
Still on the **Bot** tab, scroll to **Privileged Gateway Intents** and turn on
**MESSAGE CONTENT INTENT**, then **Save Changes**. Without this, the bot
connects but sees every message as empty and can't work. (Leave "Server Members
Intent" off — it isn't needed.)

### 2.4 Invite the bot to your server
1. Open the **OAuth2 → URL Generator** tab.
2. Under **Scopes**, tick **`bot`**.
3. Under **Bot Permissions**, tick at least:
   - **View Channel**
   - **Send Messages**
   - **Create Public Threads**
   - **Send Messages in Threads**
4. Copy the generated URL at the bottom, open it in your browser, pick your
   server, and **Authorize**.

You should now see the bot listed in your server's member list.

### 2.5 Get the channel ID
1. In Discord, enable Developer Mode: **User Settings → Advanced → Developer
   Mode** (toggle on).
2. Right-click the channel you want the bot to watch → **Copy Channel ID**.
   This is your `DISCORD_CHANNEL_ID`.
   > Tip: a channel URL looks like
   > `https://discord.com/channels/<server-id>/<channel-id>` — the **second**
   > number is the channel ID.

---

## 3. Environment variables

The bot reads two values. Put them in a local `.env` file (loaded automatically
on startup) or export them in your shell.

| Variable             | What it is                     | Where to get it                          |
| -------------------- | ------------------------------ | ---------------------------------------- |
| `DISCORD_TOKEN`      | Secret bot token (a password)  | Developer Portal → **Bot** → Reset Token (step 2.2) |
| `DISCORD_CHANNEL_ID` | ID of the channel to watch     | Right-click the channel → Copy Channel ID (step 2.5) |

Copy the template and fill it in:

```sh
cp .env.example .env
# then edit .env and paste your two values
```

`.env` is listed in `.gitignore`, so your token is **never committed**.

---

## 4. Run it locally

```sh
go run .
```

You should see:

```
Vibe Apply is listening on channel <your-channel-id>. Press Ctrl+C to stop.
```

Keep this terminal open — the bot only responds while the process is running.
Post a message in the watched channel from your own account: the bot opens a
`Vibe Apply` thread and greets you. Reply inside that thread and it greets you
again. Press **Ctrl+C** to stop.

You can also pass the values as flags instead of using `.env`:

```sh
go run . -token "your-bot-token" -channel "your-channel-id"
```

To build a standalone binary:

```sh
go build -o vibe-apply .
./vibe-apply
```

---

## 5. Test

```sh
go test ./...      # run unit tests
go vet ./...       # static analysis
go build ./...     # compile check
```

The unit tests cover the decision logic only and need **no** Discord token, so
they run fully offline.

---

## 6. Continuous integration

Every push to `main` and every pull request runs
[`.github/workflows/ci.yml`](.github/workflows/ci.yml), which executes
`go build`, `go vet`, and `go test` on the Go version pinned in `go.mod`.

---

## 7. How it behaves

| Someone posts…                                   | The bot…                                   |
| ------------------------------------------------ | ------------------------------------------ |
| a message in the watched channel (human)         | opens a `Vibe Apply` thread and greets     |
| a message inside a thread under that channel     | greets again in that thread                |
| a message from a bot, or the bot's own greeting  | ignores it (no loops)                      |
| a message in any other channel                   | ignores it                                 |

> Note: the bot greets on **every** human message in the channel, so a busy
> channel will grow a lot of threads. That's by design — open an issue if you'd
> like a cooldown or one-thread-per-user option.
