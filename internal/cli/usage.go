package cli

import (
	"fmt"
	"strings"

	"ansm/internal/version"
)

// usageTemplate follows the documented behavioral contract. See P0007 2.2, P0007, NSSM.
const usageTemplate = `NSSM: The non-sucking service manager
Version %[2]s

Usage: %[1]s <option> [<args> ...]

To show service installation GUI:

	%[1]s install [<servicename>]

To show the installation GUI directly, without the install verb:

	%[1]s gui

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

// Usage follows the documented behavioral contract. See Usage, Windows.
func Usage(exe string) string {
	versionLine := version.Number + " " + version.Configuration() + ", " + version.BuildDate
	return strings.ReplaceAll(fmt.Sprintf(usageTemplate, exe, versionLine), "\n", "\r\n")
}
