//go:build linux

package setup

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/alexcatdad/paw-proxy/internal/ssl"
)

func Run(config *Config) error {
	fmt.Println("paw-proxy setup")
	fmt.Println("================")

	// 1. Create support directory
	fmt.Printf("\n[1/6] Creating support directory...\n")
	if err := os.MkdirAll(config.SupportDir, 0700); err != nil {
		return fmt.Errorf("creating support dir: %w", err)
	}
	// SECURITY: chown support dir to the real user when running under sudo,
	// so the systemd user service (which runs as the user) can access it.
	if err := chownToRealUser(config.SupportDir); err != nil {
		return fmt.Errorf("fixing support dir ownership: %w", err)
	}
	fmt.Printf("  ✓ %s\n", config.SupportDir)

	// 2. Generate CA
	fmt.Printf("\n[2/6] Generating CA certificate...\n")
	certPath := filepath.Join(config.SupportDir, "ca.crt")
	keyPath := filepath.Join(config.SupportDir, "ca.key")

	if _, err := os.Stat(certPath); err == nil {
		fmt.Printf("  ✓ CA already exists\n")
	} else {
		if err := ssl.GenerateCA(certPath, keyPath); err != nil {
			return fmt.Errorf("generating CA: %w", err)
		}
		fmt.Printf("  ✓ Generated CA certificate\n")
	}
	// SECURITY: chown CA files to the real user so the daemon can read them.
	if err := chownToRealUser(certPath, keyPath); err != nil {
		return fmt.Errorf("fixing CA file ownership: %w", err)
	}

	// 3. Trust CA in system store
	fmt.Printf("\n[3/6] Adding CA to system trust store...\n")
	if err := trustCA(certPath); err != nil {
		return fmt.Errorf("trusting CA: %w", err)
	}
	fmt.Printf("  ✓ CA trusted in system store\n")

	// 4. Configure DNS resolver (systemd-resolved)
	fmt.Printf("\n[4/6] Configuring DNS resolver (systemd-resolved)...\n")
	if err := configureResolver(config.TLD, config.DNSPort); err != nil {
		return fmt.Errorf("configuring resolver: %w", err)
	}
	fmt.Printf("  ✓ systemd-resolved configured for .%s\n", config.TLD)

	// 5. Set capabilities on binary for port 80/443 binding
	fmt.Printf("\n[5/6] Setting port binding capabilities...\n")
	if err := setCapabilities(config.BinaryPath); err != nil {
		return fmt.Errorf("setting capabilities: %w", err)
	}
	fmt.Printf("  ✓ cap_net_bind_service set on %s\n", config.BinaryPath)

	// 6. Install systemd user service
	fmt.Printf("\n[6/6] Installing systemd user service...\n")
	if err := installSystemdUnit(config); err != nil {
		return fmt.Errorf("installing systemd unit: %w", err)
	}
	fmt.Printf("  ✓ systemd user service installed and started\n")

	fmt.Println("\n================")
	fmt.Println("Setup complete!")
	fmt.Println("")
	fmt.Println("Note: Restart your browser to pick up the new CA certificate.")
	fmt.Println("")
	fmt.Println("Firefox users: Install NSS tools for certificate trust:")
	fmt.Println("  sudo apt install libnss3-tools   # Debian/Ubuntu")
	fmt.Println("  sudo dnf install nss-tools        # Fedora/RHEL")
	fmt.Println("  sudo paw-proxy setup              # Re-run to update Firefox")
	fmt.Println("")
	fmt.Println("Note: If you upgrade the binary, re-run 'sudo paw-proxy setup'")
	fmt.Println("      to restore port binding capabilities.")
	fmt.Println("")
	fmt.Println("Usage:")
	fmt.Println("  up bun dev           # Start dev server with HTTPS")
	fmt.Println("  up -n myapp npm start # Custom domain name")

	return nil
}

// trustCA installs the CA certificate into the system trust store.
// Supports Debian/Ubuntu (update-ca-certificates) and Fedora/RHEL/Arch (update-ca-trust).
func trustCA(certPath string) error {
	// SECURITY: Try Debian/Ubuntu first, then Fedora/RHEL/Arch.
	// Detect by binary existence rather than distro name to handle edge cases.
	if _, err := exec.LookPath("update-ca-certificates"); err == nil {
		dest := "/usr/local/share/ca-certificates/paw-proxy-ca.crt"
		if err := copyFile(certPath, dest); err != nil {
			return fmt.Errorf("copying CA to %s: %w", dest, err)
		}
		cmd := exec.Command("update-ca-certificates")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	}

	if _, err := exec.LookPath("update-ca-trust"); err == nil {
		dest := "/etc/pki/ca-trust/source/anchors/paw-proxy-ca.crt"
		if err := copyFile(certPath, dest); err != nil {
			return fmt.Errorf("copying CA to %s: %w", dest, err)
		}
		cmd := exec.Command("update-ca-trust")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	}

	return fmt.Errorf("no supported CA trust tool found (need update-ca-certificates or update-ca-trust)")
}

// configureResolver sets up a systemd-resolved stub zone for the .test TLD.
// Requires systemd 247+ for non-standard port syntax in DNS= directive.
func configureResolver(tld string, port int) error {
	// Check that systemd-resolved is active
	if err := exec.Command("systemctl", "is-active", "--quiet", "systemd-resolved").Run(); err != nil {
		return fmt.Errorf("systemd-resolved is not active; configure DNS routing for .%s manually", tld)
	}

	confDir := "/etc/systemd/resolved.conf.d"
	if err := os.MkdirAll(confDir, 0755); err != nil {
		return fmt.Errorf("creating %s: %w", confDir, err)
	}

	content := fmt.Sprintf("# Generated by paw-proxy\n[Resolve]\nDNS=127.0.0.1:%d\nDomains=~%s\n", port, tld)
	confPath := filepath.Join(confDir, "paw-proxy.conf")

	if err := os.WriteFile(confPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("writing %s: %w", confPath, err)
	}

	cmd := exec.Command("systemctl", "restart", "systemd-resolved")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// setCapabilities grants the binary permission to bind to privileged ports
// (80, 443) without running as root.
func setCapabilities(binaryPath string) error {
	if _, err := exec.LookPath("setcap"); err != nil {
		return fmt.Errorf("setcap not found; install libcap2-bin (Debian/Ubuntu) or libcap (Fedora/RHEL)")
	}
	// SECURITY: cap_net_bind_service=+ep grants the binary effective+permitted
	// capability to bind ports <1024 without root. This is file-scoped and does
	// not persist to child processes. Cleared when the binary is overwritten.
	cmd := exec.Command("setcap", "cap_net_bind_service=+ep", binaryPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

var systemdUnitTemplate = `# Generated by paw-proxy
[Unit]
Description=paw-proxy local HTTPS proxy
After=network.target

[Service]
ExecStart={{.BinaryPath}} run
Restart=always
RestartSec=1s

[Install]
WantedBy=default.target
`

// installSystemdUnit installs and enables a systemd user service.
func installSystemdUnit(config *Config) error {
	homeDir, err := realUserHomeDir()
	if err != nil {
		return fmt.Errorf("cannot determine home directory: %w", err)
	}
	unitDir := filepath.Join(homeDir, ".config", "systemd", "user")
	unitPath := filepath.Join(unitDir, "paw-proxy.service")

	if err := os.MkdirAll(unitDir, 0755); err != nil {
		return fmt.Errorf("creating %s: %w", unitDir, err)
	}

	f, err := os.Create(unitPath)
	if err != nil {
		return fmt.Errorf("creating unit file: %w", err)
	}
	defer f.Close()

	tmpl, err := template.New("unit").Parse(systemdUnitTemplate)
	if err != nil {
		return fmt.Errorf("parsing systemd unit template: %w", err)
	}

	if err := tmpl.Execute(f, config); err != nil {
		return fmt.Errorf("rendering systemd unit template: %w", err)
	}

	// SECURITY: chown unit file and all parent dirs to the real user so systemd
	// can traverse the path. MkdirAll may have created ~/.config and ~/.config/systemd
	// as root, which would prevent the user's systemd session from finding the unit.
	configDir := filepath.Join(homeDir, ".config")
	systemdDir := filepath.Join(configDir, "systemd")
	if err := chownToRealUser(unitPath, unitDir, systemdDir, configDir); err != nil {
		return fmt.Errorf("fixing unit file ownership: %w", err)
	}

	// Reload and enable the service as the real user
	if err := systemctlAsUser("daemon-reload"); err != nil {
		return fmt.Errorf("daemon-reload: %w", err)
	}
	if err := systemctlAsUser("enable", "--now", "paw-proxy"); err != nil {
		return fmt.Errorf("enabling service: %w", err)
	}

	return nil
}

// systemctlAsUser runs a systemctl --user subcommand as the real user.
// When running under sudo, XDG_RUNTIME_DIR and DBUS_SESSION_BUS_ADDRESS
// must be explicitly set for the real user's session.
func systemctlAsUser(args ...string) error {
	if os.Getuid() == 0 {
		if sudoUser := os.Getenv("SUDO_USER"); sudoUser != "" {
			uid, err := resolveRealUID()
			if err != nil {
				return err
			}
			xdgRuntime := fmt.Sprintf("XDG_RUNTIME_DIR=/run/user/%d", uid)
			dbusAddr := fmt.Sprintf("DBUS_SESSION_BUS_ADDRESS=unix:path=/run/user/%d/bus", uid)
			sudoArgs := []string{"-u", sudoUser, "env", xdgRuntime, dbusAddr, "systemctl", "--user"}
			sudoArgs = append(sudoArgs, args...)
			cmd := exec.Command("sudo", sudoArgs...)
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			return cmd.Run()
		}
	}
	cmd := exec.Command("systemctl", append([]string{"--user"}, args...)...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// realUserHomeDir returns the home directory of the real user (not root)
// when running under sudo.
func realUserHomeDir() (string, error) {
	if os.Getuid() == 0 {
		if sudoUser := os.Getenv("SUDO_USER"); sudoUser != "" {
			// getent passwd format: name:x:uid:gid:gecos:home:shell
			out, err := exec.Command("getent", "passwd", sudoUser).Output()
			if err == nil {
				fields := strings.Split(strings.TrimRight(string(out), "\r\n"), ":")
				if len(fields) >= 6 && fields[5] != "" {
					return fields[5], nil
				}
			}
		}
	}
	return os.UserHomeDir()
}

// copyFile copies src to dst with 0644 permissions.
func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0644)
}
