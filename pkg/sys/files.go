// SPDX-License-Identifier: MIT
// © 2026 Hannes Stauss (scalarion@nimblescape.com)
// Licensed under the MIT License. See LICENSE in the repository root for details.

package sys

import (
	"io/fs"
	"os"
	"path/filepath"
)

// WriteFile writes a file and creates its folder if needed.
func WriteFile(path, content string, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		return err
	}
	return os.Chmod(path, mode)
}

// ShareWithGroup gives a group write access to a folder tree: the owner
// becomes user:group, the group can read and write, and new files in the
// folders belong to the group (setgid bit on folders).
func ShareWithGroup(root string, uid, gid int) error {
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := os.Lchown(path, uid, gid); err != nil {
			return err
		}
		if d.Type()&fs.ModeSymlink != 0 {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		mode := info.Mode() | 0o060
		if d.IsDir() {
			mode |= fs.ModeSetgid | 0o010
		}
		return os.Chmod(path, mode)
	})
}

// ChownTree changes the owner of a folder tree.
func ChownTree(root string, uid, gid int) error {
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		return os.Lchown(path, uid, gid)
	})
}
