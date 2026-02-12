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
		if err := writeManPage(f, c.cmd, date); err != nil {
			fmt.Fprintf(os.Stderr, "error writing %s: %v\n", c.file, err)
			os.Exit(1)
		}
		if err := f.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "error closing %s: %v\n", c.file, err)
			os.Exit(1)
		}
		fmt.Printf("Generated %s\n", c.file)
	}
}

// roffWriter wraps an *os.File and captures the first write error.
type roffWriter struct {
	f   *os.File
	err error
}

func (rw *roffWriter) w(format string, args ...interface{}) {
	if rw.err != nil {
		return
	}
	_, rw.err = fmt.Fprintf(rw.f, format+"\n", args...)
}

func writeManPage(f *os.File, cmd *help.Command, date string) error {
	name := strings.ToUpper(cmd.Name)
	rw := &roffWriter{f: f}

	// Title header
	rw.w(`.TH %s 1 "%s" "paw-proxy" "User Commands"`, name, date)

	// NAME
	rw.w(".SH NAME")
	rw.w(`%s \- %s`, cmd.Name, escRoff(cmd.Summary))

	// SYNOPSIS
	rw.w(".SH SYNOPSIS")
	writeSynopsis(rw, cmd)

	// DESCRIPTION
	rw.w(".SH DESCRIPTION")
	rw.w(`.B %s`, cmd.Name)
	if cmd.Description != "" {
		rw.w("%s", escRoff(cmd.Description))
	} else {
		rw.w("%s", escRoff(cmd.Summary)+".")
	}

	// COMMANDS (for binaries with subcommands)
	if len(cmd.Subcommands) > 0 {
		rw.w(".SH COMMANDS")
		for _, sc := range cmd.Subcommands {
			rw.w(".TP")
			rw.w(`.B %s`, sc.Name)
			rw.w("%s", escRoff(sc.Summary))
			for _, fl := range sc.Flags {
				rw.w(".RS")
				rw.w(".TP")
				rw.w(`.B %s`, escFlag(fl))
				rw.w("%s", escRoff(fl.Desc))
				rw.w(".RE")
			}
		}
	}

	// OPTIONS (for top-level flags)
	if len(cmd.Flags) > 0 {
		rw.w(".SH OPTIONS")
		for _, fl := range cmd.Flags {
			rw.w(".TP")
			rw.w(`.B %s`, escFlag(fl))
			rw.w("%s", escRoff(fl.Desc))
		}
	}

	// ENVIRONMENT
	if len(cmd.EnvVars) > 0 {
		rw.w(".SH ENVIRONMENT")
		for _, ev := range cmd.EnvVars {
			rw.w(".TP")
			rw.w(`.B %s`, ev.Name)
			rw.w("%s", escRoff(ev.Desc))
		}
	}

	// EXAMPLES
	if len(cmd.Examples) > 0 {
		rw.w(".SH EXAMPLES")
		for _, ex := range cmd.Examples {
			if ex.Desc != "" {
				rw.w(".PP")
				rw.w("%s:", escRoff(ex.Desc))
			}
			rw.w(".PP")
			rw.w(".RS")
			rw.w(".nf")
			rw.w("$ %s", escRoff(ex.Command))
			rw.w(".fi")
			rw.w(".RE")
		}
	}

	// FILES
	if len(cmd.Files) > 0 {
		rw.w(".SH FILES")
		for _, fp := range cmd.Files {
			rw.w(".TP")
			rw.w(`.I %s`, escRoff(fp.Path))
			rw.w("%s", escRoff(fp.Desc))
		}
	}

	// SEE ALSO
	if len(cmd.SeeAlso) > 0 {
		rw.w(".SH SEE ALSO")
		refs := make([]string, len(cmd.SeeAlso))
		for i, sa := range cmd.SeeAlso {
			refs[i] = fmt.Sprintf(`.BR %s`, escRoff(sa))
		}
		rw.w("%s", strings.Join(refs, ",\n"))
	}

	if rw.err != nil {
		return fmt.Errorf("write man page: %w", rw.err)
	}
	return nil
}

func writeSynopsis(rw *roffWriter, cmd *help.Command) {
	rw.w(`.B %s`, cmd.Name)
	if len(cmd.Subcommands) > 0 {
		rw.w(`.RI [ command ]`)
		rw.w(`.RI [ options ]`)
	} else {
		for _, fl := range cmd.Flags {
			if fl.Short != "" && fl.Arg != "" {
				rw.w(`.RB [ %s`, fl.Short)
				rw.w(`.IR %s ]`, fl.Arg)
			} else if fl.Long != "" {
				rw.w(`.RB [ %s ]`, escRoff(fl.Long))
			} else if fl.Short != "" {
				rw.w(`.RB [ %s ]`, fl.Short)
			}
		}
		rw.w(`.I command`)
		rw.w(`.RI [ args... ]`)
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
