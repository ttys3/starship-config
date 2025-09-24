package main

// https://stackoverflow.com/a/64959957/13267147
// https://github.com/zcalusic/sysinfo/blob/master/os.go

import (
	"flag"
	"fmt"
	"runtime"
	"sort"
	"strings"

	"github.com/go-ini/ini"
)

var iconMap = map[string]string{
	"arch":      " ",
	"ubuntu":    " ",
	"fedora":    " ",
	"centos":    " ",
	"debian":    " ",
	"gentoo":    " ",
	"opensuse":  " ",
	"manjaro":   " ",
	"linuxmint": " ",
	"darwin":    " ",
	"windows":   " ",
	"freebsd":   " ",
	// "android":   " ",
	"linux": " ",

	"": " ",
}

var listIcons bool

func main() {
	flag.BoolVar(&listIcons, "list", false, "list available icons")
	flag.Parse()

	if listIcons {
		osArr := make([]string, 0, len(iconMap))
		for os := range iconMap {
			if os == "" {
				continue
			}
			osArr = append(osArr, os)
		}
		sort.Strings(osArr)

		for _, os := range osArr {
			fmt.Printf("%10s %s\n", os, iconMap[os])
		}
		return
	}

	ID := GetOsID()
	fmt.Print(iconMap[ID])
}

// GetOsID returns the operating system identifier
// For macOS, Windows, FreeBSD, and Android, it returns the GOOS value directly
// For Linux, it attempts to read the distribution ID from /etc/os-release
func GetOsID() string {
	switch runtime.GOOS {
	case "windows", "darwin", "freebsd", "android":
		return runtime.GOOS
	case "linux":
		return getLinuxDistroID()
	default:
		return ""
	}
}

// getLinuxDistroID reads the Linux distribution ID from /etc/os-release
func getLinuxDistroID() string {
	const osReleaseFile = "/etc/os-release"
	if cfg, err := ini.Load(osReleaseFile); err == nil {
		if id := cfg.Section("").Key("ID").String(); id != "" {
			return strings.ToLower(id)
		}
	}
	// Fallback to generic linux if /etc/os-release is not available or ID is empty
	return "linux"
}
