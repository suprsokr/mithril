package cmd

import (
	"crypto/rand"
	"crypto/sha1"
	"fmt"
	"math/big"
	"os/exec"
	"strconv"
	"strings"
)

// SRP6 constants used by TrinityCore for authentication.
var (
	srp6G = big.NewInt(7)
	srp6N = func() *big.Int {
		n, _ := new(big.Int).SetString("894B645E89E1535BBDAD5B8B290650530801B18EBFBF5E8FAB3C82872A3E9BB7", 16)
		return n
	}()
)

func runAccount(subcmd string, args []string) error {
	cfg := DefaultConfig()

	if !fileExists(cfg.DockerComposeFile) {
		return fmt.Errorf("mithril not initialized — run 'mithril init' first")
	}

	switch subcmd {
	case "create":
		return accountCreate(cfg, args)
	default:
		return fmt.Errorf("unknown account subcommand: %s (use: create)", subcmd)
	}
}

func accountCreate(cfg *Config, args []string) error {
	if len(args) < 2 {
		fmt.Println("Usage: mithril server account create <username> <password> [gm_level]")
		fmt.Println()
		fmt.Println("GM Levels:")
		fmt.Println("  0 = Player")
		fmt.Println("  1 = Moderator")
		fmt.Println("  2 = GameMaster")
		fmt.Println("  3 = Administrator (default)")
		return fmt.Errorf("username and password are required")
	}

	username := args[0]
	password := args[1]
	gmLevel := 3
	if len(args) >= 3 {
		level, err := strconv.Atoi(args[2])
		if err != nil || level < 0 || level > 3 {
			return fmt.Errorf("gm_level must be 0, 1, 2, or 3")
		}
		gmLevel = level
	}

	// SRP6 requires uppercase
	usernameUpper := strings.ToUpper(username)
	passwordUpper := strings.ToUpper(password)

	// Check if the container is running
	containerID, err := composeContainerID(cfg)
	if err != nil || containerID == "" {
		return fmt.Errorf("server is not running — start it with 'mithril server start'")
	}

	// Check if account already exists
	out, err := dockerExecOutput(containerID,
		"mysql", "-u"+cfg.MySQLUser, "-p"+cfg.MySQLPassword,
		"-N", "-e",
		fmt.Sprintf("SELECT COUNT(*) FROM auth.account WHERE username = '%s';", usernameUpper))
	if err != nil {
		return fmt.Errorf("failed to check existing account: %w", err)
	}
	count := strings.TrimSpace(out)
	if count != "0" {
		return fmt.Errorf("account '%s' already exists", username)
	}

	// Compute SRP6 salt and verifier
	salt, verifier, err := computeSRP6(usernameUpper, passwordUpper)
	if err != nil {
		return fmt.Errorf("failed to compute SRP6 credentials: %w", err)
	}

	saltHex := fmt.Sprintf("%x", salt)
	verifierHex := fmt.Sprintf("%x", verifier)

	// Insert account
	insertSQL := fmt.Sprintf(
		"INSERT INTO auth.account (username, salt, verifier, email, reg_mail, expansion) "+
			"VALUES ('%s', X'%s', X'%s', '', '', 2);",
		usernameUpper, saltHex, verifierHex)

	_, err = dockerExecOutput(containerID,
		"mysql", "-u"+cfg.MySQLUser, "-p"+cfg.MySQLPassword, "-e", insertSQL)
	if err != nil {
		return fmt.Errorf("failed to create account: %w", err)
	}

	// Set GM level if > 0
	if gmLevel > 0 {
		// Get account ID
		out, err = dockerExecOutput(containerID,
			"mysql", "-u"+cfg.MySQLUser, "-p"+cfg.MySQLPassword,
			"-N", "-e",
			fmt.Sprintf("SELECT id FROM auth.account WHERE username = '%s';", usernameUpper))
		if err != nil {
			return fmt.Errorf("failed to retrieve account ID: %w", err)
		}
		accountID := strings.TrimSpace(out)

		gmSQL := fmt.Sprintf(
			"INSERT INTO auth.account_access (AccountID, SecurityLevel, RealmID, Comment) "+
				"VALUES (%s, %d, -1, 'Created by mithril');",
			accountID, gmLevel)

		_, err = dockerExecOutput(containerID,
			"mysql", "-u"+cfg.MySQLUser, "-p"+cfg.MySQLPassword, "-e", gmSQL)
		if err != nil {
			return fmt.Errorf("failed to set GM level: %w", err)
		}
	}

	fmt.Println()
	printSuccess("Account created!")
	printInfo(fmt.Sprintf("Username: %s", username))
	printInfo(fmt.Sprintf("Password: %s", password))
	printInfo(fmt.Sprintf("GM Level: %d", gmLevel))
	fmt.Println()
	printInfo("You can now login to the game!")
	return nil
}

// computeSRP6 calculates the salt and verifier for TrinityCore's SRP6
// authentication system.
//
// Algorithm:
//
//	v = g ^ SHA1(salt || SHA1(username || ':' || password)) mod N
//
// Salt and verifier are stored as 32-byte little-endian byte arrays.
func computeSRP6(username, password string) (salt, verifier []byte, err error) {
	// Generate random 32-byte salt
	salt = make([]byte, 32)
	if _, err := rand.Read(salt); err != nil {
		return nil, nil, err
	}

	// Step 1: H(username || ':' || password)
	h1 := sha1.Sum([]byte(username + ":" + password))

	// Step 2: H(salt || h1) — concatenating raw bytes
	h2data := append(salt, h1[:]...)
	h2 := sha1.Sum(h2data)

	// Convert h2 to big.Int (little-endian)
	x := new(big.Int).SetBytes(reverseCopy(h2[:]))

	// Step 3: v = g^x mod N
	v := new(big.Int).Exp(srp6G, x, srp6N)

	// Convert verifier to 32-byte little-endian
	vBytes := v.Bytes()                   // big-endian
	verifier = make([]byte, 32)           // zero-filled 32 bytes
	reversed := reverseCopy(vBytes)       // little-endian
	copy(verifier, reversed)              // pad with trailing zeros if < 32 bytes

	return salt, verifier, nil
}

// reverseCopy returns a new slice with bytes in reversed order.
func reverseCopy(b []byte) []byte {
	out := make([]byte, len(b))
	for i, v := range b {
		out[len(b)-1-i] = v
	}
	return out
}

// dockerExecOutput runs a command inside the container and returns stdout.
func dockerExecOutput(containerID string, cmdArgs ...string) (string, error) {
	args := append([]string{"exec", containerID}, cmdArgs...)
	cmd := exec.Command("docker", args...)
	out, err := cmd.CombinedOutput()
	// Filter out MySQL password warnings
	lines := strings.Split(string(out), "\n")
	var filtered []string
	for _, line := range lines {
		if !strings.Contains(line, "Using a password on the command line") {
			filtered = append(filtered, line)
		}
	}
	return strings.Join(filtered, "\n"), err
}
