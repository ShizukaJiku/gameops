// Package gwconfig loads the config for `gameops gateway` — a separate
// shape from internal/config.Config, since the gateway has no [[instances]]
// of its own, only a list of hosts to proxy to.
package gwconfig

import "github.com/BurntSushi/toml"

type GatewayConfig struct {
	ListenAddr        string      `toml:"listen_addr"`
	Domain            string      `toml:"domain"`
	AdminPasswordHash string      `toml:"admin_password_hash"`
	SessionSecret     string      `toml:"session_secret"`
	Hosts             []HostEntry `toml:"hosts"`
}

// HostEntry is one machine gameops gateway proxies to. Addr is reached over
// the existing frp tunnel (e.g. "127.0.0.1:8090" once frps forwards a proxy
// for that host's gameops host API port) — never a public address. Token
// must match the token field in that host's own [host] config section.
type HostEntry struct {
	Name  string `toml:"name"`
	Addr  string `toml:"addr"`
	Token string `toml:"token"`
}

func Load(path string) (*GatewayConfig, error) {
	var cfg GatewayConfig
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}
