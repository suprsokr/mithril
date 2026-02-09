package cmd

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const scriptUsage = `Mithril Mod Script — TrinityCore Server-Side C++ Scripts

Usage:
  mithril mod script <command> [args]

Commands:
  create <name> --mod <mod> [--type <type>]
                              Create a new C++ script file
  remove <name> --mod <mod>   Remove a script file
  list [--mod <mod>]          List all scripts across mods (or for a specific mod)

Script types (use with --type):
  creature      Custom NPC AI (default)
  player        Player event hooks (login, kill, chat, level, etc.)
  spell         Custom spell/aura handlers
  command       Custom GM/.chat commands
  worldscript   World/server event hooks (startup, shutdown, tick)
  item          Item use/quest/expire handlers
  gameobject    GameObject AI
  areatrigger   Area trigger handlers
  unit          Unit damage/healing modifiers

Workflow:
  mithril mod script create my_npc --mod my-mod
  mithril mod script create welcome_msg --mod my-mod --type player
  mithril mod script create custom_spell --mod my-mod --type spell
  # Edit modules/my-mod/scripts/<name>.cpp
  mithril mod build
  mithril server restart
`

// containerCustomScriptsDir is where TrinityCore looks for custom scripts.
const containerCustomScriptsDir = "/src/TrinityCore/src/server/scripts/Custom"

func runModScript(subcmd string, args []string) error {
	switch subcmd {
	case "create":
		return runModScriptCreate(args)
	case "remove":
		return runModScriptRemove(args)
	case "list":
		return runModScriptList(args)
	case "-h", "--help", "help":
		fmt.Print(scriptUsage)
		return nil
	default:
		return fmt.Errorf("unknown mod script command: %s", subcmd)
	}
}

func runModScriptCreate(args []string) error {
	modName, remaining := parseModFlag(args)
	scriptType, remaining := parseStringFlag(remaining, "type")
	if len(remaining) < 1 || modName == "" {
		return fmt.Errorf("usage: mithril mod script create <name> --mod <mod_name> [--type <type>]")
	}

	if scriptType == "" {
		scriptType = "creature"
	}

	cfg := DefaultConfig()
	scriptName := remaining[0]

	// Ensure mod exists
	if _, err := os.Stat(filepath.Join(cfg.ModDir(modName), "mod.json")); os.IsNotExist(err) {
		return fmt.Errorf("mod not found: %s (run 'mithril mod create %s' first)", modName, modName)
	}

	// Sanitize name
	safeName := strings.ReplaceAll(strings.ToLower(scriptName), " ", "_")
	if !strings.HasSuffix(safeName, ".cpp") && !strings.HasSuffix(safeName, ".h") {
		safeName += ".cpp"
	}

	scriptsDir := filepath.Join(cfg.ModDir(modName), "scripts")
	scriptPath := filepath.Join(scriptsDir, safeName)

	if _, err := os.Stat(scriptPath); err == nil {
		return fmt.Errorf("script file already exists: %s", scriptPath)
	}

	if err := os.MkdirAll(scriptsDir, 0755); err != nil {
		return fmt.Errorf("create scripts dir: %w", err)
	}

	baseName := strings.TrimSuffix(safeName, filepath.Ext(safeName))
	className := snakeToPascal(baseName)

	template, err := scriptTemplate(scriptType, scriptName, modName, baseName, className)
	if err != nil {
		return err
	}

	if err := os.WriteFile(scriptPath, []byte(template), 0644); err != nil {
		return fmt.Errorf("write script file: %w", err)
	}

	fmt.Printf("✓ Created %s script: %s\n", scriptType, scriptPath)
	fmt.Println()
	fmt.Println("Next steps:")
	fmt.Printf("  1. Edit the script:        %s\n", scriptPath)
	fmt.Printf("  2. Build mods:             mithril mod build\n")
	fmt.Printf("  3. Restart server:         mithril server restart\n")
	return nil
}

// parseStringFlag extracts --flag value from args and returns the value and remaining args.
func parseStringFlag(args []string, flag string) (string, []string) {
	longFlag := "--" + flag
	var remaining []string
	var value string
	for i := 0; i < len(args); i++ {
		if args[i] == longFlag && i+1 < len(args) {
			value = args[i+1]
			i++ // skip value
		} else {
			remaining = append(remaining, args[i])
		}
	}
	return value, remaining
}

func scriptTemplate(scriptType, scriptName, modName, baseName, className string) (string, error) {
	header := fmt.Sprintf(`/*
 * Script: %s
 * Mod:    %s
 * Type:   %s
 *
 * Custom TrinityCore server script.
 * Synced to the server via "mithril mod build".
 */

`, scriptName, modName, scriptType)

	var body string

	switch scriptType {
	case "creature":
		body = fmt.Sprintf(`#include "ScriptMgr.h"
#include "Creature.h"
#include "CreatureAI.h"
#include "Player.h"

class %s : public CreatureScript
{
public:
    %s() : CreatureScript("%s") { }

    struct %sAI : public ScriptedAI
    {
        %sAI(Creature* creature) : ScriptedAI(creature) { }

        void JustEngagedWith(Unit* /*who*/) override
        {
            // Called when the creature enters combat
        }

        void UpdateAI(uint32 diff) override
        {
            if (!UpdateVictim())
                return;

            // TODO: Add combat logic, spell timers, etc.

            DoMeleeAttackIfReady();
        }
    };

    CreatureAI* GetAI(Creature* creature) const override
    {
        return new %sAI(creature);
    }
};

void AddSC_%s()
{
    new %s();
}
`, className, className, baseName,
			className, className, className,
			baseName, className)

	case "player":
		body = fmt.Sprintf(`#include "ScriptMgr.h"
#include "Player.h"
#include "Chat.h"

class %s : public PlayerScript
{
public:
    %s() : PlayerScript("%s") { }

    // Called when a player logs in
    void OnLogin(Player* player, bool firstLogin) override
    {
        if (firstLogin)
            ChatHandler(player->GetSession()).PSendSysMessage("Welcome to the server, %%s!", player->GetName().c_str());
        else
            ChatHandler(player->GetSession()).PSendSysMessage("Welcome back, %%s!", player->GetName().c_str());
    }

    // Called when a player logs out
    // void OnLogout(Player* player) override { }

    // Called when a player kills another player
    // void OnPVPKill(Player* killer, Player* killed) override { }

    // Called when a player kills a creature
    // void OnCreatureKill(Player* killer, Creature* killed) override { }

    // Called when a player's level changes
    // void OnLevelChanged(Player* player, uint8 oldLevel) override { }

    // Called when a player switches zones
    // void OnUpdateZone(Player* player, uint32 newZone, uint32 newArea) override { }

    // Called when a player casts a spell
    // void OnSpellCast(Player* player, Spell* spell, bool skipCheck) override { }
};

void AddSC_%s()
{
    new %s();
}
`, className, className, baseName, baseName, className)

	case "spell":
		body = fmt.Sprintf(`#include "ScriptMgr.h"
#include "SpellScript.h"
#include "SpellAuraEffects.h"

// Spell script — handles spell cast events
class %s : public SpellScript
{
    PrepareSpellScript(%s);

    void HandleDummy(SpellEffIndex /*effIndex*/)
    {
        Unit* caster = GetCaster();
        Unit* target = GetHitUnit();
        if (!caster || !target)
            return;

        // TODO: Implement spell effect logic
    }

    void Register() override
    {
        OnEffectHitTarget += SpellEffectFn(%s::HandleDummy, EFFECT_0, SPELL_EFFECT_DUMMY);
    }
};

// Aura script — handles persistent aura events (optional, remove if not needed)
class %s_aura : public AuraScript
{
    PrepareAuraScript(%s_aura);

    void OnApply(AuraEffect const* /*aurEff*/, AuraEffectHandleModes /*mode*/)
    {
        // TODO: Called when the aura is applied
    }

    void OnRemove(AuraEffect const* /*aurEff*/, AuraEffectHandleModes /*mode*/)
    {
        // TODO: Called when the aura is removed
    }

    void Register() override
    {
        OnEffectApply += AuraEffectApplyFn(%s_aura::OnApply, EFFECT_0, SPELL_AURA_DUMMY, AURA_EFFECT_HANDLE_REAL);
        OnEffectRemove += AuraEffectRemoveFn(%s_aura::OnRemove, EFFECT_0, SPELL_AURA_DUMMY, AURA_EFFECT_HANDLE_REAL);
    }
};

void AddSC_%s()
{
    RegisterSpellScript(%s);
    RegisterSpellScript(%s_aura);
}
`, className, className, className,
			className, className, className, className,
			baseName, className, className)

	case "command":
		body = fmt.Sprintf(`#include "ScriptMgr.h"
#include "Chat.h"
#include "ChatCommand.h"
#include "Player.h"

using namespace Trinity::ChatCommands;

class %s : public CommandScript
{
public:
    %s() : CommandScript("%s") { }

    std::vector<ChatCommandBuilder> GetCommands() const override
    {
        static std::vector<ChatCommandBuilder> commandTable =
        {
            { "%s", HandleCommand, rbac::RBAC_PERM_COMMAND_GM, Console::No },
        };
        return commandTable;
    }

    static bool HandleCommand(ChatHandler* handler, Optional<PlayerIdentifier> target)
    {
        Player* player = handler->GetPlayer();
        if (!player)
            return false;

        // TODO: Implement your command logic here
        handler->PSendSysMessage("Hello from custom command!");
        return true;
    }
};

void AddSC_%s()
{
    new %s();
}
`, className, className, baseName, baseName, baseName, className)

	case "worldscript":
		body = fmt.Sprintf(`#include "ScriptMgr.h"
#include "Log.h"

class %s : public WorldScript
{
public:
    %s() : WorldScript("%s") { }

    // Called when the server starts up
    void OnStartup() override
    {
        TC_LOG_INFO("server.loading", "%s: Server started!");
    }

    // Called when the server shuts down
    // void OnShutdown() override { }

    // Called on every world tick (keep this lightweight!)
    // void OnUpdate(uint32 diff) override { }

    // Called after config is (re)loaded
    // void OnConfigLoad(bool reload) override { }

    // Called when a shutdown is initiated
    // void OnShutdownInitiate(ShutdownExitCode code, ShutdownMask mask) override { }
};

void AddSC_%s()
{
    new %s();
}
`, className, className, baseName, className, baseName, className)

	case "item":
		body = fmt.Sprintf(`#include "ScriptMgr.h"
#include "Item.h"
#include "Player.h"
#include "Spell.h"

class %s : public ItemScript
{
public:
    %s() : ItemScript("%s") { }

    // Called when a player uses the item
    bool OnUse(Player* player, Item* item, SpellCastTargets const& targets) override
    {
        // TODO: Implement item use logic
        // Return true to prevent default handling, false to allow it
        return false;
    }

    // Called when a player accepts a quest from the item
    // bool OnQuestAccept(Player* player, Item* item, Quest const* quest) override { return false; }

    // Called when the item expires (is destroyed)
    // bool OnExpire(Player* player, ItemTemplate const* proto) override { return false; }

    // Called when the item is removed
    // bool OnRemove(Player* player, Item* item) override { return false; }
};

void AddSC_%s()
{
    new %s();
}
`, className, className, baseName, baseName, className)

	case "gameobject":
		body = fmt.Sprintf(`#include "ScriptMgr.h"
#include "GameObjectAI.h"
#include "Player.h"

class %s : public GameObjectScript
{
public:
    %s() : GameObjectScript("%s") { }

    struct %sAI : public GameObjectAI
    {
        %sAI(GameObject* go) : GameObjectAI(go) { }

        bool OnGossipHello(Player* player) override
        {
            // TODO: Implement interaction logic
            return false;
        }

        void UpdateAI(uint32 diff) override
        {
            // TODO: Add periodic logic if needed
        }
    };

    GameObjectAI* GetAI(GameObject* go) const override
    {
        return new %sAI(go);
    }
};

void AddSC_%s()
{
    new %s();
}
`, className, className, baseName,
			className, className, className,
			baseName, className)

	case "areatrigger":
		body = fmt.Sprintf(`#include "ScriptMgr.h"
#include "Player.h"

class %s : public AreaTriggerScript
{
public:
    %s() : AreaTriggerScript("%s") { }

    bool OnTrigger(Player* player, AreaTriggerEntry const* /*trigger*/) override
    {
        // TODO: Implement area trigger logic
        // Return true to prevent default handling, false to allow it
        return false;
    }
};

void AddSC_%s()
{
    new %s();
}
`, className, className, baseName, baseName, className)

	case "unit":
		body = fmt.Sprintf(`#include "ScriptMgr.h"
#include "Unit.h"

class %s : public UnitScript
{
public:
    %s() : UnitScript("%s") { }

    // Called when a unit deals healing to another unit
    // void OnHeal(Unit* healer, Unit* receiver, uint32& gain) override { }

    // Called when a unit deals damage to another unit
    void OnDamage(Unit* attacker, Unit* victim, uint32& damage) override
    {
        // TODO: Modify damage as needed
    }

    // Called when DoT damage ticks
    // void ModifyPeriodicDamageAurasTick(Unit* target, Unit* attacker, uint32& damage) override { }

    // Called when melee damage is dealt
    // void ModifyMeleeDamage(Unit* target, Unit* attacker, uint32& damage) override { }

    // Called when spell damage is dealt
    // void ModifySpellDamageTaken(Unit* target, Unit* attacker, int32& damage) override { }
};

void AddSC_%s()
{
    new %s();
}
`, className, className, baseName, baseName, className)

	default:
		return "", fmt.Errorf("unknown script type: %s\nValid types: creature, player, spell, command, worldscript, item, gameobject, areatrigger, unit", scriptType)
	}

	return header + body, nil
}

func runModScriptRemove(args []string) error {
	modName, remaining := parseModFlag(args)
	if len(remaining) < 1 || modName == "" {
		return fmt.Errorf("usage: mithril mod script remove <name> --mod <mod_name>")
	}

	cfg := DefaultConfig()
	scriptName := remaining[0]
	if !strings.HasSuffix(scriptName, ".cpp") && !strings.HasSuffix(scriptName, ".h") {
		scriptName += ".cpp"
	}

	scriptPath := filepath.Join(cfg.ModDir(modName), "scripts", scriptName)
	if _, err := os.Stat(scriptPath); os.IsNotExist(err) {
		return fmt.Errorf("script file not found: %s", scriptPath)
	}

	if err := os.Remove(scriptPath); err != nil {
		return fmt.Errorf("remove script: %w", err)
	}

	// Clean up empty scripts directory
	scriptsDir := filepath.Join(cfg.ModDir(modName), "scripts")
	entries, _ := os.ReadDir(scriptsDir)
	if len(entries) == 0 {
		os.Remove(scriptsDir)
	}

	fmt.Printf("✓ Removed script: %s\n", scriptPath)

	// Sync and offer to rebuild if scripts changed
	changed, err := syncScriptsToContainer(cfg)
	if err != nil {
		fmt.Printf("  ⚠ Error syncing scripts: %v\n", err)
		return nil
	}
	if changed {
		fmt.Println()
		if promptYesNo("Scripts changed. Rebuild the server now?") {
			if err := serverRebuild(cfg); err != nil {
				fmt.Printf("  ⚠ Server rebuild failed: %v\n", err)
				fmt.Println("  You can retry manually with: mithril server rebuild")
			} else {
				fmt.Println()
				fmt.Println("⚠ Restart the server to load the new build:")
				fmt.Println("  mithril server restart")
			}
		}
	}
	return nil
}

func runModScriptList(args []string) error {
	modName, _ := parseModFlag(args)
	cfg := DefaultConfig()

	var mods []string
	if modName != "" {
		mods = []string{modName}
	} else {
		mods = getAllMods(cfg)
	}

	found := 0
	for _, mod := range mods {
		scripts := findModScripts(cfg, mod)
		if len(scripts) == 0 {
			continue
		}
		fmt.Printf("  %s:\n", mod)
		for _, s := range scripts {
			fmt.Printf("    %s\n", s)
		}
		found += len(scripts)
	}

	if found == 0 {
		if modName != "" {
			fmt.Printf("No scripts found in mod '%s'.\n", modName)
		} else {
			fmt.Println("No scripts found in any mod.")
		}
	}
	return nil
}

// findModScripts returns the filenames of all .cpp and .h files in a mod's scripts/ directory.
func findModScripts(cfg *Config, modName string) []string {
	scriptsDir := filepath.Join(cfg.ModDir(modName), "scripts")
	entries, err := os.ReadDir(scriptsDir)
	if err != nil {
		return nil
	}

	var scripts []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		ext := strings.ToLower(filepath.Ext(name))
		if ext == ".cpp" || ext == ".h" {
			scripts = append(scripts, name)
		}
	}
	return scripts
}

// scriptDesired describes a script file that should be in the container.
type scriptDesired struct {
	mod           string
	file          string
	containerFile string
	srcPath       string
	checksum      string
}

// countAllScripts returns the total number of script files across all mods.
func countAllScripts(cfg *Config) int {
	total := 0
	for _, mod := range getAllMods(cfg) {
		total += len(findModScripts(cfg, mod))
	}
	return total
}

// ── scripts_applied.json tracker ──

// ScriptTracker records which scripts have been synced to the container.
type ScriptTracker struct {
	Scripts []AppliedScript `json:"scripts"`
}

// AppliedScript tracks a single script file synced to the container.
type AppliedScript struct {
	Mod           string `json:"mod"`
	File          string `json:"file"`
	ContainerFile string `json:"container_file"` // filename inside /Custom
	Checksum      string `json:"checksum"`       // MD5 of the source file
}

func loadScriptTracker(cfg *Config) (*ScriptTracker, error) {
	path := filepath.Join(cfg.ModulesDir, "scripts_applied.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &ScriptTracker{}, nil
		}
		return nil, err
	}
	var t ScriptTracker
	if err := json.Unmarshal(data, &t); err != nil {
		return nil, err
	}
	return &t, nil
}

func saveScriptTracker(cfg *Config, t *ScriptTracker) error {
	path := filepath.Join(cfg.ModulesDir, "scripts_applied.json")
	data, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// fileChecksum returns the hex-encoded MD5 of a file's contents.
func fileChecksum(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	h := md5.Sum(data)
	return hex.EncodeToString(h[:])
}

// syncScriptsToContainer compares mod scripts against the tracker, then
// docker-cp's only changed/new files into the running container and removes
// files that no longer exist. Returns true if any changes were made.
func syncScriptsToContainer(cfg *Config) (changed bool, err error) {
	containerID, err := composeContainerID(cfg)
	if err != nil || containerID == "" {
		return false, fmt.Errorf("server container is not running — start it with 'mithril server start'")
	}

	tracker, err := loadScriptTracker(cfg)
	if err != nil {
		return false, fmt.Errorf("load script tracker: %w", err)
	}

	// Build the desired state: all scripts from all mods
	var want []scriptDesired

	mods := getAllMods(cfg)
	for _, mod := range mods {
		scripts := findModScripts(cfg, mod)
		srcDir := filepath.Join(cfg.ModDir(mod), "scripts")
		for _, script := range scripts {
			srcPath := filepath.Join(srcDir, script)
			containerFile := mod + "_" + script
			want = append(want, scriptDesired{
				mod:           mod,
				file:          script,
				containerFile: containerFile,
				srcPath:       srcPath,
				checksum:      fileChecksum(srcPath),
			})
		}
	}

	// Index current tracker state by container filename
	applied := make(map[string]AppliedScript)
	for _, s := range tracker.Scripts {
		applied[s.ContainerFile] = s
	}

	// Determine what to add/update and what to remove
	var toSync []scriptDesired
	wantSet := make(map[string]bool)

	for _, w := range want {
		wantSet[w.containerFile] = true
		existing, exists := applied[w.containerFile]
		if !exists || existing.Checksum != w.checksum {
			toSync = append(toSync, w)
		}
	}

	var toRemove []AppliedScript
	for _, s := range tracker.Scripts {
		if !wantSet[s.ContainerFile] {
			toRemove = append(toRemove, s)
		}
	}

	if len(toSync) == 0 && len(toRemove) == 0 {
		// Even if no script files changed, ensure the loader exists
		if err := generateCustomScriptLoader(cfg, containerID, want); err != nil {
			return false, fmt.Errorf("generate script loader: %w", err)
		}
		return false, nil
	}

	// Copy changed/new files into the container
	for _, w := range toSync {
		containerPath := containerCustomScriptsDir + "/" + w.containerFile
		fmt.Printf("  → syncing %s/%s\n", w.mod, w.file)
		cmd := exec.Command("docker", "cp", w.srcPath, containerID+":"+containerPath)
		if output, err := cmd.CombinedOutput(); err != nil {
			return true, fmt.Errorf("docker cp %s: %s — %w", w.file, strings.TrimSpace(string(output)), err)
		}
	}

	// Remove deleted files from the container
	for _, s := range toRemove {
		containerPath := containerCustomScriptsDir + "/" + s.ContainerFile
		fmt.Printf("  ✕ removing %s/%s\n", s.Mod, s.File)
		cmd := exec.Command("docker", "exec", containerID, "rm", "-f", containerPath)
		cmd.CombinedOutput() // best-effort
	}

	// Update tracker
	var newScripts []AppliedScript
	for _, w := range want {
		newScripts = append(newScripts, AppliedScript{
			Mod:           w.mod,
			File:          w.file,
			ContainerFile: w.containerFile,
			Checksum:      w.checksum,
		})
	}
	tracker.Scripts = newScripts
	if err := saveScriptTracker(cfg, tracker); err != nil {
		return true, fmt.Errorf("save script tracker: %w", err)
	}

	// Generate the custom_script_loader.cpp in the container
	if err := generateCustomScriptLoader(cfg, containerID, want); err != nil {
		return true, fmt.Errorf("generate script loader: %w", err)
	}

	return true, nil
}

// generateCustomScriptLoader creates a custom_script_loader.cpp inside the
// container that declares and calls all AddSC_* functions from the synced scripts.
// This is required by TrinityCore's build system — it calls AddCustomScripts()
// which must invoke each script's registration function.
func generateCustomScriptLoader(cfg *Config, containerID string, scripts []scriptDesired) error {
	// Extract AddSC_ function names from each .cpp file
	var addSCFuncs []string
	for _, s := range scripts {
		if !strings.HasSuffix(s.file, ".cpp") {
			continue
		}
		// Read the file to find AddSC_ declarations
		data, err := os.ReadFile(s.srcPath)
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "void AddSC_") && strings.Contains(line, "(") {
				// Extract function name: "void AddSC_foo()" -> "AddSC_foo"
				funcName := strings.TrimPrefix(line, "void ")
				if idx := strings.Index(funcName, "("); idx > 0 {
					funcName = funcName[:idx]
				}
				addSCFuncs = append(addSCFuncs, funcName)
			}
		}
	}

	// Build the loader source
	var sb strings.Builder
	sb.WriteString("// Auto-generated by mithril — do not edit manually\n\n")
	for _, fn := range addSCFuncs {
		sb.WriteString(fmt.Sprintf("void %s();\n", fn))
	}
	sb.WriteString("\nvoid AddCustomScripts()\n{\n")
	for _, fn := range addSCFuncs {
		sb.WriteString(fmt.Sprintf("    %s();\n", fn))
	}
	sb.WriteString("}\n")

	loaderContent := sb.String()

	// Write to a temp file and docker cp it into the container
	tmpFile := filepath.Join(os.TempDir(), "tmp_rovodev_custom_script_loader.cpp")
	if err := os.WriteFile(tmpFile, []byte(loaderContent), 0644); err != nil {
		return fmt.Errorf("write temp loader: %w", err)
	}
	defer os.Remove(tmpFile)

	containerPath := containerCustomScriptsDir + "/custom_script_loader.cpp"
	cmd := exec.Command("docker", "cp", tmpFile, containerID+":"+containerPath)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("docker cp loader: %s — %w", strings.TrimSpace(string(output)), err)
	}

	return nil
}

// snakeToPascal converts a snake_case string to PascalCase.
func snakeToPascal(s string) string {
	parts := strings.Split(s, "_")
	for i, p := range parts {
		if len(p) > 0 {
			parts[i] = strings.ToUpper(p[:1]) + p[1:]
		}
	}
	return strings.Join(parts, "")
}
