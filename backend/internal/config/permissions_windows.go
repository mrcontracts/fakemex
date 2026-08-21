//go:build windows

package config

import (
	"fmt"
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

func validateConfigFilePermissions(path string, _ os.FileInfo) error {
	descriptor, err := windows.GetNamedSecurityInfo(
		path,
		windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION,
	)
	if err != nil {
		return fmt.Errorf("inspect config file ACL %s: %w", path, err)
	}
	if descriptor == nil {
		return insecureConfigACLError(path)
	}
	dacl, _, err := descriptor.DACL()
	if err != nil || dacl == nil {
		return insecureConfigACLError(path)
	}

	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return fmt.Errorf("inspect current Windows identity: %w", err)
	}
	system, err := windows.CreateWellKnownSid(windows.WinLocalSystemSid)
	if err != nil {
		return fmt.Errorf("resolve Windows Local System identity: %w", err)
	}
	administrators, err := windows.CreateWellKnownSid(windows.WinBuiltinAdministratorsSid)
	if err != nil {
		return fmt.Errorf("resolve Windows Administrators identity: %w", err)
	}
	trusted := []*windows.SID{user.User.Sid, system, administrators}
	readMask := windows.ACCESS_MASK(windows.GENERIC_ALL | windows.GENERIC_READ | windows.FILE_READ_DATA)

	for i := uint16(0); i < dacl.AceCount; i++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(dacl, uint32(i), &ace); err != nil {
			return fmt.Errorf("inspect config file ACL entry %s: %w", path, err)
		}
		if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE || ace.Mask&readMask == 0 {
			continue
		}
		sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
		allowed := false
		for _, candidate := range trusted {
			if windows.EqualSid(sid, candidate) {
				allowed = true
				break
			}
		}
		if !allowed {
			return insecureConfigACLError(path)
		}
	}
	return nil
}

func insecureConfigACLError(path string) error {
	return fmt.Errorf("config file %s grants read access outside the current user, Local System, or Administrators; restrict its Windows ACL", path)
}
