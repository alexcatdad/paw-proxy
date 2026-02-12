package help

import (
	"bytes"
	"strings"
	"testing"
)

func TestRender_IncludesVersion(t *testing.T) {
	cmd := Command{
		Name:    "testcmd",
		Version: "1.2.3",
		Summary: "A test command",
		Usage:   "testcmd [options]",
	}
	var buf bytes.Buffer
	cmd.Render(&buf)
	out := buf.String()
	if !strings.Contains(out, "testcmd version 1.2.3") {
		t.Errorf("expected version in output, got:\n%s", out)
	}
}

func TestRender_OmitsVersionWhenEmpty(t *testing.T) {
	cmd := Command{
		Name:    "testcmd",
		Summary: "A test command",
		Usage:   "testcmd [options]",
	}
	var buf bytes.Buffer
	cmd.Render(&buf)
	out := buf.String()
	if strings.Contains(out, "version") {
		t.Errorf("expected no version line, got:\n%s", out)
	}
}

func TestRender_IncludesSubcommands(t *testing.T) {
	cmd := Command{
		Name:    "app",
		Summary: "An app",
		Usage:   "app <command>",
		Subcommands: []Subcommand{
			{Name: "start", Summary: "Start the service"},
			{Name: "stop", Summary: "Stop the service"},
		},
	}
	var buf bytes.Buffer
	cmd.Render(&buf)
	out := buf.String()

	if !strings.Contains(out, "Commands:") {
		t.Error("expected Commands: section")
	}
	if !strings.Contains(out, "start") || !strings.Contains(out, "Start the service") {
		t.Error("expected start subcommand")
	}
	if !strings.Contains(out, "stop") || !strings.Contains(out, "Stop the service") {
		t.Error("expected stop subcommand")
	}
	if !strings.Contains(out, `"app <command> --help"`) {
		t.Errorf("expected subcommand help hint, got:\n%s", out)
	}
}

func TestRender_IncludesFlags(t *testing.T) {
	cmd := Command{
		Name:    "app",
		Summary: "An app",
		Usage:   "app [options]",
		Flags: []Flag{
			{Short: "-n", Arg: "name", Desc: "Set the name"},
			{Long: "--verbose", Desc: "Enable verbose output"},
		},
	}
	var buf bytes.Buffer
	cmd.Render(&buf)
	out := buf.String()

	if !strings.Contains(out, "Options:") {
		t.Error("expected Options: section")
	}
	if !strings.Contains(out, "-n <name>") {
		t.Errorf("expected flag with arg, got:\n%s", out)
	}
	if !strings.Contains(out, "--verbose") {
		t.Error("expected verbose flag")
	}
}

func TestRender_IncludesEnvVars(t *testing.T) {
	cmd := Command{
		Name:    "app",
		Summary: "An app",
		Usage:   "app",
		EnvVars: []EnvVar{
			{Name: "PORT", Desc: "The port number"},
		},
	}
	var buf bytes.Buffer
	cmd.Render(&buf)
	out := buf.String()

	if !strings.Contains(out, "Environment variables") {
		t.Error("expected env vars section")
	}
	if !strings.Contains(out, "PORT") || !strings.Contains(out, "The port number") {
		t.Error("expected PORT env var")
	}
}

func TestRender_IncludesExamples(t *testing.T) {
	cmd := Command{
		Name:    "app",
		Summary: "An app",
		Usage:   "app",
		Examples: []Example{
			{Command: "app start", Desc: "Start the app"},
		},
	}
	var buf bytes.Buffer
	cmd.Render(&buf)
	out := buf.String()

	if !strings.Contains(out, "Examples:") {
		t.Error("expected Examples: section")
	}
	if !strings.Contains(out, "app start") {
		t.Error("expected example command")
	}
	if !strings.Contains(out, "Start the app") {
		t.Error("expected example description")
	}
}

func TestRenderSubcommand_Found(t *testing.T) {
	cmd := Command{
		Name:    "app",
		Summary: "An app",
		Subcommands: []Subcommand{
			{
				Name:    "logs",
				Summary: "Show logs",
				Usage:   "app logs [--tail]",
				Flags: []Flag{
					{Short: "-f", Long: "--tail", Desc: "Follow output"},
				},
			},
		},
	}
	var buf bytes.Buffer
	found := cmd.RenderSubcommand(&buf, "logs")
	if !found {
		t.Fatal("expected subcommand to be found")
	}
	out := buf.String()

	if !strings.Contains(out, "app logs -- Show logs") {
		t.Errorf("expected subcommand header, got:\n%s", out)
	}
	if !strings.Contains(out, "app logs [--tail]") {
		t.Error("expected usage line")
	}
	if !strings.Contains(out, "-f, --tail") {
		t.Errorf("expected flag, got:\n%s", out)
	}
}

func TestRenderSubcommand_NotFound(t *testing.T) {
	cmd := Command{
		Name:    "app",
		Summary: "An app",
		Subcommands: []Subcommand{
			{Name: "start", Summary: "Start"},
		},
	}
	var buf bytes.Buffer
	found := cmd.RenderSubcommand(&buf, "nonexistent")
	if found {
		t.Error("expected subcommand to not be found")
	}
	if buf.Len() != 0 {
		t.Errorf("expected no output for missing subcommand, got: %s", buf.String())
	}
}

func TestRenderSubcommand_ShowsRequiresRoot(t *testing.T) {
	cmd := Command{
		Name: "app",
		Subcommands: []Subcommand{
			{Name: "setup", Summary: "Setup", RequiresRoot: true},
		},
	}
	var buf bytes.Buffer
	cmd.RenderSubcommand(&buf, "setup")
	out := buf.String()

	if !strings.Contains(out, "requires sudo") {
		t.Errorf("expected sudo note, got:\n%s", out)
	}
}

func TestFlagLabel(t *testing.T) {
	tests := []struct {
		flag Flag
		want string
	}{
		{Flag{Short: "-f"}, "-f"},
		{Flag{Long: "--verbose"}, "--verbose"},
		{Flag{Short: "-n", Arg: "name"}, "-n <name>"},
		{Flag{Short: "-f", Long: "--file", Arg: "path"}, "-f, --file <path>"},
		{Flag{Short: "-v", Long: "--verbose"}, "-v, --verbose"},
	}

	for _, tt := range tests {
		got := flagLabel(tt.flag)
		if got != tt.want {
			t.Errorf("flagLabel(%+v) = %q, want %q", tt.flag, got, tt.want)
		}
	}
}

// TestPawProxyCommandCompleteness verifies that the PawProxyCommand definition
// covers all expected subcommands.
func TestPawProxyCommandCompleteness(t *testing.T) {
	expected := []string{"setup", "uninstall", "status", "run", "logs", "doctor", "version"}
	names := make(map[string]bool)
	for _, sc := range PawProxyCommand.Subcommands {
		names[sc.Name] = true
	}
	for _, name := range expected {
		if !names[name] {
			t.Errorf("PawProxyCommand missing subcommand: %s", name)
		}
	}
}

// TestUpCommandHasEnvVars verifies that the UpCommand defines the expected environment variables.
func TestUpCommandHasEnvVars(t *testing.T) {
	expected := []string{"PORT", "APP_DOMAIN", "APP_URL", "HTTPS", "NODE_EXTRA_CA_CERTS"}
	names := make(map[string]bool)
	for _, ev := range UpCommand.EnvVars {
		names[ev.Name] = true
	}
	for _, name := range expected {
		if !names[name] {
			t.Errorf("UpCommand missing env var: %s", name)
		}
	}
}
