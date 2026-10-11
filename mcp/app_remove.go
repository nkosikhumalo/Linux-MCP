package mcp

import (
	"fmt"
	"strings"
)

// RemoveApp removes a package-managed desktop app using its owning package ID.
// Manual installs and PWAs are deliberately left untouched because their files
// may be shared or user-managed.
func RemoveApp(app *AppEntry) (string, error) {
	if app == nil {
		return "", fmt.Errorf("nil app")
	}
	if app.Kind == KindPWA || app.Kind == KindWeb {
		return fmt.Sprintf("status=unsupported kind=%s name=%q reason=\"this is a browser app or website, not a removable system package\"\n", app.Kind, app.Name), nil
	}
	if app.Package == "" {
		return fmt.Sprintf("status=unsupported kind=%s name=%q reason=\"no owning package was found; this may be a manual install\"\n", app.Kind, app.Name), nil
	}

	var res *ExecResult
	var err error
	switch app.Kind {
	case KindNative:
		res, err = RunCommand(nil, "pkexec", "apt-get", "remove", "-y", "--", app.Package)
	case KindSnap:
		res, err = RunCommand(nil, "pkexec", "snap", "remove", app.Package)
	case KindFlatpak:
		installation := flatpakInstallation(app.Package)
		if installation == "user" {
			res, err = RunCommand(nil, "flatpak", "uninstall", "--user", "--noninteractive", app.Package)
		} else if installation == "system" {
			res, err = RunCommand(nil, "pkexec", "flatpak", "uninstall", "--system", "--noninteractive", app.Package)
		} else {
			return fmt.Sprintf("status=unsupported kind=flatpak name=%q id=%q reason=\"could not determine which Flatpak installation owns this app\"\n", app.Name, app.Package), nil
		}
	default:
		return fmt.Sprintf("status=unsupported kind=%s name=%q reason=\"no safe package removal method is available\"\n", app.Kind, app.Name), nil
	}
	if err != nil {
		return "", err
	}
	if res.ExitCode != 0 || res.TimedOut || res.Cancelled {
		detail := res.Stderr
		if detail == "" {
			detail = res.Stdout
		}
		if res.TimedOut {
			detail = "package operation timed out; its final system state should be checked"
		}
		if res.Cancelled {
			detail = "package operation was cancelled; its final system state should be checked"
		}
		return fmt.Sprintf("status=fail kind=%s name=%q package=%q reason=%q exit=%d\n", app.Kind, app.Name, app.Package, detail, res.ExitCode), nil
	}
	return fmt.Sprintf("status=ok kind=%s name=%q package=%q result=%q\n", app.Kind, app.Name, app.Package, res.Stdout), nil
}

func flatpakInstallation(appID string) string {
	res, err := RunCommand(nil, "flatpak", "list", "--app", "--columns=application,installation")
	if err != nil || res.ExitCode != 0 {
		return ""
	}
	for _, line := range strings.Split(res.Stdout, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[0] == appID && (fields[1] == "user" || fields[1] == "system") {
			return fields[1]
		}
	}
	return ""
}
