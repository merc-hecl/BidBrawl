// build-desktop packages the GUI and the independently usable player CLI.
package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func command(env []string, name string, args ...string) error {
	c := exec.Command(name, args...)
	c.Env = append(os.Environ(), env...)
	c.Stdout, c.Stderr = os.Stdout, os.Stderr
	return c.Run()
}

func run() error {
	platform := flag.String("platform", runtime.GOOS+"/"+runtime.GOARCH, "comma-separated desktop targets (windows/amd64,linux/amd64,darwin/arm64)")
	flag.Parse()
	var config struct {
		Version string
		Out     string
	}
	data, err := os.ReadFile("mygo.json")
	if err != nil {
		return err
	}
	if err = json.Unmarshal(data, &config); err != nil {
		return err
	}
	if err = licenses(); err != nil {
		return err
	}
	for _, target := range strings.Split(*platform, ",") {
		parts := strings.Split(target, "/")
		if len(parts) != 2 {
			return fmt.Errorf("invalid target %q", target)
		}
		goos, arch := parts[0], parts[1]
		if (goos != "windows" && goos != "linux" && goos != "darwin") || (arch != "amd64" && arch != "arm64") {
			return fmt.Errorf("unsupported desktop target %s", target)
		}
		bin := filepath.Join("build", "desktop-resources", "bin")
		if err = os.MkdirAll(bin, 0755); err != nil {
			return err
		}
		for _, name := range []string{"bidbrawl", "bidbrawl.exe", "bidbrawl-install.exe"} {
			if err = os.Remove(filepath.Join(bin, name)); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
		}
		env := []string{"GOOS=" + goos, "GOARCH=" + arch, "CGO_ENABLED=0"}
		exe := "bidbrawl"
		if goos == "windows" {
			exe += ".exe"
		}
		if err = command(env, "go", "build", "-trimpath", "-ldflags", "-s -w", "-o", filepath.Join(bin, exe), "./cmd/bidbrawl"); err != nil {
			return err
		}
		if goos == "windows" {
			if err = command(env, "go", "build", "-trimpath", "-ldflags", "-s -w -H=windowsgui", "-o", filepath.Join(bin, "bidbrawl-install.exe"), "./cmd/bidbrawl-install"); err != nil {
				return err
			}
		}
		if err = command(nil, "go", "run", "github.com/egoist/mygo/cmd/mygo@v0.6.2", "build", "-platform", target); err != nil {
			return err
		}
		dir, err := filepath.Abs(filepath.Join(config.Out, goos+"-"+arch))
		if err != nil {
			return err
		}
		switch goos {
		case "windows":
			err = windowsInstaller(dir, config.Version, arch)
		case "linux":
			err = linuxPackage(dir, config.Version, arch)
		case "darwin":
			if runtime.GOOS == "darwin" {
				err = macPackage(dir, config.Version, arch)
			} else {
				fmt.Println("macOS app built; create the CLI-inclusive .pkg on macOS with this same command.")
			}
		}
		if err != nil {
			return err
		}
		fmt.Println("Desktop and CLI:", dir)
	}
	return nil
}

func windowsInstaller(dir, version, arch string) error {
	compiler, err := exec.LookPath("makensis")
	if err != nil && runtime.GOOS == "windows" {
		cache, cacheErr := os.UserCacheDir()
		if cacheErr != nil {
			return cacheErr
		}
		for _, rel := range []string{"Bin/makensis.exe", "makensis.exe"} {
			p := filepath.Join(cache, "mygo", "nsis-3.13", rel)
			if _, e := os.Stat(p); e == nil {
				compiler = p
				err = nil
				break
			}
		}
	}
	if err != nil {
		return errors.New("install NSIS to build the Windows installer")
	}
	script, err := filepath.Abs("desktop/windows/installer.nsi")
	if err != nil {
		return err
	}
	args := []string{"-DSOURCE=" + dir, "-DOUTPUT=" + filepath.Join(dir, "BidBrawl Setup "+version+" "+arch+".exe"), "-DVERSION=" + version, script}
	if runtime.GOOS == "windows" {
		for i := 0; i < 3; i++ {
			args[i] = "/" + strings.TrimPrefix(args[i], "-")
		}
	}
	return command(nil, compiler, args...)
}

// Debian packages install CLI links in /usr/bin, without editing shell files.
func linuxPackage(dir, version, arch string) error {
	var payload bytes.Buffer
	gz := gzip.NewWriter(&payload)
	tw := tar.NewWriter(gz)
	add := func(name string, data []byte, mode int64) error {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: mode, Size: int64(len(data)), Typeflag: tar.TypeReg}); err != nil {
			return err
		}
		_, err := tw.Write(data)
		return err
	}
	for _, entry := range []string{"bidbrawl", "bin", "THIRD_PARTY_LICENSES.txt"} {
		root := filepath.Join(dir, entry)
		if err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			rel, _ := filepath.Rel(dir, path)
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			mode := int64(0755)
			if entry == "THIRD_PARTY_LICENSES.txt" {
				mode = 0644
			}
			return add("opt/bidbrawl/"+filepath.ToSlash(rel), data, mode)
		}); err != nil {
			return err
		}
	}
	if err := tw.WriteHeader(&tar.Header{Name: "usr/bin/bidbrawl", Typeflag: tar.TypeSymlink, Linkname: "/opt/bidbrawl/bin/bidbrawl", Mode: 0777}); err != nil {
		return err
	}
	desktop := []byte("[Desktop Entry]\nType=Application\nName=BidBrawl\nExec=/opt/bidbrawl/bidbrawl\nIcon=bidbrawl\nCategories=Game;\nTerminal=false\n")
	if err := add("usr/share/applications/bidbrawl.desktop", desktop, 0644); err != nil {
		return err
	}
	icon, err := os.ReadFile("internal/server/web/brand-avatar.png")
	if err != nil {
		return err
	}
	if err = add("usr/share/icons/hicolor/512x512/apps/bidbrawl.png", icon, 0644); err != nil {
		return err
	}
	if err = tw.Close(); err != nil {
		return err
	}
	if err = gz.Close(); err != nil {
		return err
	}
	controlText := fmt.Sprintf("Package: bidbrawl\nVersion: %s\nArchitecture: %s\nMaintainer: BidBrawl contributors\nDepends: libgtk-3-0 | libgtk-3-0t64, libwebkit2gtk-4.1-0\nRecommends: libayatana-appindicator3-1\nSection: games\nPriority: optional\nDescription: Agent-driven sealed-bid auctions with a desktop client and CLI\n", version, arch)
	var control bytes.Buffer
	cg := gzip.NewWriter(&control)
	ct := tar.NewWriter(cg)
	if err = ct.WriteHeader(&tar.Header{Name: "control", Mode: 0644, Size: int64(len(controlText)), Typeflag: tar.TypeReg}); err != nil {
		return err
	}
	if _, err = ct.Write([]byte(controlText)); err != nil {
		return err
	}
	if err = ct.Close(); err != nil {
		return err
	}
	if err = cg.Close(); err != nil {
		return err
	}
	f, err := os.Create(filepath.Join(dir, "bidbrawl_"+version+"_"+arch+".deb"))
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err = io.WriteString(f, "!<arch>\n"); err != nil {
		return err
	}
	for _, entry := range []struct {
		name string
		data []byte
	}{{"debian-binary", []byte("2.0\n")}, {"control.tar.gz", control.Bytes()}, {"data.tar.gz", payload.Bytes()}} {
		if _, err = fmt.Fprintf(f, "%-16s%-12d%-6d%-6d%-8o%-10d`\n", entry.name+"/", 0, 0, 0, 0644, len(entry.data)); err != nil {
			return err
		}
		if _, err = f.Write(entry.data); err != nil {
			return err
		}
		if len(entry.data)%2 != 0 {
			if _, err = f.Write([]byte{'\n'}); err != nil {
				return err
			}
		}
	}
	// MyGo's generic packages launch the GUI only; keep the complete package.
	for _, pattern := range []string{"*.tar.gz", "install.sh"} {
		matches, _ := filepath.Glob(filepath.Join(dir, pattern))
		for _, path := range matches {
			if err = os.Remove(path); err != nil {
				return err
			}
		}
	}
	return nil
}

func licenses() error {
	c := exec.Command("go", "list", "-m", "-json", "all")
	data, err := c.Output()
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	var output strings.Builder
	for {
		var module struct {
			Path, Version, Dir string
			Main               bool
		}
		if err = decoder.Decode(&module); errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			return err
		}
		if module.Main || module.Dir == "" {
			continue
		}
		found := false
		for _, name := range []string{"LICENSE", "LICENSE.txt", "LICENSE.md", "COPYING"} {
			if license, err := os.ReadFile(filepath.Join(module.Dir, name)); err == nil {
				fmt.Fprintf(&output, "%s %s\n%s\n\n", module.Path, module.Version, license)
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("license missing for %s", module.Path)
		}
	}
	license, err := os.ReadFile("internal/server/web/icons/octicons-LICENSE")
	if err != nil {
		return err
	}
	fmt.Fprintf(&output, "GitHub Octicons\n%s\n\n", license)
	if err = os.MkdirAll("build/desktop-resources", 0755); err != nil {
		return err
	}
	return os.WriteFile("build/desktop-resources/THIRD_PARTY_LICENSES.txt", []byte(output.String()), 0644)
}

func macPackage(dir, version, arch string) error {
	root, err := os.MkdirTemp("", "bidbrawl-pkg-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(root)
	app := filepath.Join(dir, "BidBrawl.app")
	if err = command(nil, "ditto", app, filepath.Join(root, "Applications", "BidBrawl.app")); err != nil {
		return err
	}
	bin := filepath.Join(root, "usr", "local", "bin")
	if err = os.MkdirAll(bin, 0755); err != nil {
		return err
	}
	if err = os.Symlink("/Applications/BidBrawl.app/Contents/Resources/bin/bidbrawl", filepath.Join(bin, "bidbrawl")); err != nil {
		return err
	}
	return command(nil, "pkgbuild", "--root", root, "--identifier", "io.github.merc-hecl.bidbrawl", "--version", version, "--ownership", "recommended", "--install-location", "/", filepath.Join(dir, "BidBrawl-"+version+"-"+arch+".pkg"))
}
