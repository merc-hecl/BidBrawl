package main

import (
	"fmt"
	"net"
	"os/exec"
	"runtime"

	"bidbrawl/internal/host"
)

func browserAddress(address net.Addr) string {
	return host.BrowserAddress(address)
}

func observerLaunchURL(address, room, code string) string {
	return host.LaunchURL(address, room, code)
}

func openBrowser(address string) error {
	var command *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		command = exec.Command("rundll32.exe", "url.dll,FileProtocolHandler", address)
	case "darwin":
		command = exec.Command("open", address)
	case "linux":
		command = exec.Command("xdg-open", address)
	default:
		return fmt.Errorf("unsupported desktop: %s", runtime.GOOS)
	}
	return command.Run()
}
