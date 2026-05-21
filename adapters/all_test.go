package adapters

import (
	"testing"

	"github.com/danieljbfz/chronicle/internal/config"
	"github.com/danieljbfz/chronicle/internal/paths"
)

// TestAll_returnsRegisteredFactories pins down the set of providers
// chronicle knows about. If anyone adds or removes a factory in
// all.go without updating this test, the test fails and forces a
// conscious decision. The test is intentionally precise about the
// expected count, because silently growing or shrinking the
// provider set should never happen without someone noticing.
func TestAll_returnsRegisteredFactories(t *testing.T) {
	factories := All()
	if len(factories) != 3 {
		t.Errorf("registered factories = %d, want 3 (claude + copilot-chat + copilot-agent)", len(factories))
	}
}

// withProvider returns a copy of the defaults with one
// provider's config field overridden. We use a copy so test
// cases do not mutate the package-level defaults map (which
// would leak state between tests). The helper keeps the
// per-test assignments below readable, so each test's first
// line shows what it changes from the baseline.
func withProvider(name string, override config.ProviderConfig) config.Config {
	settings := config.Defaults()
	out := make(map[string]config.ProviderConfig, len(settings.Providers))
	for k, v := range settings.Providers {
		out[k] = v
	}
	out[name] = override
	settings.Providers = out
	return settings
}

// TestClaudeFactory_disabledReturnsNothing confirms the Enabled
// flag actually disables the provider. A user who turns off the
// Claude adapter in their config should see no Claude entry,
// regardless of whether ~/.claude exists.
func TestClaudeFactory_disabledReturnsNothing(t *testing.T) {
	settings := withProvider(config.ProviderClaude, config.ProviderConfig{Enabled: false})
	entries := claudeFactory(settings, paths.Locations{ClaudeRoot: "/tmp/anywhere"})
	if entries != nil {
		t.Errorf("disabled Claude factory returned %d entries, want nil", len(entries))
	}
}

// TestClaudeFactory_usesConfigRootWhenSet confirms the config
// override takes priority over the default location. Users who
// keep their data on an external drive depend on this path being
// honoured.
func TestClaudeFactory_usesConfigRootWhenSet(t *testing.T) {
	settings := withProvider(config.ProviderClaude, config.ProviderConfig{
		Enabled: true,
		Root:    "/custom/claude/location",
	})
	entries := claudeFactory(settings, paths.Locations{ClaudeRoot: "/default/location"})
	if len(entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(entries))
	}
	if entries[0].Root != "/custom/claude/location" {
		t.Errorf("Root = %q, want the config value", entries[0].Root)
	}
}

// TestClaudeFactory_fallsBackToDefaultRoot confirms the inverse:
// when config does not set a root, we use the default from the
// paths package. That is what most users will hit, because they
// never touch the config file.
func TestClaudeFactory_fallsBackToDefaultRoot(t *testing.T) {
	settings := withProvider(config.ProviderClaude, config.ProviderConfig{Enabled: true})
	entries := claudeFactory(settings, paths.Locations{ClaudeRoot: "/default/.claude"})
	if len(entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(entries))
	}
	if entries[0].Root != "/default/.claude" {
		t.Errorf("Root = %q, want the default", entries[0].Root)
	}
}

// TestCopilotChatFactory_disabledReturnsNothing mirrors the Claude
// case for symmetry. Users who only run one provider deserve a
// clean disable switch for the other.
func TestCopilotChatFactory_disabledReturnsNothing(t *testing.T) {
	settings := withProvider(config.ProviderCopilotChat, config.ProviderConfig{Enabled: false})
	entries := copilotChatFactory(settings, paths.Locations{CopilotChatRoots: []string{"/tmp"}})
	if entries != nil {
		t.Errorf("disabled Copilot factory returned %d entries, want nil", len(entries))
	}
}

// TestCopilotChatFactory_skipsMissingRoots is the key edge case. The
// default Copilot roots cover both VS Code and VS Code Insiders.
// Most machines have only one of the two, so the factory must
// silently skip roots that do not exist on disk. Otherwise the
// doctor view would show a noisy "directory not found" warning
// for every absent install.
func TestCopilotChatFactory_skipsMissingRoots(t *testing.T) {
	settings := config.Defaults()
	entries := copilotChatFactory(settings, paths.Locations{
		CopilotChatRoots: []string{
			"/does/not/exist/at/all",
			"/also/does/not/exist",
		},
	})
	if len(entries) != 0 {
		t.Errorf("entries = %d, want 0 when no roots exist", len(entries))
	}
}

// TestCopilotChatFactory_findsExistingRoot confirms the inverse: when
// at least one root exists, the factory returns an Entry for it.
// We use the test's own temp directory as a real path on disk.
func TestCopilotChatFactory_findsExistingRoot(t *testing.T) {
	real := t.TempDir()
	settings := config.Defaults()
	entries := copilotChatFactory(settings, paths.Locations{
		CopilotChatRoots: []string{real, "/missing/path"},
	})
	if len(entries) != 1 {
		t.Fatalf("entries = %d, want 1 (only the real path)", len(entries))
	}
	if entries[0].Root != real {
		t.Errorf("Root = %q, want %q", entries[0].Root, real)
	}
}

// TestCopilotAgentFactory_disabledReturnsNothing covers the
// disable switch for the third provider, matching the Claude
// and Copilot Chat cases. A user who never runs the Copilot
// agent runtime should see no agent entry.
func TestCopilotAgentFactory_disabledReturnsNothing(t *testing.T) {
	settings := withProvider(config.ProviderCopilotAgent, config.ProviderConfig{Enabled: false})
	entries := copilotAgentFactory(settings, paths.Locations{CopilotAgentRoot: "/tmp/anywhere"})
	if entries != nil {
		t.Errorf("disabled Copilot agent factory returned %d entries, want nil", len(entries))
	}
}

// TestCopilotAgentFactory_skipsMissingRoot confirms the
// factory returns nothing when the agent's session-state
// directory is absent. The agent runtime is newer and in
// public preview, so most installs will not have ~/.copilot
// on disk, and the factory must stay quiet rather than
// surface a missing-directory entry.
func TestCopilotAgentFactory_skipsMissingRoot(t *testing.T) {
	settings := config.Defaults()
	entries := copilotAgentFactory(settings, paths.Locations{CopilotAgentRoot: "/does/not/exist/at/all"})
	if len(entries) != 0 {
		t.Errorf("entries = %d, want 0 when the root is absent", len(entries))
	}
}

// TestCopilotAgentFactory_findsExistingRoot confirms the
// inverse: when the root exists on disk, the factory returns
// exactly one Entry pointed at it. We use the test's own temp
// directory as a real path.
func TestCopilotAgentFactory_findsExistingRoot(t *testing.T) {
	real := t.TempDir()
	settings := config.Defaults()
	entries := copilotAgentFactory(settings, paths.Locations{CopilotAgentRoot: real})
	if len(entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(entries))
	}
	if entries[0].Root != real {
		t.Errorf("Root = %q, want %q", entries[0].Root, real)
	}
}
