package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/suprsokr/mithril/internal/dbc"
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
	patchLetter := cfg.PatchLetter

	// Isolated DBC build: reset database to baseline, apply only this mod's
	// migrations, export, then restore all mods' migrations afterward.
	dbcMigrations := findDBCMigrations(cfg, modName)
	if len(dbcMigrations) > 0 {
		fmt.Println("  Building isolated DBC artifacts...")
		fmt.Println("    Resetting DBC database to baseline...")

		db, err := openDBCDB(cfg)
		if err != nil {
			return fmt.Errorf("connect to dbc database: %w", err)
		}

		// Step 1: Reset to baseline
		if _, _, err := dbc.ImportAllDBCs(db, cfg.BaselineDbcDir, true); err != nil {
			db.Close()
			return fmt.Errorf("reset DBC database: %w", err)
		}

		// Step 2: Apply only this mod's DBC migrations
		fmt.Printf("    Applying %s DBC migrations...\n", modName)
		for _, m := range dbcMigrations {
			sqlContent, err := os.ReadFile(m.path)
			if err != nil {
				db.Close()
				return fmt.Errorf("read migration %s: %w", m.filename, err)
			}
			if _, err := db.Exec(string(sqlContent)); err != nil {
				db.Close()
				return fmt.Errorf("apply migration %s: %w", m.filename, err)
			}
			fmt.Printf("    ✓ %s\n", m.filename)
		}

		// Step 3: Export modified DBC tables
		metaFiles, err := dbc.GetEmbeddedMetaFiles()
		if err != nil {
			db.Close()
			return fmt.Errorf("get meta files: %w", err)
		}

		exportDbcDir := filepath.Join(releaseDir, "dbc_export")
		os.RemoveAll(exportDbcDir)
		if err := os.MkdirAll(exportDbcDir, 0755); err != nil {
			db.Close()
			return fmt.Errorf("create export dir: %w", err)
		}

		exported, err := dbc.ExportModifiedDBCs(db, metaFiles, cfg.BaselineDbcDir, exportDbcDir)
		if err != nil {
			db.Close()
			return fmt.Errorf("export modified DBCs: %w", err)
		}

		// Build file list from exported tables
		var dbcFiles []builtFile
		for _, tableName := range exported {
			for _, metaFile := range metaFiles {
				meta, err := dbc.LoadEmbeddedMeta(metaFile)
				if err != nil {
					continue
				}
				if dbc.TableName(meta) == tableName {
					dbcOutPath := filepath.Join(exportDbcDir, meta.File)
					mpqInternalPath := "DBFilesClient\\" + meta.File
					dbcFiles = append(dbcFiles, builtFile{diskPath: dbcOutPath, mpqPath: mpqInternalPath})
					break
				}
			}
		}

		// Create DBC MPQ
		if len(dbcFiles) > 0 {
			dbcMpqName := "patch-" + patchLetter + ".MPQ"
			dbcMpqPath := filepath.Join(clientDir, "Data", dbcMpqName)
			os.MkdirAll(filepath.Dir(dbcMpqPath), 0755)
			if err := createMPQ(dbcMpqPath, dbcFiles); err != nil {
				db.Close()
				return fmt.Errorf("create DBC MPQ: %w", err)
			}
			hasClient = true
			fmt.Printf("  ✓ Client DBC: Data/%s (%d files)\n", dbcMpqName, len(dbcFiles))

			// Also stage server DBC files
			serverDbcDir := filepath.Join(releaseDir, "server", "dbc")
			os.MkdirAll(serverDbcDir, 0755)
			for _, bf := range dbcFiles {
				dbcFileName := filepath.Base(strings.ReplaceAll(bf.mpqPath, "\\", "/"))
				copyFile(bf.diskPath, filepath.Join(serverDbcDir, dbcFileName))
			}
			fmt.Printf("  ✓ Server DBC files (%d files)\n", len(dbcFiles))
		}

		// Step 4: Restore database — re-import baseline and re-apply all mods' migrations
		fmt.Println("    Restoring DBC database...")
		if _, _, err := dbc.ImportAllDBCs(db, cfg.BaselineDbcDir, true); err != nil {
			db.Close()
			return fmt.Errorf("restore DBC database: %w", err)
		}

		allMods := getAllMods(cfg)
		tracker, _ := loadSQLTracker(cfg)
		for _, mod := range allMods {
			for _, m := range findDBCMigrations(cfg, mod) {
				if !tracker.IsApplied(m.mod, m.filename) {
					continue
				}
				sqlContent, err := os.ReadFile(m.path)
				if err != nil {
					fmt.Printf("    ⚠ Failed to read %s: %v\n", m.filename, err)
					continue
				}
				if _, err := db.Exec(string(sqlContent)); err != nil {
					fmt.Printf("    ⚠ Failed to re-apply %s: %v\n", m.filename, err)
				}
			}
		}
		db.Close()
		fmt.Println("    ✓ DBC database restored")
	}

	// Copy addon files
	addonFiles := collectModAddons(cfg, modName)
	if len(addonFiles) > 0 {
		addonMpqName := "patch-" + locale + "-" + patchLetter + ".MPQ"
		addonMpqPath := filepath.Join(clientDir, "Data", locale, addonMpqName)
		os.MkdirAll(filepath.Dir(addonMpqPath), 0755)
		if err := createMPQ(addonMpqPath, addonFiles); err != nil {
			return fmt.Errorf("create addon MPQ: %w", err)
		}
		hasClient = true
		fmt.Printf("  ✓ Client addons: Data/%s/%s (%d files)\n", locale, addonMpqName, len(addonFiles))
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

	// Stage server files (DBC files may already be staged above, so preserve them)
	serverDir := filepath.Join(releaseDir, "server")
	// Clean any leftover non-dbc server content from previous exports
	for _, subdir := range []string{"sql", "core-patches"} {
		os.RemoveAll(filepath.Join(serverDir, subdir))
	}
	hasServer := false

	// Check if server dir has any content (DBC files may have been staged above)
	if entries, err := os.ReadDir(serverDir); err == nil && len(entries) > 0 {
		hasServer = true
	}

	// Copy SQL migrations (exclude dbc/ — those are used to build .dbc binaries, not for the server)
	sqlDir := filepath.Join(cfg.ModDir(modName), "sql")
	if entries, err := os.ReadDir(sqlDir); err == nil {
		for _, entry := range entries {
			if !entry.IsDir() || entry.Name() == "dbc" {
				continue
			}
			srcSubdir := filepath.Join(sqlDir, entry.Name())
			destSubdir := filepath.Join(serverDir, "sql", entry.Name())
			if err := copyDirRecursive(srcSubdir, destSubdir); err == nil {
				hasServer = true
				fmt.Printf("  ✓ Server SQL migrations (%s)\n", entry.Name())
			}
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
		fmt.Println("No artifacts to export.")
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
