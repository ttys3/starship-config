package main

// https://stackoverflow.com/a/64959957/13267147
// https://github.com/zcalusic/sysinfo/blob/master/os.go

import (
	"fmt"
	"log"

	"github.com/go-ini/ini"
)

var iconMap = map[string]string{
	"arch":     " ",
	"ubuntu":   " ",
	"fedora":   " ",
	"centos":   " ",
	"debian":   " ",
	"gentoo":   " ",
	"opensuse": " ",
	"apple":    " ",
	"windows":  " ",
	"":         "err",
}

func main() {
	OSInfo := ReadOSRelease("/etc/os-release")
	osID := OSInfo["ID"]
	fmt.Print(iconMap[osID])
}

func ReadOSRelease(configfile string) map[string]string {
	cfg, err := ini.Load(configfile)
	if err != nil {
		log.Fatal("Fail to read file: ", err)
	}

	osReleaseMap := make(map[string]string)
	osReleaseMap["ID"] = cfg.Section("").Key("ID").String()

	return osReleaseMap
}
