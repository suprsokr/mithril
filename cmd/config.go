package cmd

import (
	"os"
	"path/filepath"
)

// Config holds paths and settings used across all commands.
type Config struct {
	// MithrilDir is the root data directory (default: ./mithril-data).
	MithrilDir string

	// SourceDir is where TrinityCore source is cloned (inside the Docker build).
	SourceDir string

	// ServerDir is the install prefix inside the container.
	ServerDir string

	// DataDir holds extracted client data (maps, vmaps, mmaps, dbc).
	DataDir string

	// ClientDir holds our working copy of the WoW 3.3.5a client.
	ClientDir string

	// DockerComposeFile is the path to the generated docker-compose.yml.
	DockerComposeFile string

	// DockerProjectName is the compose project name.
	DockerProjectName string

	// MySQL credentials.
	MySQLRootPassword string
	MySQLUser         string
	MySQLPassword     string
}

// DefaultConfig returns a Config with sensible defaults relative to cwd.
func DefaultConfig() *Config {
	cwd, _ := os.Getwd()
	dir := filepath.Join(cwd, "mithril-data")

	return &Config{
		MithrilDir:        dir,
		SourceDir:         filepath.Join(dir, "TrinityCore"),
		ServerDir:         "/opt/trinitycore",
		DataDir:           filepath.Join(dir, "data"),
		ClientDir:         filepath.Join(dir, "client"),
		DockerComposeFile: filepath.Join(dir, "docker-compose.yml"),
		DockerProjectName: "mithril",
		MySQLRootPassword: "mithril",
		MySQLUser:         "trinity",
		MySQLPassword:     "trinity",
	}
}

// EnsureDirs creates all host-side directories that get volume-mounted into
// the container.
func (c *Config) EnsureDirs() error {
	for _, d := range []string{
		c.MithrilDir,
		c.DataDir,
		c.ClientDir,
		filepath.Join(c.MithrilDir, "mysql"),
		filepath.Join(c.MithrilDir, "etc"),
		filepath.Join(c.MithrilDir, "log"),
		filepath.Join(c.MithrilDir, "tdb"),
	} {
		if err := os.MkdirAll(d, 0755); err != nil {
			return err
		}
	}
	return nil
}
