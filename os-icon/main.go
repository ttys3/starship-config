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
	"pop":       " ",
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

	ID := GetOsReleaseID("/etc/os-release")
	fmt.Print(iconMap[ID])
}

func GetOsReleaseID(osReleaseFile string) string {
	if runtime.GOOS == "windows" ||
		runtime.GOOS == "darwin" ||
		runtime.GOOS == "freebsd" ||
		runtime.GOOS == "android" {
		return runtime.GOOS
	}

	if runtime.GOOS == "linux" {
		if cfg, err := ini.Load(osReleaseFile); err == nil {
			id := strings.ToLower(cfg.Section("").Key("ID").String())
			if _, ok := iconMap[id]; ok {
				return id
			}
		}
		return runtime.GOOS
	} else {
		return ""
	}
}
