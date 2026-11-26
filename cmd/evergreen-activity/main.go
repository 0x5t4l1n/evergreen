// Command evergreen-activity fetches public GitHub activity for a list of
// users and writes it to a static JSON file for the Evergreen site. It reads
// an optional GITHUB_TOKEN from the environment; the token is used for API
// requests only and is never written to the output.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"evergreen/internal/github/fetch"
)

func main() {
	users := flag.String("users", os.Getenv("EVERGREEN_GITHUB_USERS"), "comma separated GitHub logins")
	out := flag.String("output", "web/data/activity.json", "output JSON path")
	flag.Parse()

	var logins []string
	for _, u := range strings.Split(*users, ",") {
		if u = strings.TrimSpace(u); u != "" {
			logins = append(logins, u)
		}
	}
	if len(logins) == 0 {
		fmt.Fprintln(os.Stderr, "no users configured (use --users or EVERGREEN_GITHUB_USERS); writing an empty activity file")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	file, errs := fetch.NewClient(os.Getenv("GITHUB_TOKEN")).Build(ctx, logins, time.Now())
	for _, e := range errs {
		fmt.Fprintln(os.Stderr, "warning:", e)
	}
	if len(logins) > 0 && len(file.Users) == 0 {
		fmt.Fprintln(os.Stderr, "error: no user could be fetched; keeping the previous file")
		os.Exit(1)
	}
	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	if err := os.MkdirAll(filepath.Dir(*out), 0o755); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	if err := os.WriteFile(*out, append(data, '\n'), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	fmt.Printf("wrote %s (%d users)\n", *out, len(file.Users))
}
