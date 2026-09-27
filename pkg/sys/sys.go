// Package sys contains the system operations that the layers use: running
// programs, Debian packages, downloads with checksums, archives, files and
// users. All functions return errors instead of stopping the program.
package sys

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
)

// Logf prints a progress line of the build.
func Logf(format string, args ...any) {
	fmt.Printf(format+"\n", args...)
}

// Run runs a program and shows its output. extraEnv adds environment
// variables in the form NAME=value.
func Run(extraEnv []string, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(), extraEnv...)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
	}
	return nil
}

// Output runs a program and returns its standard output without surrounding
// white space.
func Output(name string, args ...string) (string, error) {
	var stderr bytes.Buffer
	cmd := exec.Command(name, args...)
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(string(out)), nil
}

// RunAs runs a program as another user, with the user's home folder in HOME
// (the calling program runs as root). extraEnv adds environment variables in
// the form NAME=value.
func RunAs(userName string, extraEnv []string, name string, args ...string) error {
	u, err := LookupUser(userName)
	if err != nil {
		return err
	}
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Dir = u.Home
	cmd.Env = append(os.Environ(), "HOME="+u.Home, "USER="+u.Name, "LOGNAME="+u.Name)
	cmd.Env = append(cmd.Env, extraEnv...)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Credential: &syscall.Credential{Uid: uint32(u.UID), Gid: uint32(u.GID), Groups: u.Groups},
	}
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s %s (as %s): %w", name, strings.Join(args, " "), userName, err)
	}
	return nil
}

// Exists reports whether a path exists.
func Exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// IsRoot reports whether the program runs as root.
func IsRoot() bool {
	return os.Geteuid() == 0
}
