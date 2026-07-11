# gameops

Single Go binary with subcommands for operating self-hosted game servers — auto stop/start on inactivity, backups, world regeneration, and startup automation. Built to replace a pile of ad-hoc PowerShell scripts with one ordered, testable tool that anyone can point at their own server.

## Subcommands

| Subcommand | Status |
|---|---|
| `idle-watch` | Implemented — proxies a game server, waking it on a real login attempt and stopping it after a configurable idle period. |
| `backup` | Roadmap |
| `maintenance` | Roadmap |
| `startup apply` | Roadmap |
| `world regen` | Roadmap |

## idle-watch

Sits in front of a backend game server (currently: Minecraft/Forge). While the backend is asleep it speaks just enough of the game's own protocol to answer server-list pings and login attempts with a "starting up" message, without needing the backend running. A real login attempt starts the backend; once it's reachable, idle-watch proxies raw bytes through. After a configurable number of minutes with no players online (checked via RCON), it stops the backend and goes back to sleep.

### Config

```toml
[[instances]]
name = "minecraft"
game = "minecraft"
listen_port = 25565
backend_port = 25566
idle_timeout_minutes = 15
poll_interval_seconds = 30
backend_ready_timeout_minutes = 5
start_command = "schtasks /run /tn mc-forge"

[instances.minecraft_config]
rcon_port = 25575
rcon_password = "secret"
forge_properties_path = "C:\\mc-forge\\server.properties"
motd_fallback = "Server asleep - connect to wake it up (~1-2 min)"
```

### Game defaults

Instances of the same game can share defaults instead of repeating every
field, via `[games.<name>]`:

```toml
[games.minecraft]
idle_timeout_minutes = 15
poll_interval_seconds = 30
start_command = "schtasks /run /tn mc-forge"

[games.minecraft.minecraft_config]
rcon_port = 25575
motd_fallback = "Server asleep - connect to wake it up (~1-2 min)"

[games.minecraft.backup_config]
max_backups = 10

[games.minecraft.maintenance_config]
process_name = "java"
stop_command = "stop"

[[instances]]
name = "servermc1"
game = "minecraft"
listen_port = 25565
backend_port = 25566

[instances.minecraft_config]
rcon_password = "secret"                                    # per-instance, not shared
forge_properties_path = "C:\\mc-forge\\server.properties"   # per-instance, not shared
```

Each field on an instance wins if set; otherwise it falls back to its
game's default, then to a built-in default. `rcon_password` *can* be set
under `[games.<name>]` and inherited, but sharing one password across
instances means a leak on one compromises all of them — prefer setting it
per-instance.

Run: `gameops.exe idle-watch -config gameops.toml`

## License

MIT
