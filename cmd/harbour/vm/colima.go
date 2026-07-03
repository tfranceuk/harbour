package vm

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type Colima struct {
	cfg Config
}

var _ Backend = Colima{}

func (Colima) Name() string {
	return "Colima"
}

func (Colima) EnsureInstalled() error {
	if err := ensureCommand("colima"); err != nil {
		return fmt.Errorf("colima is required for Harbour. Install it with: brew install colima: %w", err)
	}
	return nil
}

func (c Colima) Status() (bool, error) {
	env, err := c.colimaEnv()
	if err != nil {
		return false, err
	}
	return commandSucceeded(env, "colima", "status", "-p", c.cfg.Profile)
}

// HasExactMounts reports whether the running profile's mounts are exactly the
// expected set — same count, and every expected (location, mode) present. The
// comparison is order-independent: Colima may list mounts in a different order
// than Harbour passes them. This must match the full set, not just contain it,
// so a removed or added mount forces a restart to reconcile.
func (c Colima) HasExactMounts(expected []Mount) (bool, error) {
	colimaHome, err := c.colimaHome()
	if err != nil {
		return false, err
	}
	profileConfig := filepath.Join(colimaHome, c.cfg.Profile, "colima.yaml")
	file, err := os.Open(profileConfig)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	defer file.Close()

	found := map[string]bool{}
	var mountsFound int
	scanner := bufio.NewScanner(file)
	inMounts := false
	location := ""
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "mounts:") {
			inMounts = true
			continue
		}
		if inMounts && trimmed != "" && !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") {
			inMounts = false
		}
		if !inMounts {
			continue
		}
		if strings.HasPrefix(trimmed, "- location:") {
			location = strings.TrimSpace(strings.TrimPrefix(trimmed, "- location:"))
			continue
		}
		if strings.HasPrefix(trimmed, "writable:") && location != "" {
			writable := strings.TrimSpace(strings.TrimPrefix(trimmed, "writable:")) == "true"
			found[mountKey(Mount{Location: location, Writable: writable})] = true
			mountsFound++
			location = ""
		}
	}
	if err := scanner.Err(); err != nil {
		return false, err
	}
	if mountsFound != len(expected) {
		return false, nil
	}
	for _, m := range expected {
		if !found[mountKey(m)] {
			return false, nil
		}
	}
	return true, nil
}

func mountKey(m Mount) string {
	mode := "ro"
	if m.Writable {
		mode = "rw"
	}
	return fmt.Sprintf("%s|%s", m.Location, mode)
}

func (c Colima) Start(mounts []Mount) error {
	env, err := c.colimaEnv()
	if err != nil {
		return err
	}
	// Write the host-port-forwarding override before `colima start`, so lima
	// reads it when the instance boots (see writeLimaOverride).
	if err := c.writeLimaOverride(); err != nil {
		return err
	}

	args := []string{
		"start", c.cfg.Profile,
		"--runtime", c.cfg.Runtime,
		"--vm-type", c.cfg.Type,
		"--arch", c.cfg.Arch,
		"--cpu", fmt.Sprintf("%d", c.cfg.CPU),
		"--memory", fmt.Sprintf("%d", c.cfg.Memory),
		"--disk", fmt.Sprintf("%d", c.cfg.Disk),
		"--mount-type", c.cfg.MountType,
	}
	if c.cfg.ForwardSSHAgent {
		args = append(args, "--ssh-agent")
	}
	if c.cfg.NetworkAddress {
		args = append(args, "--network-address")
	}
	for _, mount := range mounts {
		// Colima mounts read-only unless the spec ends in ":w".
		spec := mount.Location
		if mount.Writable {
			spec += ":w"
		}
		args = append(args, "--mount", spec)
	}
	fmt.Printf("Executing:\n  colima %s\n", shellQuoteArgs(args))
	return runCommand(env, "colima", args...)
}

func (c Colima) Stop() error {
	env, err := c.colimaEnv()
	if err != nil {
		return err
	}
	return runCommand(env, "colima", "stop", "-p", c.cfg.Profile)
}

func (c Colima) RunRemoteCommand(command string) error {
	env, err := c.colimaEnv()
	if err != nil {
		return err
	}
	return runCommand(env, "colima", "ssh", "-p", c.cfg.Profile, "--", "/usr/bin/bash", "-lc", command)
}

func (c Colima) RunRemoteScript(script string, args []string) error {
	env, err := c.colimaEnv()
	if err != nil {
		return err
	}
	sshArgs := append([]string{
		"ssh", "-p", c.cfg.Profile, "--", "/usr/bin/bash", "-s", "--",
	}, args...)
	return runCommandInput(env, script, "colima", sshArgs...)
}

// RunColima runs the host `colima` CLI with COLIMA_HOME set (see colimaEnv), so
// it targets Harbour's isolated home. This is the blessed way to drive Harbour's
// Colima profile by hand (e.g. `harbour colima list`, `harbour colima restart -p
// harbour`) without a bare `colima` command accidentally hitting the default
// ~/.colima home — where, with no override, a new instance would forward host
// ports again.
func (c Colima) RunColima(args []string) error {
	env, err := c.colimaEnv()
	if err != nil {
		return err
	}
	return runCommand(env, "colima", args...)
}

// colimaHome is the COLIMA_HOME for this VM. Harbour runs its Colima/lima
// instance in a home of its own — separate from the engineer's default
// ~/.colima — so the lima override from writeLimaOverride disables host port
// forwarding for THIS VM only. The engineer's default VM keeps forwarding its
// ports (e.g. the dev DB on 127.0.0.1:1119) untouched.
//
// NOTE — one-time migration: a `harbour` instance created under the default
// ~/.colima before this change is orphaned by it and will keep claiming host
// ports until removed. Once the harness points here instead, run once, with no
// COLIMA_HOME set, so it targets the default home:
//
//	colima stop -p harbour && colima delete -p harbour
//
// The harness then creates a fresh instance in the isolated home. That instance
// won't appear in a plain `colima list` — manage it with COLIMA_HOME set (or via
// Harbour).
func (c Colima) colimaHome() (string, error) {
	if c.cfg.ColimaHome != "" {
		return c.cfg.ColimaHome, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".harbour", "colima"), nil
}

// colimaEnv returns the environment for a `colima` invocation, carrying
// COLIMA_HOME so the command targets Harbour's isolated home.
//
// NOTE — apply to EVERY colima call: this must be set on all of them
// (Start/Stop/Status/ssh/RunRemote*), not just Start. If some calls set
// COLIMA_HOME and others don't, they split between the isolated home and the
// default ~/.colima and operate on different instances. That is why it is
// threaded through the shared run helpers rather than set once at Start.
func (c Colima) colimaEnv() ([]string, error) {
	home, err := c.colimaHome()
	if err != nil {
		return nil, err
	}
	return append(os.Environ(), "COLIMA_HOME="+home), nil
}

// limaOverride stops this VM forwarding any guest port to the host. lima reads
// it from $COLIMA_HOME/_lima/_config/override.yaml and applies it to every
// instance under that home; because the home is Harbour's own (see colimaHome),
// only this VM is affected. The agent can't use the dev DB anyway (the bcrypt in
// the mounted node_modules is the host's macOS build and won't load on Linux),
// so not exposing ports loses nothing and stops this VM claiming host ports the
// engineer's default VM also publishes.
const limaOverride = `# Managed by Harbour — do not edit.
# Do not forward any guest port to the host: keeps this (harbour) VM from
# claiming host ports the engineer's default Colima VM also publishes.
portForwards:
  - guestPortRange: [1, 65535]
    ignore: true
`

func (c Colima) writeLimaOverride() error {
	home, err := c.colimaHome()
	if err != nil {
		return err
	}
	dir := filepath.Join(home, "_lima", "_config")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "override.yaml"), []byte(limaOverride), 0o644)
}

func ensureCommand(name string) error {
	if _, err := exec.LookPath(name); err != nil {
		return fmt.Errorf("%s is required but not installed", name)
	}
	return nil
}

// The run helpers take env so colima invocations carry COLIMA_HOME (see
// Colima.colimaEnv). A nil env means inherit the process environment unchanged.
func runCommand(env []string, name string, args ...string) error {
	if err := ensureCommand(name); err != nil {
		return err
	}
	cmd := exec.Command(name, args...)
	if env != nil {
		cmd.Env = env
	}
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func runCommandInput(env []string, input string, name string, args ...string) error {
	if err := ensureCommand(name); err != nil {
		return err
	}
	cmd := exec.Command(name, args...)
	if env != nil {
		cmd.Env = env
	}
	cmd.Stdin = strings.NewReader(input)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func commandSucceeded(env []string, name string, args ...string) (bool, error) {
	if err := ensureCommand(name); err != nil {
		return false, err
	}
	cmd := exec.Command(name, args...)
	if env != nil {
		cmd.Env = env
	}
	cmd.Stdout = nil
	cmd.Stderr = nil
	err := cmd.Run()
	if err == nil {
		return true, nil
	}
	if _, ok := err.(*exec.ExitError); ok {
		return false, nil
	}
	return false, err
}

func shellQuoteArgs(args []string) string {
	quoted := make([]string, 0, len(args))
	for _, arg := range args {
		quoted = append(quoted, fmt.Sprintf("%q", arg))
	}
	return strings.Join(quoted, " ")
}
