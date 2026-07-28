package cli

import (
	"fmt"
	"strings"

	"ansm/internal/version"
)

// usageTemplate 은 P0007 2.2 의 사용법 문구다.
//
// P0007 0장의 유일한 의도적 이탈: 원본은 프로그램 이름이 "nssm" 으로 하드코딩돼
// 있으나 이식본은 실제 실행 파일 이름(%[1]s)으로 치환한다. 사람이 읽는 안내문이며
// 기계가 파싱하지 않으므로 호환을 깨지 않는다.
//
// 첫 줄의 제품 이름은 이벤트 로그 공급자 이름과 마찬가지로 NSSM 을 유지한다.
const usageTemplate = `NSSM: The non-sucking service manager
Version %[2]s

Usage: %[1]s <option> [<args> ...]

To show service installation GUI:

	%[1]s install [<servicename>]

To install a service without confirmation:

	%[1]s install <servicename> <app> [<args> ...]

To show service editing GUI:

	%[1]s edit <servicename>

To retrieve or edit service parameters directly:

	%[1]s dump <servicename>

	%[1]s get <servicename> <parameter> [<subparameter>]

	%[1]s set <servicename> <parameter> [<subparameter>] <value>

	%[1]s reset <servicename> <parameter> [<subparameter>]

To show service removal GUI:

	%[1]s remove [<servicename>]

To remove a service without confirmation:

	%[1]s remove <servicename> confirm

To manage a service:

	%[1]s start <servicename>

	%[1]s stop <servicename>

	%[1]s restart <servicename>

	%[1]s status <servicename>

	%[1]s statuscode <servicename>

	%[1]s rotate <servicename>

	%[1]s processes <servicename>
`

// Usage 는 사용법 문구를 Windows 개행으로 조립한다.
//
// exe 는 argv[0] 에서 얻은 이름이다. 배포 시 파일 이름이 그대로 나타난다.
func Usage(exe string) string {
	versionLine := version.Number + " " + version.Configuration() + ", " + version.BuildDate
	return strings.ReplaceAll(fmt.Sprintf(usageTemplate, exe, versionLine), "\n", "\r\n")
}
