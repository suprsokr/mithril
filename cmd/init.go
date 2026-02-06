package cmd

import (
	"fmt"
	"path/filepath"
)

func runInit(args []string) error {
	cfg := DefaultConfig()

	totalSteps := 9
	step := 0

	// 1. Client
	step++
	printStep(step, totalSteps, "Locating WoW 3.3.5a client")
	if err := promptAndCopyClient(cfg); err != nil {
		return fmt.Errorf("client setup failed: %w", err)
	}

	// 2. Directories
	step++
	printStep(step, totalSteps, "Creating directory structure")
	if err := cfg.EnsureDirs(); err != nil {
		return fmt.Errorf("failed to create directories: %w", err)
	}
	printSuccess("Directory structure created at " + cfg.MithrilDir)

	// 3. Dockerfile
	step++
	printStep(step, totalSteps, "Generating Dockerfile")
	if err := writeDockerfile(filepath.Join(cfg.MithrilDir, "Dockerfile")); err != nil {
		return fmt.Errorf("failed to write Dockerfile: %w", err)
	}
	printSuccess("Dockerfile written")

	// 4. docker-compose.yml
	step++
	printStep(step, totalSteps, "Generating docker-compose.yml")
	if err := writeDockerCompose(cfg); err != nil {
		return fmt.Errorf("failed to write docker-compose.yml: %w", err)
	}
	printSuccess("docker-compose.yml written")

	// 5. Container scripts
	step++
	printStep(step, totalSteps, "Generating container scripts")
	if err := writeContainerScripts(cfg); err != nil {
		return fmt.Errorf("failed to write container scripts: %w", err)
	}
	printSuccess("Container scripts written")

	// 6. TDB database
	step++
	printStep(step, totalSteps, "Downloading TDB full database")
	if err := downloadTDB(cfg); err != nil {
		printWarning(fmt.Sprintf("TDB download issue: %v", err))
		printInfo("You can manually place TDB_full_world_335.*.sql into mithril-data/tdb/")
	}

	// 7. Docker image
	step++
	printStep(step, totalSteps, "Building Docker image (cloning and compiling TrinityCore — this will take a while)")
	if err := buildDockerImage(cfg); err != nil {
		return fmt.Errorf("docker build failed: %w", err)
	}
	printSuccess("Docker image 'mithril-server' built successfully")

	// 8. Server configs
	step++
	printStep(step, totalSteps, "Generating server configuration files")
	if err := writeServerConfigs(cfg); err != nil {
		return fmt.Errorf("failed to write config files: %w", err)
	}
	printSuccess("Configuration files written to " + filepath.Join(cfg.MithrilDir, "etc"))

	// 9. Extract maps
	step++
	printStep(step, totalSteps, "Extracting client data (maps, vmaps, mmaps)")
	if err := extractClientData(cfg); err != nil {
		return fmt.Errorf("data extraction failed: %w", err)
	}
	printSuccess("Client data extracted to " + cfg.DataDir)

	// Done
	fmt.Println()
	fmt.Println("═══════════════════════════════════════════════════════════════")
	fmt.Println("  Mithril initialization complete!")
	fmt.Println()
	fmt.Println("  Start the server:")
	fmt.Println("    mithril server start")
	fmt.Println()
	fmt.Println("  Once the server is running, create a game account:")
	fmt.Println("    mithril server account create <username> <password>")
	fmt.Println()
	fmt.Println("  Example (admin with full GM permissions):")
	fmt.Println("    mithril server account create admin admin")
	fmt.Println()
	fmt.Println("  Attach to the worldserver console:")
	fmt.Println("    mithril server attach")
	fmt.Println("═══════════════════════════════════════════════════════════════")
	return nil
}
