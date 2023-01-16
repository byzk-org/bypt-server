package utils

import "runtime"

func PathAddSuffix(n string) string {
	if runtime.GOOS == "windows" {
		return n + ".exe"
	}
	return n
}
