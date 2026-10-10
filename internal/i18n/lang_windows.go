//go:build windows

package i18n

import "golang.org/x/sys/windows"

func windowsLang() string {
	langs, err := windows.GetUserPreferredUILanguages(windows.MUI_LANGUAGE_NAME)
	if err != nil || len(langs) == 0 {
		return ""
	}
	return langs[0]
}
