// Package help provides structured help data and rendering for paw-proxy CLIs.
// Help content is defined once and consumed by both --help output and man page generation.
package help

import (
	"fmt"
	"io"
	"strings"
)

// Command describes a CLI binary's help content.
type Command struct {
	Name        string
	Version     string // injected at runtime
	Summary     string // one-line description
	Description string // longer description (optional)
	Usage       string
	Flags       []Flag
	EnvVars     []EnvVar
	Examples    []Example
	Subcommands []Subcommand
	Files       []FilePath
	SeeAlso     []string
}

// Subcommand describes a subcommand of a binary.
type Subcommand struct {
	Name        string
	Summary     string
	Usage       string
	Flags       []Flag
	RequiresRoot bool
}

// Flag describes a CLI flag.
type Flag struct {
	Short string // e.g. "-f"
	Long  string // e.g. "--tail"
	Arg   string // e.g. "name" (empty for boolean flags)
	Desc  string
}

// EnvVar describes an environment variable.
type EnvVar struct {
	Name string
	Desc string
}

// Example describes a usage example.
type Example struct {
	Command string
	Desc    string
}

// FilePath describes a notable file path.
type FilePath struct {
	Path string
	Desc string
}

// Render writes the full help text for a top-level command to w.
func (c *Command) Render(w io.Writer) {
	// Header
	if c.Version != "" {
		fmt.Fprintf(w, "%s version %s\n", c.Name, c.Version)
	}
	fmt.Fprintf(w, "%s\n\n", c.Summary)

	// Usage
	fmt.Fprintf(w, "Usage:\n  %s\n\n", c.Usage)

	// Subcommands
	if len(c.Subcommands) > 0 {
		fmt.Fprintln(w, "Commands:")
		width := subcommandWidth(c.Subcommands)
		for _, sc := range c.Subcommands {
			pad := strings.Repeat(" ", width-len(sc.Name))
			fmt.Fprintf(w, "  %s%s  %s\n", sc.Name, pad, sc.Summary)
		}
		fmt.Fprintln(w)
	}

	// Flags
	if len(c.Flags) > 0 {
		fmt.Fprintln(w, "Options:")
		renderFlags(w, c.Flags)
		fmt.Fprintln(w)
	}

	// Env vars
	if len(c.EnvVars) > 0 {
		fmt.Fprintln(w, "Environment variables (set for child process):")
		width := envVarWidth(c.EnvVars)
		for _, ev := range c.EnvVars {
			pad := strings.Repeat(" ", width-len(ev.Name))
			fmt.Fprintf(w, "  %s%s  %s\n", ev.Name, pad, ev.Desc)
		}
		fmt.Fprintln(w)
	}

	// Examples
	if len(c.Examples) > 0 {
		fmt.Fprintln(w, "Examples:")
		for _, ex := range c.Examples {
			fmt.Fprintf(w, "  %s\n", ex.Command)
			if ex.Desc != "" {
				fmt.Fprintf(w, "      %s\n", ex.Desc)
			}
		}
		fmt.Fprintln(w)
	}

	// Subcommand help hint
	if len(c.Subcommands) > 0 {
		fmt.Fprintf(w, "Use \"%s <command> --help\" for more information about a command.\n", c.Name)
	}

	// Related commands (from SeeAlso for terminal help) — only for binaries with subcommands
	if len(c.SeeAlso) > 0 && len(c.Subcommands) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "Related commands:")
		for _, ref := range c.SeeAlso {
			// Strip man page section number: "up(1)" -> "up"
			cmdName := strings.Split(ref, "(")[0]
			fmt.Fprintf(w, "  %s\n", cmdName)
		}
	}
}

// RenderSubcommand writes the help text for a specific subcommand to w.
// Returns false if the subcommand was not found.
func (c *Command) RenderSubcommand(w io.Writer, name string) bool {
	var sc *Subcommand
	for i := range c.Subcommands {
		if c.Subcommands[i].Name == name {
			sc = &c.Subcommands[i]
			break
		}
	}
	if sc == nil {
		return false
	}

	fmt.Fprintf(w, "%s %s -- %s\n\n", c.Name, sc.Name, sc.Summary)

	usage := sc.Usage
	if usage == "" {
		usage = fmt.Sprintf("%s %s", c.Name, sc.Name)
	}
	fmt.Fprintf(w, "Usage:\n  %s\n", usage)

	if sc.RequiresRoot {
		fmt.Fprintln(w, "  (requires sudo)")
	}
	fmt.Fprintln(w)

	if len(sc.Flags) > 0 {
		fmt.Fprintln(w, "Options:")
		renderFlags(w, sc.Flags)
		fmt.Fprintln(w)
	}

	return true
}

// renderFlags writes a formatted flag list to w.
func renderFlags(w io.Writer, flags []Flag) {
	width := flagWidth(flags)
	for _, f := range flags {
		label := flagLabel(f)
		pad := strings.Repeat(" ", width-len(label))
		fmt.Fprintf(w, "  %s%s  %s\n", label, pad, f.Desc)
	}
}

// flagLabel returns the display string for a flag, e.g. "-n, --name <name>" or "--restart".
func flagLabel(f Flag) string {
	var parts []string
	if f.Short != "" {
		parts = append(parts, f.Short)
	}
	if f.Long != "" {
		parts = append(parts, f.Long)
	}
	label := strings.Join(parts, ", ")
	if f.Arg != "" {
		label += " <" + f.Arg + ">"
	}
	return label
}

func flagWidth(flags []Flag) int {
	max := 0
	for _, f := range flags {
		if n := len(flagLabel(f)); n > max {
			max = n
		}
	}
	return max
}

func subcommandWidth(subs []Subcommand) int {
	max := 0
	for _, s := range subs {
		if len(s.Name) > max {
			max = len(s.Name)
		}
	}
	return max
}

func envVarWidth(evs []EnvVar) int {
	max := 0
	for _, ev := range evs {
		if len(ev.Name) > max {
			max = len(ev.Name)
		}
	}
	return max
}
