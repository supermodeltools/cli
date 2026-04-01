package auth

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"syscall"

	"golang.org/x/term"

	"github.com/supermodeltools/cli/internal/config"
	"github.com/supermodeltools/cli/internal/ui"
)

// Login prompts the user for an API key and saves it to the config file.
// Input is read without echo when a terminal is attached.
func Login(_ context.Context) error {
	fmt.Println("Get your API key at https://supermodeltools.com/dashboard")
	fmt.Print("Paste your API key: ")

	key, err := readSecret()
	if err != nil {
		return fmt.Errorf("read input: %w", err)
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return fmt.Errorf("API key cannot be empty")
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	cfg.APIKey = key
	if err := cfg.Save(); err != nil {
		return err
	}

	ui.Success("Authenticated — key saved to %s", config.Path())
	return nil
}

// Logout removes the API key from the config file.
func Logout(_ context.Context) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if cfg.APIKey == "" {
		fmt.Println("Already logged out.")
		return nil
	}
	cfg.APIKey = ""
	if err := cfg.Save(); err != nil {
		return err
	}
	ui.Success("Logged out — API key removed from %s", config.Path())
	return nil
}

// readSecret reads a line from stdin, suppressing echo when a TTY is attached.
func readSecret() (string, error) {
	fd := int(syscall.Stdin)
	if term.IsTerminal(fd) {
		b, err := term.ReadPassword(fd)
		fmt.Println() // restore newline after hidden input
		if err != nil {
			return "", err
		}
		return string(b), nil
	}
	// Non-TTY (pipe, CI): read as plain text
	scanner := bufio.NewScanner(os.Stdin)
	if scanner.Scan() {
		return scanner.Text(), nil
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return "", fmt.Errorf("no input received")
}
