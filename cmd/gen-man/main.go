// gen-man generates roff-formatted man pages from the structured help data
// in internal/help. Run with: go run ./cmd/gen-man
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/alexcatdad/paw-proxy/internal/help"
)

func main() {
	date := time.Now().Format("2006-01")

	cmds := []struct {
		cmd  *help.Command
		file string
	}{
		{&help.PawProxyCommand, "man/paw-proxy.1"},
		{&help.UpCommand, "man/up.1"},
	}

	for _, c := range cmds {
		if err := os.MkdirAll(filepath.Dir(c.file), 0755); err != nil {
			fmt.Fprintf(os.Stderr, "error creating directory: %v\n", err)
			os.Exit(1)
		}
		f, err := os.Create(c.file)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error creating %s: %v\n", c.file, err)
			os.Exit(1)
		}
		writeManPage(f, c.cmd, date)
		f.Close()
		fmt.Printf("Generated %s\n", c.file)
	}
}

func writeManPage(f *os.File, cmd *help.Command, date string) {
	name := strings.ToUpper(cmd.Name)
	w := func(format string, args ...interface{}) {
		fmt.Fprintf(f, format+"\n", args...)
	}

	// Title header
	w(`.TH %s 1 "%s" "paw-proxy" "User Commands"`, name, date)

	// NAME
	w(".SH NAME")
	w(`%s \- %s`, cmd.Name, escRoff(cmd.Summary))

	// SYNOPSIS
	w(".SH SYNOPSIS")
	writeSynopsis(f, cmd)

	// DESCRIPTION
	w(".SH DESCRIPTION")
	w(`.B %s`, cmd.Name)
	if cmd.Description != "" {
		w("%s", escRoff(cmd.Description))
	} else {
		w("%s", escRoff(cmd.Summary)+".")
	}

	// COMMANDS (for binaries with subcommands)
	if len(cmd.Subcommands) > 0 {
		w(".SH COMMANDS")
		for _, sc := range cmd.Subcommands {
			w(".TP")
			w(`.B %s`, sc.Name)
			desc := escRoff(sc.Summary)
			if sc.RequiresRoot {
				desc += " Requires sudo."
			}
			w("%s", desc)
			for _, fl := range sc.Flags {
				w(".RS")
				w(".TP")
				w(`.B %s`, escFlag(fl))
				w("%s", escRoff(fl.Desc))
				w(".RE")
			}
		}
	}

	// OPTIONS (for top-level flags)
	if len(cmd.Flags) > 0 {
		w(".SH OPTIONS")
		for _, fl := range cmd.Flags {
			w(".TP")
			w(`.B %s`, escFlag(fl))
			w("%s", escRoff(fl.Desc))
		}
	}

	// ENVIRONMENT
	if len(cmd.EnvVars) > 0 {
		w(".SH ENVIRONMENT")
		for _, ev := range cmd.EnvVars {
			w(".TP")
			w(`.B %s`, ev.Name)
			w("%s", escRoff(ev.Desc))
		}
	}

	// EXAMPLES
	if len(cmd.Examples) > 0 {
		w(".SH EXAMPLES")
		for _, ex := range cmd.Examples {
			if ex.Desc != "" {
				w(".PP")
				w("%s:", escRoff(ex.Desc))
			}
			w(".PP")
			w(".RS")
			w(".nf")
			w("$ %s", escRoff(ex.Command))
			w(".fi")
			w(".RE")
		}
	}

	// FILES
	if len(cmd.Files) > 0 {
		w(".SH FILES")
		for _, fp := range cmd.Files {
			w(".TP")
			w(`.I %s`, escRoff(fp.Path))
			w("%s", escRoff(fp.Desc))
		}
	}

	// SEE ALSO
	if len(cmd.SeeAlso) > 0 {
		w(".SH SEE ALSO")
		refs := make([]string, len(cmd.SeeAlso))
		for i, sa := range cmd.SeeAlso {
			refs[i] = fmt.Sprintf(`.BR %s`, sa)
		}
		w("%s", strings.Join(refs, ",\n"))
	}
}

func writeSynopsis(f *os.File, cmd *help.Command) {
	w := func(format string, args ...interface{}) {
		fmt.Fprintf(f, format+"\n", args...)
	}
	w(`.B %s`, cmd.Name)
	if len(cmd.Subcommands) > 0 {
		w(`.RI [ command ]`)
		w(`.RI [ options ]`)
	} else {
		for _, fl := range cmd.Flags {
			if fl.Short != "" && fl.Arg != "" {
				w(`.RB [ %s`, fl.Short)
				w(`.IR %s ]`, fl.Arg)
			} else if fl.Long != "" {
				w(`.RB [ %s ]`, escRoff(fl.Long))
			} else if fl.Short != "" {
				w(`.RB [ %s ]`, fl.Short)
			}
		}
		w(`.I command`)
		w(`.RI [ args... ]`)
	}
}

// escRoff escapes text for roff: hyphens become \-, leading dots become \&.
func escRoff(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, "-", `\-`)
	if strings.HasPrefix(s, ".") {
		s = `\&` + s
	}
	return s
}

// escFlag formats a Flag for roff display.
func escFlag(fl help.Flag) string {
	var parts []string
	if fl.Short != "" {
		parts = append(parts, escRoff(fl.Short))
	}
	if fl.Long != "" {
		parts = append(parts, escRoff(fl.Long))
	}
	label := strings.Join(parts, " , ")
	if fl.Arg != "" {
		label += " \\fI" + fl.Arg + "\\fR"
	}
	return label
}
