package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func runModPublish(args []string) error {
	if len(args) == 0 {
		fmt.Print(publishUsage)
		return nil
	}

	switch args[0] {
	case "export":
		return runModPublishExport(args[1:])
	case "register":
		return runModPublishRegister(args[1:])
	case "-h", "--help", "help":
		fmt.Print(publishUsage)
		return nil
	default:
		return fmt.Errorf("unknown publish command: %s", args[0])
	}
}

const publishUsage = `Mithril Mod Publish - Share your mods with the community

Usage:
  mithril mod publish <command> [args]

Commands:
  register --mod <name>     Generate a registry JSON file for the mod
  export --mod <name>       Export mod as client.zip and server.zip (optional)

The primary way to share mods is through a git repository. Other mithril users
can install your mod with 'mithril mod registry install <name>' which clones
your repo and lets them build locally.

The export command is optional — it produces pre-built release artifacts
(client.zip, server.zip) for users who don't use mithril and want to manually
install the mod files.

Examples:
  mithril mod publish register --mod my-mod --repo https://github.com/user/my-mod
  mithril mod publish export --mod my-mod
`

func runModPublishExport(args []string) error {
	modName, _ := parseModFlag(args)
	if modName == "" {
		return fmt.Errorf("usage: mithril mod publish export --mod <name>")
	}

	cfg := DefaultConfig()
	if _, err := os.Stat(filepath.Join(cfg.ModDir(modName), "mod.json")); os.IsNotExist(err) {
		return fmt.Errorf("mod not found: %s", modName)
	}

	releaseDir := filepath.Join(cfg.ModulesBuildDir, "release", modName)
	if err := os.MkdirAll(releaseDir, 0755); err != nil {
		return fmt.Errorf("create release dir: %w", err)
	}

	fmt.Printf("=== Exporting %s for release ===\n\n", modName)

	// Stage client files
	clientDir := filepath.Join(releaseDir, "client")
	os.RemoveAll(clientDir)

	hasClient := false
	locale := detectLocaleFromManifest(cfg)

	// Copy DBC MPQ
	patchLetter := cfg.PatchLetter
	dbcMpqName := "patch-" + patchLetter + ".MPQ"
	dbcMpq := filepath.Join(cfg.ModulesBuildDir, dbcMpqName)
	if _, err := os.Stat(dbcMpq); err == nil {
		destDir := filepath.Join(clientDir, "Data")
		os.MkdirAll(destDir, 0755)
		copyFile(dbcMpq, filepath.Join(destDir, dbcMpqName))
		hasClient = true
		fmt.Printf("  ✓ Client DBC: Data/%s\n", dbcMpqName)
	}

	// Copy addon MPQ
	addonMpqName := "patch-" + locale + "-" + patchLetter + ".MPQ"
	addonMpq := filepath.Join(cfg.ModulesBuildDir, addonMpqName)
	if _, err := os.Stat(addonMpq); err == nil {
		destDir := filepath.Join(clientDir, "Data", locale)
		os.MkdirAll(destDir, 0755)
		copyFile(addonMpq, filepath.Join(destDir, addonMpqName))
		hasClient = true
		fmt.Printf("  ✓ Client addons: Data/%s/%s\n", locale, addonMpqName)
	}

	// Copy binary patches
	binaryPatchDir := filepath.Join(cfg.ModDir(modName), "binary-patches")
	if entries, err := os.ReadDir(binaryPatchDir); err == nil {
		for _, entry := range entries {
			if strings.HasSuffix(entry.Name(), ".json") {
				destDir := filepath.Join(clientDir, "binary-patches")
				os.MkdirAll(destDir, 0755)
				copyFile(filepath.Join(binaryPatchDir, entry.Name()), filepath.Join(destDir, entry.Name()))
				hasClient = true
				fmt.Printf("  ✓ Binary patch: %s\n", entry.Name())
			}
		}
	}

	// Stage server files
	serverDir := filepath.Join(releaseDir, "server")
	os.RemoveAll(serverDir)

	hasServer := false

	// Copy SQL migrations
	sqlDir := filepath.Join(cfg.ModDir(modName), "sql")
	if _, err := os.Stat(sqlDir); err == nil {
		destDir := filepath.Join(serverDir, "sql")
		if err := copyDirRecursive(sqlDir, destDir); err == nil {
			hasServer = true
			fmt.Println("  ✓ Server SQL migrations")
		}
	}

	// Copy core patches
	corePatchDir := filepath.Join(cfg.ModDir(modName), "core-patches")
	if _, err := os.Stat(corePatchDir); err == nil {
		destDir := filepath.Join(serverDir, "core-patches")
		if err := copyDirRecursive(corePatchDir, destDir); err == nil {
			hasServer = true
			fmt.Println("  ✓ Server core patches")
		}
	}

	// Copy built DBC files for the server
	buildDbcDir := filepath.Join(cfg.ModulesBuildDir, modName, "DBFilesClient")
	if entries, err := os.ReadDir(buildDbcDir); err == nil {
		for _, entry := range entries {
			if strings.HasSuffix(strings.ToLower(entry.Name()), ".dbc") {
				destDir := filepath.Join(serverDir, "dbc")
				os.MkdirAll(destDir, 0755)
				copyFile(filepath.Join(buildDbcDir, entry.Name()), filepath.Join(destDir, entry.Name()))
				hasServer = true
			}
		}
		if hasServer {
			fmt.Println("  ✓ Server DBC files")
		}
	}

	// Create zips
	fmt.Println()
	if hasClient {
		clientZip := filepath.Join(releaseDir, "client.zip")
		if err := createZip(clientDir, clientZip); err != nil {
			fmt.Printf("  ⚠ Failed to create client.zip: %v\n", err)
		} else {
			fmt.Printf("  📦 %s\n", clientZip)
		}
	}

	if hasServer {
		serverZip := filepath.Join(releaseDir, "server.zip")
		if err := createZip(serverDir, serverZip); err != nil {
			fmt.Printf("  ⚠ Failed to create server.zip: %v\n", err)
		} else {
			fmt.Printf("  📦 %s\n", serverZip)
		}
	}

	if !hasClient && !hasServer {
		fmt.Println("No artifacts to export. Run 'mithril mod build' first.")
		return nil
	}

	fmt.Printf("\n=== Export Complete ===\n")
	fmt.Printf("  Release files: %s\n", releaseDir)
	fmt.Println()
	fmt.Println("These pre-built artifacts are for users who don't use mithril.")
	fmt.Println("Upload client.zip and/or server.zip to a GitHub release so")
	fmt.Println("non-mithril users can download and install them manually.")
	fmt.Println()
	fmt.Println("Mithril users install mods via git clone — no release artifacts needed.")
	fmt.Println("To register your mod: mithril mod publish register --mod " + modName)

	return nil
}

func runModPublishRegister(args []string) error {
	modName, remaining := parseModFlag(args)
	if modName == "" {
		return fmt.Errorf("usage: mithril mod publish register --mod <name> --repo <url>")
	}

	cfg := DefaultConfig()
	modMeta, err := loadModMeta(cfg, modName)
	if err != nil {
		return fmt.Errorf("mod not found: %s", modName)
	}

	// Parse --repo flag
	repo := ""
	for i := 0; i < len(remaining); i++ {
		if remaining[i] == "--repo" && i+1 < len(remaining) {
			repo = remaining[i+1]
			break
		}
	}

	if repo == "" {
		return fmt.Errorf("--repo is required: mithril mod publish register --mod %s --repo https://github.com/user/%s", modName, modName)
	}

	// Detect mod types
	var modTypes []string
	if dbcMigrations := findDBCMigrations(cfg, modName); len(dbcMigrations) > 0 {
		modTypes = append(modTypes, "dbc")
	}
	if addons := findModifiedAddons(cfg, modName); len(addons) > 0 {
		modTypes = append(modTypes, "addon")
	}
	if migrations := findMigrations(cfg, modName); len(migrations) > 0 {
		modTypes = append(modTypes, "sql")
	}
	if patches := findCorePatches(cfg, modName); len(patches) > 0 {
		modTypes = append(modTypes, "core")
	}
	binaryPatchDir := filepath.Join(cfg.ModDir(modName), "binary-patches")
	if entries, err := os.ReadDir(binaryPatchDir); err == nil && len(entries) > 0 {
		modTypes = append(modTypes, "binary-patch")
	}

	entry := RegistryEntry{
		Name:        modName,
		Description: modMeta.Description,
		Author:      "", // user fills in
		Repo:        repo,
		Tags:        []string{},
		Version:     "1.0.0",
		ModTypes:    modTypes,
	}

	// Write to the mod directory
	registryFile := filepath.Join(cfg.ModDir(modName), modName+".registry.json")
	data, _ := json.MarshalIndent(entry, "", "  ")
	if err := os.WriteFile(registryFile, data, 0644); err != nil {
		return fmt.Errorf("write registry file: %w", err)
	}

	fmt.Printf("✓ Generated registry file: %s\n\n", registryFile)
	fmt.Println("To register your mod:")
	fmt.Println("  1. Push your mod to a git repository (e.g. GitHub)")
	fmt.Println("  2. Fill in 'author', 'description', and 'tags' in the JSON file")
	fmt.Println("  3. Fork https://github.com/suprsokr/mithril-registry")
	fmt.Printf("  4. Copy %s to mods/%s.json in your fork\n", registryFile, modName)
	fmt.Println("  5. Submit a pull request")
	fmt.Println()
	fmt.Println("Once registered, other mithril users can install your mod with:")
	fmt.Printf("  mithril mod registry install %s\n", modName)

	return nil
}

// copyDirRecursive copies a directory tree.
func copyDirRecursive(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		destPath := filepath.Join(dst, rel)

		if info.IsDir() {
			return os.MkdirAll(destPath, 0755)
		}

		return copyFile(path, destPath)
	})
}

func createZip(sourceDir, zipPath string) error {
	// Use zip command relative to the parent dir so paths inside zip are clean
	parentDir := filepath.Dir(sourceDir)
	baseName := filepath.Base(sourceDir)

	cmd := exec.Command("zip", "-r", "-q", zipPath, baseName)
	cmd.Dir = parentDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %s", err, string(output))
	}
	return nil
}
