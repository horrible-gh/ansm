// mkrsrc 는 Windows 리소스 오브젝트(`.syso`)를 만든다.
//
// 이벤트 뷰어는 ANSM 이 남긴 이벤트 번호를 실행 파일 안의 MESSAGETABLE 에서
// 찾는다(P0007 1.1 의 EventMessageFile). Go 링커는 리소스를 스스로 만들지
// 못하므로 이 도구가 대신 만든다. 외부 툴체인(mc.exe·rc.exe·windres)은 쓰지
// 않는다 — 그 의존을 버리는 것이 이식의 동기였다(docs/T1-spike.md 1장).
//
// 만든 오브젝트는 저장소에 함께 넣는다. 평소 빌드는 `go build ./cmd/ansm`
// 하나로 끝난다.
//
//	go generate ./cmd/ansm      # 두 대상 모두 다시 만든다
//	go run ./tools/mkrsrc -arch amd64 -out cmd/ansm/rsrc_windows_amd64.syso
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"ansm/internal/msgcat"
	"ansm/internal/rsrc"
	"ansm/internal/version"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "mkrsrc: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	var (
		catalogPath = flag.String("messages", filepath.Join("resources", "messages.mc"), "message catalogue to compile")
		iconPath    = flag.String("icon", "", "optional .ico file to embed")
		arches      = flag.String("arch", "", "GOARCH to build for; empty means every supported architecture")
		out         = flag.String("out", "", "output path; empty means cmd/ansm/rsrc_windows_<arch>.syso")
		number      = flag.String("version", version.Number, "version string")
		date        = flag.String("date", version.BuildDate, "build date")
	)
	flag.Parse()

	if *out != "" && *arches == "" {
		return fmt.Errorf("-out needs a single -arch")
	}

	targets := rsrc.Arches
	if *arches != "" {
		arch, err := rsrc.ArchByName(*arches)
		if err != nil {
			return err
		}
		targets = []rsrc.Arch{arch}
	}

	catalog, err := msgcat.ParseFile(*catalogPath)
	if err != nil {
		return err
	}

	var icon []byte
	if *iconPath != "" {
		if icon, err = os.ReadFile(*iconPath); err != nil {
			return err
		}
	}

	for _, arch := range targets {
		set := &rsrc.Set{}
		if err := rsrc.AddMessageTables(set, catalog); err != nil {
			return err
		}
		if err := rsrc.AddManifest(set, rsrc.DefaultManifest, neutralLanguage); err != nil {
			return err
		}
		if icon != nil {
			if err := rsrc.AddIcon(set, icon, groupIconName, firstIconName, englishLanguage); err != nil {
				return err
			}
		}
		info, err := versionInfo(arch, *number, *date).Build()
		if err != nil {
			return err
		}
		if err := set.Add(rsrc.Entry{
			Type:     rsrc.TypeVersion,
			Name:     1,
			Language: englishLanguage,
			Data:     info,
		}); err != nil {
			return err
		}

		path := *out
		if path == "" {
			path = filepath.Join("cmd", "ansm", "rsrc_windows_"+arch.GOARCH+".syso")
		}
		if err := write(path, arch, set); err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "mkrsrc: wrote %s\n", path)
	}
	return nil
}

// 리소스 이름과 언어. 원본과 같은 자리를 쓴다.
const (
	englishLanguage = 0x0409
	// 매니페스트는 언어를 가리지 않는다.
	neutralLanguage = 0x0409
	groupIconName   = 101
	firstIconName   = 1
)

func versionInfo(arch rsrc.Arch, number, date string) rsrc.VersionInfo {
	configuration := "64-bit"
	if arch.GOARCH == "386" {
		configuration = "32-bit"
	}
	numeric := version.Numeric(number)

	return rsrc.VersionInfo{
		FileVersion:    numeric,
		ProductVersion: numeric,
		PreRelease:     version.PreRelease(number),
		Strings: map[string]string{
			"Comments":         "Go port of NSSM, https://nssm.cc/",
			"FileDescription":  "The non-sucking service manager",
			"FileVersion":      number,
			"InternalName":     "ansm",
			"LegalCopyright":   "Public Domain; Author Iain Patterson 2003-2017",
			"OriginalFilename": "ansm.exe",
			"ProductName":      "NSSM " + configuration,
			"ProductVersion":   number,
			"BuildDate":        date,
		},
		// 메시지 표가 가진 세 언어를 그대로 적는다. 코드페이지 0x04B0 은 유니코드다.
		Translations: []rsrc.Translation{
			{Language: 0x0409, CodePage: 0x04b0},
			{Language: 0x040c, CodePage: 0x04b0},
			{Language: 0x0410, CodePage: 0x04b0},
		},
	}
}

func write(path string, arch rsrc.Arch, set *rsrc.Set) error {
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := rsrc.WriteObject(f, arch, set); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
