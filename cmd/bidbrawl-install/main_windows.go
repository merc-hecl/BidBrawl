// This helper is run by the installer only. The game and desktop never edit PATH.
package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) != 2 {
		return errors.New("installer action required: install, uninstall, check")
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	dir := filepath.Dir(exe)
	if os.Args[1] == "check" {
		f, err := os.OpenFile(filepath.Join(filepath.Dir(dir), "BidBrawl.exe"), os.O_WRONLY, 0)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return errors.New("close BidBrawl from its tray menu before updating or uninstalling")
		}
		return f.Close()
	}
	install := os.Args[1] == "install"
	if !install && os.Args[1] != "uninstall" {
		return errors.New("unknown installer action")
	}
	key, _, err := registry.CreateKey(registry.CURRENT_USER, "Environment", registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer key.Close()
	path, valueType, err := key.GetStringValue("Path")
	if err != nil && !errors.Is(err, registry.ErrNotExist) {
		return err
	}
	owner, _, err := registry.CreateKey(registry.CURRENT_USER, `Software\BidBrawl\CLI`, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer owner.Close()
	previous, _, _ := owner.GetStringValue("Directory")
	owned, _, _ := owner.GetIntegerValue("Owned")
	equal := func(a, b string) bool {
		return strings.EqualFold(strings.TrimRight(strings.TrimSpace(a), `\/`), strings.TrimRight(b, `\/`))
	}
	found := false
	var kept []string
	for _, entry := range strings.Split(path, ";") {
		if equal(entry, dir) {
			found = true
		} else {
			kept = append(kept, entry)
		}
	}
	updated := path
	if install {
		if !found {
			updated = dir
			if path != "" {
				updated += ";" + path
			}
			owned = 1
		} else if !equal(previous, dir) {
			owned = 0
		}
		if err = owner.SetStringValue("Directory", dir); err != nil {
			return err
		}
		if err = owner.SetDWordValue("Owned", uint32(owned)); err != nil {
			return err
		}
	} else {
		if !equal(previous, dir) {
			return nil
		}
		if owned == 1 {
			updated = strings.Join(kept, ";")
		}
	}
	if updated != path {
		if valueType == registry.EXPAND_SZ {
			err = key.SetExpandStringValue("Path", updated)
		} else {
			err = key.SetStringValue("Path", updated)
		}
		if err != nil {
			return err
		}
		environment, _ := windows.UTF16PtrFromString("Environment")
		var result uintptr
		windows.NewLazySystemDLL("user32.dll").NewProc("SendMessageTimeoutW").Call(0xffff, 0x001a, 0, uintptr(unsafe.Pointer(environment)), 0x0002, 2000, uintptr(unsafe.Pointer(&result)))
	}
	if !install {
		_ = registry.DeleteKey(registry.CURRENT_USER, `Software\BidBrawl\CLI`)
	}
	return nil
}
