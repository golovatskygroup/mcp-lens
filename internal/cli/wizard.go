// Package cli provides interactive CLI tools for mcp-lens configuration.
package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/golovatskygroup/mcp-lens/internal/presets"
	"golang.org/x/term"
)

// Wizard provides interactive configuration for MCP presets.
type Wizard struct {
	registry *presets.Registry
	in       io.Reader
	out      io.Writer
	scanner  *bufio.Scanner
	// For testing: allows overriding terminal detection
	isTTY bool
}

// NewWizard creates a new wizard instance.
func NewWizard() *Wizard {
	return &Wizard{
		registry: presets.NewRegistry(),
		in:       os.Stdin,
		out:      os.Stdout,
		scanner:  bufio.NewScanner(os.Stdin),
		isTTY:    term.IsTerminal(int(os.Stdin.Fd())),
	}
}

// NewWizardWithIO creates a wizard with custom IO (for testing).
func NewWizardWithIO(in io.Reader, out io.Writer) *Wizard {
	return &Wizard{
		registry: presets.NewRegistry(),
		in:       in,
		out:      out,
		scanner:  bufio.NewScanner(in),
		isTTY:    false, // Custom IO is not a TTY
	}
}

// Run starts the interactive setup wizard.
func (w *Wizard) Run() error {
	w.printHeader()

	// Step 1: Select category
	cat, err := w.selectCategory()
	if err != nil {
		return err
	}

	// Step 2: Select preset from category
	preset, err := w.selectPreset(cat)
	if err != nil {
		return err
	}

	// Step 3: Configure required env vars
	envVars, err := w.configureEnvVars(preset)
	if err != nil {
		return err
	}

	// Step 4: Output configuration
	w.printConfiguration(preset, envVars)

	return nil
}

func (w *Wizard) printHeader() {
	fmt.Fprintln(w.out)
	fmt.Fprintln(w.out, "🔧 MCP Lens Server Setup Wizard")
	fmt.Fprintln(w.out)
}

func (w *Wizard) selectCategory() (presets.Category, error) {
	cats := presets.SortedCategories()
	catInfo := presets.Categories()

	fmt.Fprintln(w.out, "Available preset categories:")
	for i, cat := range cats {
		info := catInfo[cat]
		fmt.Fprintf(w.out, "  %d. %s - %s\n", i+1, info.Name, info.Description)
	}
	fmt.Fprintln(w.out)

	for {
		fmt.Fprintf(w.out, "Select category [1-%d]: ", len(cats))
		input, err := w.readLine()
		if err != nil {
			return "", err
		}

		num, err := strconv.Atoi(strings.TrimSpace(input))
		if err != nil || num < 1 || num > len(cats) {
			fmt.Fprintln(w.out, "Invalid selection, please try again.")
			continue
		}

		return cats[num-1], nil
	}
}

func (w *Wizard) selectPreset(cat presets.Category) (presets.PresetMeta, error) {
	catPresets := w.registry.GetByCategory(cat)
	catInfo := presets.Categories()[cat]

	fmt.Fprintln(w.out)
	fmt.Fprintf(w.out, "Available presets in %s:\n", catInfo.Name)
	for i, p := range catPresets {
		fmt.Fprintf(w.out, "  %d. %s - %s\n", i+1, p.Name, p.Description)
	}
	fmt.Fprintln(w.out)

	for {
		fmt.Fprintf(w.out, "Select preset [1-%d]: ", len(catPresets))
		input, err := w.readLine()
		if err != nil {
			return presets.PresetMeta{}, err
		}

		num, err := strconv.Atoi(strings.TrimSpace(input))
		if err != nil || num < 1 || num > len(catPresets) {
			fmt.Fprintln(w.out, "Invalid selection, please try again.")
			continue
		}

		return catPresets[num-1], nil
	}
}

func (w *Wizard) configureEnvVars(preset presets.PresetMeta) (map[string]string, error) {
	envVars := make(map[string]string)

	fmt.Fprintln(w.out)
	fmt.Fprintf(w.out, "Configuring: %s\n", preset.Name)
	fmt.Fprintf(w.out, "%s\n", preset.Description)
	if preset.DocsURL != "" {
		fmt.Fprintf(w.out, "📖 Docs: %s\n", preset.DocsURL)
	}
	fmt.Fprintln(w.out)

	// Required env vars
	if len(preset.RequiredEnv) > 0 {
		fmt.Fprintln(w.out, "Required configuration:")
		for _, env := range preset.RequiredEnv {
			value, err := w.promptEnvVar(env, true)
			if err != nil {
				return nil, err
			}
			envVars[env.Name] = value
		}
	}

	// Optional env vars
	if len(preset.OptionalEnv) > 0 {
		fmt.Fprintln(w.out)
		fmt.Fprintln(w.out, "Optional configuration (press Enter to skip):")
		for _, env := range preset.OptionalEnv {
			value, err := w.promptEnvVar(env, false)
			if err != nil {
				return nil, err
			}
			if value != "" {
				envVars[env.Name] = value
			}
		}
	}

	return envVars, nil
}

func (w *Wizard) promptEnvVar(env presets.EnvVar, required bool) (string, error) {
	desc := env.Description
	if env.Example != "" {
		desc += fmt.Sprintf(" (e.g., %s)", env.Example)
	}
	fmt.Fprintf(w.out, "  %s: %s\n", env.Name, desc)

	for {
		fmt.Fprintf(w.out, "  → Enter value")
		if env.Secret {
			fmt.Fprint(w.out, " (hidden)")
		}
		fmt.Fprint(w.out, ": ")

		var value string
		var err error

		if env.Secret && w.isTTY {
			// Read password without echo
			byteValue, readErr := term.ReadPassword(int(os.Stdin.Fd()))
			fmt.Fprintln(w.out) // New line after hidden input
			if readErr != nil {
				return "", readErr
			}
			value = string(byteValue)
		} else {
			value, err = w.readLine()
			if err != nil {
				return "", err
			}
		}

		value = strings.TrimSpace(value)

		if required && value == "" {
			fmt.Fprintln(w.out, "  ⚠️  This field is required. Please enter a value.")
			continue
		}

		return value, nil
	}
}

func (w *Wizard) printConfiguration(preset presets.PresetMeta, envVars map[string]string) {
	fmt.Fprintln(w.out)
	fmt.Fprintln(w.out, "✅ Configuration complete!")
	fmt.Fprintln(w.out)

	// Shell export commands
	fmt.Fprintln(w.out, "# Add to your shell profile or .env file:")
	fmt.Fprintf(w.out, "export MCP_LENS_PRESET=\"%s\"\n", preset.Name)
	for name, value := range envVars {
		// Mask secrets in output
		displayValue := value
		if w.isSecretEnv(preset, name) {
			if len(value) > 8 {
				displayValue = value[:4] + "..." + value[len(value)-4:]
			} else {
				displayValue = "****"
			}
		}
		fmt.Fprintf(w.out, "export %s=\"%s\"\n", name, displayValue)
	}

	fmt.Fprintln(w.out)
	fmt.Fprintln(w.out, "# Or use config.yaml:")
	fmt.Fprintln(w.out, "upstream:")
	fmt.Fprintf(w.out, "  preset: %s\n", preset.Name)
	if len(envVars) > 0 {
		fmt.Fprintln(w.out, "  env:")
		for name, value := range envVars {
			displayValue := value
			if w.isSecretEnv(preset, name) {
				if len(value) > 8 {
					displayValue = value[:4] + "..." + value[len(value)-4:]
				} else {
					displayValue = "****"
				}
			}
			fmt.Fprintf(w.out, "    %s: \"%s\"\n", name, displayValue)
		}
	}

	fmt.Fprintln(w.out)
	fmt.Fprintln(w.out, "# Run with:")
	fmt.Fprintln(w.out, "mcp-lens --config config.yaml")
}

func (w *Wizard) isSecretEnv(preset presets.PresetMeta, name string) bool {
	for _, env := range preset.RequiredEnv {
		if env.Name == name && env.Secret {
			return true
		}
	}
	for _, env := range preset.OptionalEnv {
		if env.Name == name && env.Secret {
			return true
		}
	}
	return false
}

func (w *Wizard) readLine() (string, error) {
	if w.scanner.Scan() {
		return w.scanner.Text(), nil
	}
	if err := w.scanner.Err(); err != nil {
		return "", err
	}
	return "", io.EOF
}

// ListPresets prints all available presets grouped by category.
func ListPresets(out io.Writer) {
	reg := presets.NewRegistry()
	cats := presets.SortedCategories()
	catInfo := presets.Categories()

	fmt.Fprintln(out, "Available MCP Server Presets:")
	fmt.Fprintln(out)

	for _, cat := range cats {
		info := catInfo[cat]
		catPresets := reg.GetByCategory(cat)

		fmt.Fprintf(out, "📂 %s (%s)\n", info.Name, info.Description)
		for _, p := range catPresets {
			reqVars := ""
			if len(p.RequiredEnv) > 0 {
				vars := make([]string, len(p.RequiredEnv))
				for i, v := range p.RequiredEnv {
					vars[i] = v.Name
				}
				reqVars = fmt.Sprintf(" [requires: %s]", strings.Join(vars, ", "))
			}
			fmt.Fprintf(out, "   • %s - %s%s\n", p.Name, p.Description, reqVars)
		}
		fmt.Fprintln(out)
	}
}

// CheckServerAvailability checks if required tools are available.
func CheckServerAvailability(presetName string) (*AvailabilityResult, error) {
	reg := presets.NewRegistry()
	meta, ok := reg.GetMeta(presetName)
	if !ok {
		return nil, fmt.Errorf("unknown preset: %s", presetName)
	}

	result := &AvailabilityResult{
		Preset:  presetName,
		Command: meta.Config.Command,
	}

	// Check if command is available
	cmd := exec.Command("which", meta.Config.Command)
	if err := cmd.Run(); err != nil {
		result.CommandAvailable = false
		result.CommandError = fmt.Sprintf("command '%s' not found in PATH", meta.Config.Command)
	} else {
		result.CommandAvailable = true
	}

	// Check required env vars
	for _, env := range meta.RequiredEnv {
		envResult := EnvVarCheck{
			Name:     env.Name,
			Required: true,
		}
		if val := os.Getenv(env.Name); val != "" {
			envResult.Present = true
		}
		result.EnvVars = append(result.EnvVars, envResult)
	}

	// Check optional env vars
	for _, env := range meta.OptionalEnv {
		envResult := EnvVarCheck{
			Name:     env.Name,
			Required: false,
		}
		if val := os.Getenv(env.Name); val != "" {
			envResult.Present = true
		}
		result.EnvVars = append(result.EnvVars, envResult)
	}

	return result, nil
}

// AvailabilityResult contains server availability check results.
type AvailabilityResult struct {
	Preset           string
	Command          string
	CommandAvailable bool
	CommandError     string
	EnvVars          []EnvVarCheck
}

// EnvVarCheck represents an environment variable check result.
type EnvVarCheck struct {
	Name     string
	Required bool
	Present  bool
}

// IsReady returns true if the preset is ready to use.
func (r *AvailabilityResult) IsReady() bool {
	if !r.CommandAvailable {
		return false
	}
	for _, env := range r.EnvVars {
		if env.Required && !env.Present {
			return false
		}
	}
	return true
}

// PrintResult outputs the availability check result.
func (r *AvailabilityResult) PrintResult(out io.Writer) {
	fmt.Fprintf(out, "🔍 Checking preset: %s\n\n", r.Preset)

	// Command check
	if r.CommandAvailable {
		fmt.Fprintf(out, "✅ Command '%s' is available\n", r.Command)
	} else {
		fmt.Fprintf(out, "❌ Command '%s': %s\n", r.Command, r.CommandError)
	}

	// Env var checks
	if len(r.EnvVars) > 0 {
		fmt.Fprintln(out, "\nEnvironment variables:")
		for _, env := range r.EnvVars {
			status := "✅"
			if !env.Present {
				if env.Required {
					status = "❌"
				} else {
					status = "⚪"
				}
			}
			reqLabel := ""
			if env.Required {
				reqLabel = " (required)"
			} else {
				reqLabel = " (optional)"
			}
			fmt.Fprintf(out, "  %s %s%s\n", status, env.Name, reqLabel)
		}
	}

	fmt.Fprintln(out)
	if r.IsReady() {
		fmt.Fprintln(out, "✅ Preset is ready to use!")
	} else {
		fmt.Fprintln(out, "⚠️  Preset is not ready. Please configure missing items.")
	}
}
