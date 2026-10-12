package main

import (
	"fmt"
	"net"
	"net/url"
	"os/exec"
	"runtime"
)

func browserAddress(address net.Addr) string {
	host, port, _ := net.SplitHostPort(address.String())
	if ip := net.ParseIP(host); ip != nil && ip.IsUnspecified() {
		host = "127.0.0.1"
	}
	return "http://" + net.JoinHostPort(host, port)
}

func observerLaunchURL(address, room, code string) string {
	u, _ := url.Parse(address)
	if room != "" {
		q := u.Query()
		q.Set("room", room)
		u.RawQuery = q.Encode()
	}
	u.Fragment = "launch=" + url.QueryEscape(code)
	return u.String()
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
