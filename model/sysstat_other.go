//go:build !linux && !darwin

package model

func readSysStat() SysStat {
	return SysStat{}
}
