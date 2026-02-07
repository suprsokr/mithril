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

	// ModulesDir is the root of the modding workspace.
	ModulesDir string

	// BaselineDir holds pristine DBC data extracted from the client MPQs.
	// This is the shared reference that all mods compare against.
	BaselineDir string

	// BaselineDbcDir holds raw .dbc binaries extracted from MPQs.
	BaselineDbcDir string


	// BaselineAddonsDir holds pristine addon files (lua, xml, toc) extracted from MPQs.
	BaselineAddonsDir string

	// ModulesBuildDir holds build artifacts (generated .dbc and .MPQ files).
	ModulesBuildDir string

	// ServerDbcDir holds DBC files used by the TrinityCore server.
	ServerDbcDir string

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
		ModulesDir:        filepath.Join(dir, "modules"),
		BaselineDir:       filepath.Join(dir, "modules", "baseline"),
		BaselineDbcDir:    filepath.Join(dir, "modules", "baseline", "dbc"),
		BaselineAddonsDir: filepath.Join(dir, "modules", "baseline", "addons"),
		ModulesBuildDir:   filepath.Join(dir, "modules", "build"),
		ServerDbcDir:      filepath.Join(dir, "data", "dbc"),
		DockerComposeFile: filepath.Join(dir, "docker-compose.yml"),
		DockerProjectName: "mithril",
		MySQLRootPassword: "mithril",
		MySQLUser:         "trinity",
		MySQLPassword:     "trinity",
	}
}

// ModDir returns the directory for a named mod.
func (c *Config) ModDir(modName string) string {
	return filepath.Join(c.ModulesDir, modName)
}

// ModAddonsDir returns the addons directory for a named mod.
func (c *Config) ModAddonsDir(modName string) string {
	return filepath.Join(c.ModulesDir, modName, "addons")
}

// MySQLHost returns the host for connecting to MySQL.
// Uses localhost since port 3306 is exposed from the Docker container.
func (c *Config) MySQLHost() string {
	return "127.0.0.1"
}

// MySQLPort returns the port for connecting to MySQL.
func (c *Config) MySQLPort() string {
	return "3306"
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
