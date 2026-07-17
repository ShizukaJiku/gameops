package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"

	"golang.org/x/crypto/acme/autocert"

	"github.com/ShizukaJiku/gameops/backup"
	"github.com/ShizukaJiku/gameops/gateway"
	"github.com/ShizukaJiku/gameops/host"
	"github.com/ShizukaJiku/gameops/idlewatch"
	"github.com/ShizukaJiku/gameops/internal/config"
	"github.com/ShizukaJiku/gameops/internal/gwconfig"
	"github.com/ShizukaJiku/gameops/internal/webauth"
	"github.com/ShizukaJiku/gameops/maintenance"
	"github.com/ShizukaJiku/gameops/startup"
	"github.com/ShizukaJiku/gameops/worldregen"
)

func main() {
	// Every subcommand here typically runs from a Scheduled Task with no
	// console attached, so the default stderr output goes nowhere — persist
	// it to a file next to the binary so decisions (idle-stop, backup
	// failures, maintenance actions) are auditable after the fact.
	logFile, err := os.OpenFile("gameops.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatalf("failed to open log file: %v", err)
	}
	log.SetOutput(logFile)

	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "idle-watch":
		runIdleWatch(os.Args[2:])
	case "backup":
		runBackup(os.Args[2:])
	case "maintenance":
		runMaintenance(os.Args[2:])
	case "startup":
		runStartup(os.Args[2:])
	case "world":
		runWorld(os.Args[2:])
	case "host":
		runHost(os.Args[2:])
	case "hash-password":
		runHashPassword(os.Args[2:])
	case "gateway":
		runGateway(os.Args[2:])
	default:
		usage()
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: gameops <subcommand> [flags]")
	fmt.Fprintln(os.Stderr, "subcommands:")
	fmt.Fprintln(os.Stderr, "  idle-watch          run the idle-watch daemon (auto stop/start backend on inactivity)")
	fmt.Fprintln(os.Stderr, "  backup run          back up all configured instances' worlds once and exit")
	fmt.Fprintln(os.Stderr, "  maintenance stop    stop all configured instances (clean RCON stop, force-kill fallback)")
	fmt.Fprintln(os.Stderr, "  maintenance resume  restart all configured instances after maintenance")
	fmt.Fprintln(os.Stderr, "  startup apply       apply configured startup commands to all instances via RCON")
	fmt.Fprintln(os.Stderr, "  world regen -instance <name> [-new-seed]   regenerate one instance's world (backend must be stopped)")
	fmt.Fprintln(os.Stderr, "  host                run the gameops host HTTP API (127.0.0.1 only, proxied by gameops gateway)")
	fmt.Fprintln(os.Stderr, "  hash-password <pwd>  print a bcrypt hash of <pwd>, for admin_password_hash in a gateway config")
	fmt.Fprintln(os.Stderr, "  gateway             run the gameops gateway (HTTPS admin frontend, proxies to configured hosts)")
}

func runIdleWatch(args []string) {
	fs := flag.NewFlagSet("idle-watch", flag.ExitOnError)
	configPath := fs.String("config", "gameops.toml", "path to config file")
	fs.Parse(args)

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("failed to load config %s: %v", *configPath, err)
	}
	if len(cfg.Instances) == 0 {
		log.Fatal("config has no [[instances]] defined")
	}

	done := make(chan error, len(cfg.Instances))
	for _, instCfg := range cfg.Instances {
		instCfg := instCfg
		adapter, err := idlewatch.BuildAdapter(instCfg)
		if err != nil {
			log.Fatalf("[%s] %v", instCfg.Name, err)
		}
		in := idlewatch.NewInstance(instCfg, adapter)
		go func() { done <- in.Run() }()
	}

	err = <-done
	log.Fatalf("an instance stopped unexpectedly: %v", err)
}

func runBackup(args []string) {
	if len(args) < 1 || args[0] != "run" {
		usage()
		os.Exit(1)
	}

	fs := flag.NewFlagSet("backup run", flag.ExitOnError)
	configPath := fs.String("config", "gameops.toml", "path to config file")
	fs.Parse(args[1:])

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("failed to load config %s: %v", *configPath, err)
	}
	if len(cfg.Instances) == 0 {
		log.Fatal("config has no [[instances]] defined")
	}

	failed := false
	for _, instCfg := range cfg.Instances {
		if instCfg.Backup == nil {
			continue
		}
		path, err := backup.Run(instCfg)
		if err != nil {
			log.Printf("[%s] backup failed: %v", instCfg.Name, err)
			failed = true
			continue
		}
		if path != "" {
			log.Printf("[%s] backup complete: %s", instCfg.Name, path)
		}
	}

	if failed {
		os.Exit(1)
	}
}

func runStartup(args []string) {
	if len(args) < 1 || args[0] != "apply" {
		usage()
		os.Exit(1)
	}

	fs := flag.NewFlagSet("startup apply", flag.ExitOnError)
	configPath := fs.String("config", "gameops.toml", "path to config file")
	fs.Parse(args[1:])

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("failed to load config %s: %v", *configPath, err)
	}
	if len(cfg.Instances) == 0 {
		log.Fatal("config has no [[instances]] defined")
	}

	failed := false
	for _, instCfg := range cfg.Instances {
		if err := startup.Apply(instCfg); err != nil {
			log.Printf("[%s] startup apply failed: %v", instCfg.Name, err)
			failed = true
		}
	}

	if failed {
		os.Exit(1)
	}
}

func runMaintenance(args []string) {
	if len(args) < 1 || (args[0] != "stop" && args[0] != "resume") {
		usage()
		os.Exit(1)
	}
	action := args[0]

	fs := flag.NewFlagSet("maintenance "+action, flag.ExitOnError)
	configPath := fs.String("config", "gameops.toml", "path to config file")
	fs.Parse(args[1:])

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("failed to load config %s: %v", *configPath, err)
	}
	if len(cfg.Instances) == 0 {
		log.Fatal("config has no [[instances]] defined")
	}

	failed := false
	for _, instCfg := range cfg.Instances {
		var actionErr error
		if action == "stop" {
			actionErr = maintenance.Stop(instCfg)
		} else {
			actionErr = maintenance.Resume(instCfg)
		}
		if actionErr != nil {
			log.Printf("[%s] maintenance %s failed: %v", instCfg.Name, action, actionErr)
			failed = true
		}
	}

	if failed {
		os.Exit(1)
	}
}

func runWorld(args []string) {
	if len(args) < 1 || args[0] != "regen" {
		usage()
		os.Exit(1)
	}

	fs := flag.NewFlagSet("world regen", flag.ExitOnError)
	configPath := fs.String("config", "gameops.toml", "path to config file")
	instanceName := fs.String("instance", "", "name of the instance to regenerate (required)")
	newSeed := fs.Bool("new-seed", false, "generate a new random seed instead of reusing the current one")
	fs.Parse(args[1:])

	if *instanceName == "" {
		log.Fatal("world regen requires -instance <name> — this is the only subcommand that operates on a single named instance, given how destructive the operation is")
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("failed to load config %s: %v", *configPath, err)
	}

	var target *config.InstanceConfig
	for i := range cfg.Instances {
		if cfg.Instances[i].Name == *instanceName {
			target = &cfg.Instances[i]
			break
		}
	}
	if target == nil {
		log.Fatalf("no instance named %q found in %s", *instanceName, *configPath)
	}

	if err := worldregen.Regen(*target, *newSeed); err != nil {
		log.Fatalf("[%s] world regen failed: %v", target.Name, err)
	}
	log.Printf("[%s] world regen complete", target.Name)
}

func runHost(args []string) {
	fs := flag.NewFlagSet("host", flag.ExitOnError)
	configPath := fs.String("config", "gameops.toml", "path to config file")
	fs.Parse(args)

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("failed to load config %s: %v", *configPath, err)
	}
	if cfg.Host == nil {
		log.Fatal("config has no [host] section defined")
	}

	srv, err := host.NewServer(*cfg, cfg.Host.Token)
	if err != nil {
		log.Fatalf("failed to build host server: %v", err)
	}

	addr := fmt.Sprintf("127.0.0.1:%d", cfg.Host.ListenPort)
	log.Printf("gameops host listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, srv))
}

func runHashPassword(args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "usage: gameops hash-password <password>")
		os.Exit(1)
	}
	hash, err := webauth.HashPassword(args[0])
	if err != nil {
		log.Fatalf("hash-password failed: %v", err)
	}
	fmt.Println(hash)
}

func runGateway(args []string) {
	fs := flag.NewFlagSet("gateway", flag.ExitOnError)
	configPath := fs.String("config", "gameops-gateway.toml", "path to gateway config file")
	fs.Parse(args)

	cfg, err := gwconfig.Load(*configPath)
	if err != nil {
		log.Fatalf("failed to load config %s: %v", *configPath, err)
	}
	if cfg.Domain == "" {
		log.Fatal("gateway config has no domain set (needed for Let's Encrypt certificate issuance)")
	}
	if cfg.SessionSecret == "" {
		log.Fatal("gateway config has no session_secret set (required — an empty secret makes session cookies forgeable)")
	}
	if cfg.AdminPasswordHash == "" {
		log.Fatal("gateway config has no admin_password_hash set (required — generate one with 'gameops hash-password <password>')")
	}

	srv := gateway.NewServer(cfg)

	certManager := autocert.Manager{
		Prompt:     autocert.AcceptTOS,
		HostPolicy: autocert.HostWhitelist(cfg.Domain),
		Cache:      autocert.DirCache("certs"),
	}

	go func() {
		log.Fatal(http.ListenAndServe(":80", certManager.HTTPHandler(nil)))
	}()

	httpsServer := &http.Server{
		Addr:      ":443",
		Handler:   srv,
		TLSConfig: certManager.TLSConfig(),
	}
	log.Printf("gameops gateway listening on :443 (domain %s)", cfg.Domain)
	log.Fatal(httpsServer.ListenAndServeTLS("", ""))
}
