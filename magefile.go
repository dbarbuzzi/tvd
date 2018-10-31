// +build mage

package main

import (
	"os/exec"
)

var baseCommand = []string{"goreleaser", "release", "--rm-dist"}

func Build() error {
	cmd := exec.Command(...baseCommand, "--skip-publish")
	return cmd.Run()
}

func Release() error {
	cmd := exec.Command(...baseCommand)
	return cmd.Run()
}

func Snapshot() error {
	cmd := exec.Command(...baseCommand, "--snapshot")
	return cmd.Run()
}
