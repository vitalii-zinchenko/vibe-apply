# vibe-apply
Do not apply jobs manually, vibe it!

A small Go CLI bot that watches one Discord channel. When a human posts there it
**opens a thread** on that message and greets them with `Hi, my name is Vibe
Apply`; when a human posts inside any thread under that channel, it greets them
again. It ignores bots and itself (so it never loops). Uses the Discord Gateway
(real-time WebSocket) — no polling.

## One-time Discord setup

1. In the [Discord Developer Portal](https://discord.com/developers/applications),
   create an application, add a **Bot**, and copy the **bot token**.
2. In the bot settings, enable the **Message Content Intent** (privileged).
   Without it, message text arrives empty and the bot can't work.
3. Invite the bot to your server with **View Channel** + **Send Messages**
   permissions.
4. Enable Developer Mode (User Settings → Advanced), right-click the target
   channel → **Copy Channel ID**.

## Run

Copy `.env.example` to `.env` and fill in your bot token (the channel ID is
already set). The app loads `.env` automatically on startup.

```sh
cp .env.example .env   # then edit .env and paste your DISCORD_TOKEN
go run .
```

`.env` is gitignored, so your token is never committed. You can also skip `.env`
and use real env vars or flags instead:

```sh
go run . -token "your-bot-token" -channel "target-channel-id"
```

Keep the terminal session open while it runs. Press **Ctrl+C** to stop.

## Test

```sh
go test ./...
```
