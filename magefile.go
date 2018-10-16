// +build mage

package main

import (
	"os/exec"
)

func Build() error {
	cmd := exec.Command("goreleaser", "release", "--rm-dist", "--skip-publish")
	return cmd.Run()
}

func Release() error {
	cmd := exec.Command("goreleaser", "release", "--rm-dist")
	return cmd.Run()
}

func Snapshot() error {
	cmd := exec.Command("goreleaser", "release", "--rm-dist", "--snapshot")
	return cmd.Run()
}
