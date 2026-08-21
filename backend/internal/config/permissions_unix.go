//go:build !windows

package config

import (
	"fmt"
	"os"
)

func validateConfigFilePermissions(path string, info os.FileInfo) error {
	if info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("config file %s must not be accessible by group or others", path)
	}
	return nil
}
