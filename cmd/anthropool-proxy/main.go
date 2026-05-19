package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/dtnguyen/anthropool-proxy/internal/config"
	"github.com/dtnguyen/anthropool-proxy/internal/pool"
	"github.com/dtnguyen/anthropool-proxy/internal/proxy"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "serve":
		cmdServe()
	case "add":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "usage: anthropool-proxy add <label>")
			os.Exit(1)
		}
		cmdAdd(os.Args[2])
	case "list":
		cmdList()
	case "remove":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "usage: anthropool-proxy remove <id|label>")
			os.Exit(1)
		}
		cmdRemove(os.Args[2])
	case "status":
		cmdStatus()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", os.Args[1])
		usage()
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `anthropool-proxy — Claude token pool proxy

Commands:
  serve            Start the proxy server
  add <label>      Add a token to the pool (prompts for bearer token)
  list             List tokens (masked) and cooldown state
  remove <id|label> Remove a token from the pool
  status           Show per-token request counts and cooldown state`)
}

func cmdServe() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("loading config: %v", err)
	}
	if len(cfg.Tokens) == 0 {
		log.Fatal("no tokens configured — run: anthropool-proxy add <label>")
	}

	p := pool.New(cfg.Tokens, cfg.CooldownMinutes)

	h, err := proxy.New(p)
	if err != nil {
		log.Fatalf("creating proxy: %v", err)
	}

	srv := &http.Server{
		Addr:         cfg.Listen,
		Handler:      h,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Minute,
		IdleTimeout:  120 * time.Second,
	}

	log.Printf("anthropool-proxy listening on %s with %d token(s), cooldown=%dm",
		cfg.Listen, len(cfg.Tokens), cfg.CooldownMinutes)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()

	<-quit
	log.Println("shutting down...")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("shutdown: %v", err)
	}
	log.Println("stopped")
}

func cmdAdd(label string) {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("loading config: %v", err)
	}

	// Check for duplicate label
	for _, t := range cfg.Tokens {
		if t.Label == label {
			fmt.Fprintf(os.Stderr, "token with label %q already exists (id: %s)\n", label, t.ID)
			os.Exit(1)
		}
	}

	fmt.Printf("Enter bearer token for %q: ", label)
	reader := bufio.NewReader(os.Stdin)
	token, err := reader.ReadString('\n')
	if err != nil {
		log.Fatalf("reading token: %v", err)
	}
	token = strings.TrimSpace(token)
	if token == "" {
		fmt.Fprintln(os.Stderr, "token cannot be empty")
		os.Exit(1)
	}

	id := generateID()
	cfg.Tokens = append(cfg.Tokens, config.Token{
		ID:          id,
		Label:       label,
		BearerToken: token,
	})

	if err := config.Save(cfg); err != nil {
		log.Fatalf("saving config: %v", err)
	}
	fmt.Printf("Added token %q (id: %s, token: %s)\n", label, id, config.MaskToken(token))
}

func cmdList() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("loading config: %v", err)
	}
	if len(cfg.Tokens) == 0 {
		fmt.Println("no tokens configured")
		return
	}
	fmt.Printf("%-24s %-20s %-12s %s\n", "ID", "LABEL", "TOKEN", "STATUS")
	fmt.Println(strings.Repeat("-", 70))
	for _, t := range cfg.Tokens {
		fmt.Printf("%-24s %-20s %-12s %s\n", t.ID, t.Label, config.MaskToken(t.BearerToken), "ready")
	}
}

func cmdRemove(idOrLabel string) {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("loading config: %v", err)
	}

	idx := -1
	for i, t := range cfg.Tokens {
		if t.ID == idOrLabel || t.Label == idOrLabel {
			idx = i
			break
		}
	}
	if idx < 0 {
		fmt.Fprintf(os.Stderr, "token %q not found\n", idOrLabel)
		os.Exit(1)
	}

	removed := cfg.Tokens[idx]
	cfg.Tokens = append(cfg.Tokens[:idx], cfg.Tokens[idx+1:]...)

	if err := config.Save(cfg); err != nil {
		log.Fatalf("saving config: %v", err)
	}
	fmt.Printf("Removed token %q (id: %s)\n", removed.Label, removed.ID)
}

func cmdStatus() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("loading config: %v", err)
	}
	if len(cfg.Tokens) == 0 {
		fmt.Println("no tokens configured")
		return
	}

	// Build pool to show cooldown state — but since status is read-only and
	// the proxy runs in a separate process, cooldown state is in-memory only.
	// We show the static config info with a note.
	fmt.Printf("Config: %s  cooldown=%dm\n\n", config.ConfigPath(), cfg.CooldownMinutes)
	fmt.Printf("%-24s %-20s %-12s %s\n", "ID", "LABEL", "TOKEN", "NOTE")
	fmt.Println(strings.Repeat("-", 70))
	for _, t := range cfg.Tokens {
		fmt.Printf("%-24s %-20s %-12s %s\n", t.ID, t.Label, config.MaskToken(t.BearerToken), "cooldown state is per-process (see server logs)")
	}
	fmt.Println("\nNote: live request counts and cooldown state are tracked in the running")
	fmt.Println("server process. Check server logs for real-time status.")
}

func generateID() string {
	b := make([]byte, 8)
	rand.Read(b) //nolint:errcheck
	return hex.EncodeToString(b)
}
